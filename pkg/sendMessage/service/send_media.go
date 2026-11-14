package send_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

// The fixed sidecar and waveform sent with voice notes from a URL.
const (
	voiceStreamingSidecar = "QpmXDsU7YLagdg=="
	voiceWaveform         = "OjAnExISDgsKCAkJBwgkHAQEBBEFAwMNAxAcKCgkFzM0QUE4Jh4eKAoKChcLCwkeFgkJCQo3JiQmIiIRPz8/Ow=="
)

// clientGone reports an error that retrying on a reconnected client can fix.
func clientGone(err error) bool {
	return strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session")
}

// deliverMedia validates and converts a file by its type, uploads it and sends the
// message. Uploads and URLs both end here, so a fix to a media type lands once.
func (s *sendService) deliverMedia(
	client *whatsmeow.Client,
	data *MediaStruct,
	fileData []byte,
	mimeType string,
	instance *instance_model.Instance,
	voiceMarkers bool,
) (*MessageSendStruct, error) {
	var err error
	var uploadType whatsmeow.MediaType
	var duration int

	switch data.Type {
	case "image":
		if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
			errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'image/jpeg', 'image/png' and 'image/webp' are accepted", mimeType)
			return nil, errors.New(errMsg)
		}
		if mimeType == "image/webp" {
			mimeType = "image/jpeg"
		}
		uploadType = whatsmeow.MediaImage
	case "video", "ptv", "gif":
		// An animated GIF is not accepted as video by WhatsApp: transcode it
		// to a silent MP4 and mark the message gifPlayback so it loops.
		if gifRequested(data, fileData, mimeType) {
			converted, cerr := convertGifToMP4(fileData)
			if cerr != nil {
				return nil, cerr
			}
			fileData = converted
			mimeType = "video/mp4"
			data.GifPlayback = true
		}
		if mimeType != "video/mp4" {
			errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'video/mp4' is accepted", mimeType)
			return nil, errors.New(errMsg)
		}
		uploadType = whatsmeow.MediaVideo
	case "audio":
		converterApiUrl := s.config.ApiAudioConverter
		converterApiKey := s.config.ApiAudioConverterKey
		var convertedData []byte
		var err error
		if converterApiUrl == "" {

			convertedData, duration, err = convertAudioToOpusWithDuration(fileData)
			if err != nil {
				return nil, err
			}
		} else {
			convertedData, duration, err = convertAudioWithApi(converterApiUrl, converterApiKey, ConvertAudio{Base64: base64.StdEncoding.EncodeToString(fileData)})
			if err != nil {
				return nil, err
			}
		}

		fileData = convertedData
		mimeType = "audio/ogg; codecs=opus"
		uploadType = whatsmeow.MediaAudio
	case "document":
		uploadType = whatsmeow.MediaDocument
	default:
		return nil, errors.New("invalid media type")
	}

	// Detectar se é newsletter para usar upload sem criptografia
	isNewsletter := strings.Contains(data.Number, "@newsletter")

	// Validar se é documento em newsletter (não suportado)
	if isNewsletter && data.Type == "document" {
		return nil, errors.New("documentos não são suportados em canais do WhatsApp. Use imagem, vídeo ou áudio")
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaFile - Upload iniciado (Newsletter: %v)...", instance.Id, isNewsletter)

	var uploaded whatsmeow.UploadResponse
	if isNewsletter {
		// Newsletter: upload SEM criptografia
		uploaded, err = s.uploadNewsletter(context.Background(), client, fileData, uploadType)
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Newsletter upload - Handle: %s", instance.Id, uploaded.Handle)
	} else {
		// Normal: upload COM criptografia
		uploaded, err = s.upload(context.Background(), client, fileData, uploadType)
	}

	if err != nil {
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Media uploaded with size %d", instance.Id, uploaded.FileLength)

	var media *waE2E.Message
	var mediaType string

	switch data.Type {
	case "image":
		// Generate a JPEG preview thumbnail for better client UX (iOS in
		// particular). On failure jpegThumb is nil and the message is sent
		// without a preview rather than failing the request.
		jpegThumb := makeJPEGThumbnail(fileData, 72)
		imgW, imgH, hasDims := imageDimensions(fileData)
		if isNewsletter {
			// Newsletter: SEM MediaKey e FileEncSHA256
			img := &waE2E.ImageMessage{
				Caption:       proto.String(data.Caption),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				Mimetype:      proto.String(mimeType),
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
				JPEGThumbnail: jpegThumb,
			}
			if hasDims {
				img.Width, img.Height = proto.Uint32(imgW), proto.Uint32(imgH)
			}
			media = &waE2E.Message{ImageMessage: img}
		} else {
			// Normal: COM MediaKey e FileEncSHA256
			img := &waE2E.ImageMessage{
				Caption:       proto.String(data.Caption),
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(fileData))),
				JPEGThumbnail: jpegThumb,
			}
			if hasDims {
				img.Width, img.Height = proto.Uint32(imgW), proto.Uint32(imgH)
			}
			media = &waE2E.Message{ImageMessage: img}
		}
		mediaType = "ImageMessage"
	case "video", "gif":
		// Video bubbles likewise use width/height (and seconds) for their
		// placeholder; without them clients draw a generic box. The first
		// frame gives a real preview instead of a blank one.
		meta, hasMeta := probeVideo(fileData)
		vidThumb := makeVideoThumbnail(fileData, 72)
		if isNewsletter {
			vid := &waE2E.VideoMessage{
				Caption:    proto.String(data.Caption),
				URL:        &uploaded.URL,
				DirectPath: &uploaded.DirectPath,
				Mimetype:   proto.String(mimeType),
				FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength,
			}
			applyVideoMeta(vid, meta, hasMeta, vidThumb)
			media = &waE2E.Message{VideoMessage: vid}
		} else {
			vid := &waE2E.VideoMessage{
				Caption:       proto.String(data.Caption),
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(fileData))),
			}
			applyVideoMeta(vid, meta, hasMeta, vidThumb)
			media = &waE2E.Message{VideoMessage: vid}
		}
		mediaType = "VideoMessage"
	case "ptv":
		// Round video notes use the same placeholder metadata as videos.
		meta, hasMeta := probeVideo(fileData)
		vidThumb := makeVideoThumbnail(fileData, 72)
		if isNewsletter {
			vid := &waE2E.VideoMessage{
				URL:        &uploaded.URL,
				DirectPath: &uploaded.DirectPath,
				Mimetype:   proto.String(mimeType),
				FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength,
			}
			applyVideoMeta(vid, meta, hasMeta, vidThumb)
			media = &waE2E.Message{PtvMessage: vid}
		} else {
			vid := &waE2E.VideoMessage{
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(fileData))),
			}
			applyVideoMeta(vid, meta, hasMeta, vidThumb)
			media = &waE2E.Message{PtvMessage: vid}
		}
		mediaType = "PtvMessage"
	case "audio":
		if isNewsletter {
			media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
				URL:        &uploaded.URL,
				PTT:        proto.Bool(true),
				DirectPath: &uploaded.DirectPath,
				Mimetype:   proto.String(mimeType),
				FileSHA256: uploaded.FileSHA256,
				FileLength: &uploaded.FileLength,
				Seconds:    proto.Uint32(uint32(duration)),
			}}
		} else {
			media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
				URL:           proto.String(uploaded.URL),
				PTT:           proto.Bool(true),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uploaded.FileLength),
				Seconds:       proto.Uint32(uint32(duration)),
			}}
		}
		if voiceMarkers {
			media.AudioMessage.StreamingSidecar = []byte(voiceStreamingSidecar)
			media.AudioMessage.Waveform = []byte(voiceWaveform)
		}
		mediaType = "AudioMessage"
	case "document":
		// For PDF documents, rasterize page 1 into a JPEG preview thumbnail.
		// A missing pdftoppm or a failure yields nil and the document is
		// sent without a preview instead of failing the request.
		var jpegThumb []byte
		if mimeType == "application/pdf" {
			jpegThumb = makePDFThumbnail(fileData, 200)
		}
		if isNewsletter {
			media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
				FileName:      &data.Filename,
				Caption:       proto.String(data.Caption),
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				Mimetype:      proto.String(mimeType),
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
				JPEGThumbnail: jpegThumb,
			}}
		} else {
			media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
				FileName:      &data.Filename,
				Caption:       proto.String(data.Caption),
				URL:           proto.String(uploaded.URL),
				DirectPath:    proto.String(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      proto.String(mimeType),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(fileData))),
				JPEGThumbnail: jpegThumb,
			}}
		}

		if media.GetDocumentMessage().GetCaption() != "" {
			media.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					DocumentMessage: media.DocumentMessage,
				},
			}
			media.DocumentMessage = nil
		}

		mediaType = "DocumentMessage"
	default:
		return nil, errors.New("invalid media type")
	}

	// view-once: when requested, flag the media so it disappears after the recipient opens it
	// once (image, video and voice/PTV support it; documents do not). Additive — without the
	// flag the media is a normal message, exactly as before.
	if data.ViewOnce {
		if media.ImageMessage != nil {
			media.ImageMessage.ViewOnce = proto.Bool(true)
		}
		if media.VideoMessage != nil {
			media.VideoMessage.ViewOnce = proto.Bool(true)
		}
		if media.AudioMessage != nil {
			media.AudioMessage.ViewOnce = proto.Bool(true)
		}
		if media.PtvMessage != nil {
			media.PtvMessage.ViewOnce = proto.Bool(true)
		}
	}

	// A GIF is delivered as a silent looping video: mark it so clients
	// autoplay it without controls instead of showing a video player.
	if data.GifPlayback && media != nil && media.VideoMessage != nil {
		media.VideoMessage.GifPlayback = proto.Bool(true)
	}

	return s.SendMessage(instance, media, mediaType, &SendDataStruct{
		Id:              data.Id,
		Number:          data.Number,
		Quoted:          data.Quoted,
		Delay:           data.Delay,
		MentionAll:      data.MentionAll,
		MentionedJID:    data.MentionedJID,
		FormatJid:       data.FormatJid,
		MediaHandle:     uploaded.Handle,
		ForwardingScore: data.ForwardingScore,
		MediaData:       fileData,
		AlbumParentKey:  data.AlbumParentKey,
		AlbumIndex:      data.AlbumIndex,
	})
}
