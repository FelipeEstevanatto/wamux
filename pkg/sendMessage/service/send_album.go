package send_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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

	// Fetch every item before the container goes out. A URL that fails, or a
	// payload that is not base64, must not leave the earlier items already sent.
	items := make([]preparedAlbumItem, len(data.Items))
	for i, item := range data.Items {
		prepared, err := s.prepareAlbumItem(data, item, i)
		if err != nil {
			return nil, fmt.Errorf("album item %d failed: %w", i, err)
		}
		items[i] = prepared
	}

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

	// The container is already on WhatsApp. If a later item is rejected, revoke
	// everything accepted so far and report the failure with nothing left behind.
	accepted := []string{containerID}
	for i := range items {
		items[i].media.AlbumParentKey = parentKey
		sent, err := s.SendMediaFile(items[i].media, items[i].data, instance)
		if err != nil {
			s.rollbackAlbum(client, instance, recipient, accepted)
			return nil, fmt.Errorf("album item %d failed: %w", i, err)
		}
		if sent != nil && sent.Info.ID != "" {
			accepted = append(accepted, sent.Info.ID)
		}
		result.Items = append(result.Items, sent)
	}

	return result, nil
}

// rollbackAlbum revokes an album that could not be finished. ids[0] is the
// container, which is not a history row; the rest are items already stored.
// A revoke is not echoed back, so each item is tombstoned here.
func (s *sendService) rollbackAlbum(client *whatsmeow.Client, instance *instance_model.Instance, chat types.JID, ids []string) {
	if client == nil || instance == nil {
		return
	}
	logger := s.loggerWrapper.GetLogger(instance.Id)
	direct := chat.Server != types.GroupServer
	chatID := chat.ToNonAD().String()
	for i, id := range ids {
		if id == "" {
			continue
		}
		revoke := client.BuildRevoke(chat, types.EmptyJID, id)
		if _, err := s.deliver(context.Background(), client, chat, revoke, whatsmeow.SendRequestExtra{ID: client.GenerateMessageID()}); err != nil {
			logger.LogError("[%s] album rollback could not revoke %s: %v", instance.Id, id, err)
		}
		whatsmeow_service.ForgetSentMessage(client, id, chat)
		if i == 0 || s.messageRepository == nil {
			continue
		}
		row := message_model.Message{
			MessageID:   id,
			InstanceId:  instance.Id,
			Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
			Status:      "Sent",
			Source:      chat.User,
			ChatJid:     chatID,
			SenderJid:   chatID,
			MessageType: "delete",
			IsFromMe:    true,
		}
		if _, err := s.messageRepository.MarkMessageDeleted(row, []string{chatID}, direct); err != nil {
			logger.LogError("[%s] album rollback could not tombstone %s: %v", instance.Id, id, err)
		}
		if err := localmedia.Remove(instance.Id, id); err != nil {
			logger.LogError("[%s] album rollback could not remove media for %s: %v", instance.Id, id, err)
		}
	}
}

// preparedAlbumItem is one album item whose bytes are already in memory.
type preparedAlbumItem struct {
	media *MediaStruct
	data  []byte
}

// prepareAlbumItem decodes or downloads one item. A URL is copied onto the
// media struct: SendMediaUrl reads MediaStruct.Url, not the album field.
func (s *sendService) prepareAlbumItem(album *AlbumStruct, item AlbumItem, index int) (preparedAlbumItem, error) {
	media := &MediaStruct{
		Number:     album.Number,
		Type:       strings.ToLower(item.Mediatype),
		Caption:    item.Caption,
		Filename:   item.FileName,
		FormatJid:  album.FormatJid,
		AlbumIndex: proto.Int32(int32(index)),
	}
	// A reply and mentions belong to the album, not to every item.
	if index == 0 {
		media.Quoted = album.Quoted
		media.MentionedJID = album.MentionedJID
		media.MentionAll = album.MentionAll
		media.Delay = album.Delay
	}

	if isMediaURL(item.Media) {
		media.Url = strings.TrimSpace(item.Media)
		data, err := fetchAlbumMedia(media.Url)
		if err != nil {
			return preparedAlbumItem{}, err
		}
		return preparedAlbumItem{media: media, data: data}, nil
	}
	data, err := decodeAlbumMedia(item.Media)
	if err != nil {
		return preparedAlbumItem{}, err
	}
	return preparedAlbumItem{media: media, data: data}, nil
}

// fetchAlbumMedia downloads one album item. The caller has not sent anything
// yet, so a refusal here leaves the chat untouched.
func fetchAlbumMedia(rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), largeDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := largeMediaHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to download media: HTTP %s", resp.Status)
	}
	return readAllLimited(resp.Body)
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
