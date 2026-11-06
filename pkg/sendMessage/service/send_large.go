package send_service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
)

// MaxInMemoryMediaBytes is the largest file sent from memory, which is what thumbnails
// and audio conversion need. Larger files stream from disk with flat memory.
const MaxInMemoryMediaBytes = 64 << 20

// largeDownloadTimeout bounds one media download by URL, which can run to gigabytes.
const largeDownloadTimeout = 30 * time.Minute

// ErrMediaTooLarge means a file is over MAX_UPLOAD_BYTES.
var ErrMediaTooLarge = errors.New("file is larger than MAX_UPLOAD_BYTES")

// ErrLargeMediaType means the type needs the file in memory. Over the in-memory
// limit only documents and MP4 videos can be sent.
var ErrLargeMediaType = fmt.Errorf("files over %d MiB must be sent as document or as an MP4 video", MaxInMemoryMediaBytes>>20)

// SendMediaStream sends a large file without loading it into memory. file must stay
// open until it returns. There is no retry: a failed attempt can cost minutes of
// upload, so the caller decides whether to try again.
func (s *sendService) SendMediaStream(data *MediaStruct, file *os.File, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := uint64(info.Size())

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	detected, err := mimetype.DetectReader(file)
	if err != nil {
		return nil, err
	}
	mimeType := detected.String()

	isNewsletter := strings.Contains(data.Number, "@newsletter")
	var uploadType whatsmeow.MediaType
	switch data.Type {
	case "document":
		if isNewsletter {
			return nil, errors.New("documentos não são suportados em canais do WhatsApp. Use imagem, vídeo ou áudio")
		}
		uploadType = whatsmeow.MediaDocument
	case "video":
		if mimeType != "video/mp4" {
			return nil, ErrLargeMediaType
		}
		uploadType = whatsmeow.MediaVideo
	default:
		return nil, ErrLargeMediaType
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	logger := s.loggerWrapper.GetLogger(instance.Id)
	start := time.Now()
	var uploaded whatsmeow.UploadResponse
	if isNewsletter {
		uploaded, err = s.uploadNewsletterReader(context.Background(), client, file, uploadType)
	} else {
		uploaded, err = s.uploadReader(context.Background(), client, file, uploadType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}
	logger.LogInfo("[%s] Large media uploaded in %v, size: %d", instance.Id, time.Since(start), size)

	var media *waE2E.Message
	var mediaType string
	if data.Type == "video" {
		video := &waE2E.VideoMessage{
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			Mimetype:      proto.String(mimeType),
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(size),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
		}
		meta, hasMeta := probeVideoFile(file.Name())
		applyVideoMeta(video, meta, hasMeta, videoThumbnailFromFile(file.Name(), 72))
		if data.ViewOnce {
			video.ViewOnce = proto.Bool(true)
		}
		media, mediaType = &waE2E.Message{VideoMessage: video}, "VideoMessage"
	} else {
		document := &waE2E.DocumentMessage{
			FileName:      proto.String(data.Filename),
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			Mimetype:      proto.String(mimeType),
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(size),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
		}
		media, mediaType = &waE2E.Message{DocumentMessage: document}, "DocumentMessage"
		if data.Caption != "" {
			media = &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: media}}
		}
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
		MediaFile:       file.Name(),
	})
}

// readOrSpool reads a body into memory, or, past the in-memory limit, into a temporary
// file the caller must close and remove. Exactly one of the results is set.
func readOrSpool(body io.Reader, maxBytes int64) ([]byte, *os.File, error) {
	head, err := io.ReadAll(io.LimitReader(body, MaxInMemoryMediaBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(head)) <= MaxInMemoryMediaBytes {
		return head, nil, nil
	}

	file, err := localmedia.CreateTemp()
	if err != nil {
		return nil, nil, err
	}
	written, err := io.Copy(file, io.LimitReader(io.MultiReader(bytes.NewReader(head), body), maxBytes+1))
	if err == nil && written > maxBytes {
		err = ErrMediaTooLarge
	}
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, nil, err
	}
	return nil, file, nil
}
