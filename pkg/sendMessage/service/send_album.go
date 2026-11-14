package send_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// Album item limits. WhatsApp renders an album for two or more items and the
// clients become unreliable well before the schema's theoretical maximum, so
// keep the same bound the reference implementation uses.
const (
	albumMinItems = 2
	albumMaxItems = 30
)

// AlbumItem is one image or video inside an album. Media accepts a URL, a data
// URI, or raw base64, matching /send/media.
type AlbumItem struct {
	// Mediatype is image or video.
	Mediatype string `json:"mediatype" enums:"image,video" example:"image"`
	// Media is a URL, a data URI, or raw base64.
	Media string `json:"media" example:"https://example.com/foto.jpg"`
	// Mimetype is optional. The server detects the real type from the bytes.
	Mimetype string `json:"mimetype,omitempty" example:"image/jpeg"`
	// Caption is shown under this item only.
	Caption string `json:"caption,omitempty" example:"Primeira foto"`
	// FileName is optional metadata for the item.
	FileName string `json:"fileName,omitempty" example:"foto.jpg"`
}

// AlbumStruct is the body of POST /send/album.
type AlbumStruct struct {
	Number string      `json:"number" example:"5511999999999"`
	Items  []AlbumItem `json:"items"`
	// Id optionally sets the album container's WhatsApp message id.
	Id           string       `json:"id,omitempty" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Delay        int32        `json:"delay,omitempty" example:"1200"`
	MentionedJID []string     `json:"mentionedJid,omitempty" example:"5511999999999@s.whatsapp.net"`
	MentionAll   bool         `json:"mentionAll,omitempty" example:"false"`
	FormatJid    *bool        `json:"formatJid,omitempty" example:"true"`
	Quoted       QuotedStruct `json:"quoted,omitempty"`
}

// AlbumContainer is the container message WhatsApp uses to group the items.
type AlbumContainer struct {
	ID                 string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Timestamp          string `json:"timestamp" example:"2026-11-14T12:00:00Z"`
	ExpectedImageCount int    `json:"expectedImageCount" example:"2"`
	ExpectedVideoCount int    `json:"expectedVideoCount" example:"0"`
}

// AlbumSendResult is returned by SendAlbum. Items are in request order.
type AlbumSendResult struct {
	Album AlbumContainer       `json:"album"`
	Items []*MessageSendStruct `json:"items"`
}

// SendAlbum sends one album container followed by its media items. Each item is
// a normal media message tied to the container by MessageAssociation; WhatsApp
// clients render them as one album while history keeps one row per item.
func (s *sendService) SendAlbum(data *AlbumStruct, instance *instance_model.Instance) (*AlbumSendResult, error) {
	if data == nil {
		return nil, errors.New("album data is required")
	}
	if strings.TrimSpace(data.Number) == "" {
		return nil, errors.New("phone number is required")
	}
	if len(data.Items) < albumMinItems || len(data.Items) > albumMaxItems {
		return nil, fmt.Errorf("an album needs between %d and %d items", albumMinItems, albumMaxItems)
	}

	imageCount, videoCount := 0, 0
	for i, item := range data.Items {
		if strings.TrimSpace(item.Media) == "" {
			return nil, fmt.Errorf("album item %d has no media", i)
		}
		switch strings.ToLower(item.Mediatype) {
		case "image":
			imageCount++
		case "video":
			videoCount++
		default:
			return nil, fmt.Errorf("album item %d has unsupported mediatype %q (use image or video)", i, item.Mediatype)
		}
	}

	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	recipient, err := s.validateAndCheckUserExists(data.Number, data.FormatJid, nil, nil, instance)
	if err != nil {
		return nil, err
	}

	// Same normalization SendMessage applies: a "+" in the user part makes the
	// WhatsApp node lookup fail.
	recipient.User = strings.ReplaceAll(recipient.User, "+", "")

	containerID := data.Id
	if containerID == "" {
		containerID = client.GenerateMessageID()
	}
	container := &waE2E.Message{AlbumMessage: &waE2E.AlbumMessage{
		ExpectedImageCount: proto.Uint32(uint32(imageCount)),
		ExpectedVideoCount: proto.Uint32(uint32(videoCount)),
	}}
	resp, err := s.deliver(context.Background(), client, recipient, container, whatsmeow.SendRequestExtra{ID: containerID})
	if err != nil {
		return nil, fmt.Errorf("failed to send album container: %w", err)
	}

	parentKey := &waCommon.MessageKey{
		RemoteJID: proto.String(recipient.ToNonAD().String()),
		FromMe:    proto.Bool(true),
		ID:        proto.String(containerID),
	}

	result := &AlbumSendResult{
		Album: AlbumContainer{
			ID:                 containerID,
			Timestamp:          resp.Timestamp.Format(time.RFC3339),
			ExpectedImageCount: imageCount,
			ExpectedVideoCount: videoCount,
		},
		Items: make([]*MessageSendStruct, 0, len(data.Items)),
	}

	for i, item := range data.Items {
		media := &MediaStruct{
			Number:         data.Number,
			Type:           strings.ToLower(item.Mediatype),
			Caption:        item.Caption,
			Filename:       item.FileName,
			FormatJid:      data.FormatJid,
			AlbumParentKey: parentKey,
			AlbumIndex:     proto.Int32(int32(i)),
		}
		// A reply and mentions belong to the album, not to every item: put
		// them on the first one only.
		if i == 0 {
			media.Quoted = data.Quoted
			media.MentionedJID = data.MentionedJID
			media.MentionAll = data.MentionAll
			media.Delay = data.Delay
		}

		var sent *MessageSendStruct
		if isMediaURL(item.Media) {
			sent, err = s.SendMediaUrl(media, instance)
		} else {
			var raw []byte
			raw, err = decodeAlbumMedia(item.Media)
			if err == nil {
				sent, err = s.SendMediaFile(media, raw, instance)
			}
		}
		if err != nil {
			return result, fmt.Errorf("album item %d failed: %w", i, err)
		}
		result.Items = append(result.Items, sent)
	}

	return result, nil
}

// isMediaURL reports whether media is an HTTP(S) URL rather than base64.
func isMediaURL(media string) bool {
	lower := strings.ToLower(strings.TrimSpace(media))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// decodeAlbumMedia accepts a data URI or raw base64. A data URI's declared
// MIME type is ignored on purpose: the send path detects the real type from the
// bytes, so a mislabeled payload cannot become a broken message.
func decodeAlbumMedia(media string) ([]byte, error) {
	media = strings.TrimSpace(media)
	if strings.HasPrefix(media, "data:") {
		comma := strings.IndexByte(media, ',')
		if comma < 0 {
			return nil, errors.New("invalid data URI")
		}
		media = media[comma+1:]
	}
	if decoded, err := base64.StdEncoding.DecodeString(media); err == nil {
		return decoded, nil
	}
	decoded, err := base64.RawStdEncoding.DecodeString(media)
	if err != nil {
		return nil, errors.New("media is neither a URL nor valid base64")
	}
	return decoded, nil
}
