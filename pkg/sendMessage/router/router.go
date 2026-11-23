// Package router selects the transport for the existing sendMessage API.
// Baileys keeps its implementation; Cloud gets neutral DTOs. This keeps the
// public route/response shape stable without scattering integration checks
// through every send implementation.
package router

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	send_service "github.com/felipeestevanatto/wamux/pkg/sendMessage/service"
	"go.mau.fi/whatsmeow/types"
)

// Router implements send_service.SendService.
type Router struct {
	baileys send_service.SendService
	cloud   *cloud_service.Service
}

func New(baileys send_service.SendService, cloud *cloud_service.Service) *Router {
	return &Router{baileys: baileys, cloud: cloud}
}

var _ send_service.SendService = (*Router)(nil)

func isCloud(inst *instance_model.Instance) bool {
	return inst != nil && channel.Normalize(inst.Integration) == channel.KindCloud
}

func unsupported(op string) error {
	return &channel.UnsupportedError{Channel: channel.KindCloud, Op: op}
}

func notImplemented(op string) error {
	return &channel.NotImplementedError{Channel: channel.KindCloud, Op: op}
}

func (r *Router) SendText(data *send_service.TextStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendText(data, inst)
	}
	res, err := r.cloud.SendText(context.Background(), inst, channel.TextMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Body: data.Text, PreviewURL: true, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendLink(data *send_service.LinkStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendLink(data, inst)
	}
	body := data.Text
	if strings.TrimSpace(body) == "" {
		body = data.Url
	}
	res, err := r.cloud.SendText(context.Background(), inst, channel.TextMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Body: body, PreviewURL: true, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendMediaUrl(data *send_service.MediaStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendMediaUrl(data, inst)
	}
	res, err := r.cloud.SendMedia(context.Background(), inst, channel.MediaMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Kind: data.Type, URL: data.Url, Filename: data.Filename, Caption: data.Caption,
		ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendMediaStream(data *send_service.MediaStruct, file *os.File, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendMediaStream(data, file, inst)
	}
	if file == nil {
		return nil, &cloud_service.APIError{Status: 400, Code: "invalid_request", Message: "media file is required"}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(io.LimitReader(file, 100<<20+1))
	if err != nil {
		return nil, err
	}
	return r.SendMediaFile(data, buf, inst)
}

func (r *Router) SendMediaFile(data *send_service.MediaStruct, fileData []byte, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendMediaFile(data, fileData, inst)
	}
	res, err := r.cloud.SendMedia(context.Background(), inst, channel.MediaMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Kind: data.Type, Data: fileData, Filename: data.Filename, Caption: data.Caption,
		ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendAlbum(data *send_service.AlbumStruct, inst *instance_model.Instance) (*send_service.AlbumSendResult, error) {
	if !isCloud(inst) {
		return r.baileys.SendAlbum(data, inst)
	}
	return nil, unsupported("send/album")
}

func (r *Router) SendPoll(data *send_service.PollStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendPoll(data, inst)
	}
	return nil, unsupported("send/poll")
}

func (r *Router) SendSticker(data *send_service.StickerStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendSticker(data, inst)
	}
	res, err := r.cloud.SendMedia(context.Background(), inst, channel.MediaMessage{
		To: data.Number, Kind: "sticker", URL: data.Sticker, ReplyTo: data.Quoted.MessageID,
		OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendLocation(data *send_service.LocationStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendLocation(data, inst)
	}
	res, err := r.cloud.SendLocation(context.Background(), inst, channel.LocationMessage{
		To: data.Number, Recipient: data.Recipient, Name: data.Name,
		Latitude: data.Latitude, Longitude: data.Longitude,
		Address: data.Address, ReplyTo: data.Quoted.MessageID,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendContact(data *send_service.ContactStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendContact(data, inst)
	}
	res, err := r.cloud.SendContact(context.Background(), inst, channel.ContactMessage{
		To: data.Number, Recipient: data.Recipient, ReplyTo: data.Quoted.MessageID,
		Contacts: []channel.Contact{{
			Name: data.Vcard.FullName, Phones: []string{data.Vcard.Phone},
		}},
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendButton(data *send_service.ButtonStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendButton(data, inst)
	}
	buttons := make([]channel.InteractiveButton, 0, len(data.Buttons))
	for _, b := range data.Buttons {
		buttons = append(buttons, channel.InteractiveButton{
			Type: b.Type, Text: b.DisplayText, ID: b.Id, URL: b.URL,
			Phone: b.PhoneNumber, CopyCode: b.CopyCode,
		})
	}
	res, err := r.cloud.SendInteractive(context.Background(), inst, channel.InteractiveMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Header: data.Title, Body: data.Description, Footer: data.Footer,
		Buttons: buttons, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendList(data *send_service.ListStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendList(data, inst)
	}
	sections := make([]channel.InteractiveSection, 0, len(data.Sections))
	for _, sec := range data.Sections {
		rows := make([]channel.InteractiveListRow, 0, len(sec.Rows))
		for _, row := range sec.Rows {
			rows = append(rows, channel.InteractiveListRow{Title: row.Title, Description: row.Description, ID: row.RowId})
		}
		sections = append(sections, channel.InteractiveSection{Title: sec.Title, Rows: rows})
	}
	res, err := r.cloud.SendInteractive(context.Background(), inst, channel.InteractiveMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Header: data.Title, Body: data.Description, Footer: data.FooterText,
		ButtonText: data.ButtonText, Sections: sections, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func (r *Router) SendCarousel(data *send_service.CarouselStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendCarousel(data, inst)
	}
	return nil, notImplemented("send/carousel")
}

func (r *Router) SendEvent(data *send_service.EventStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendEvent(data, inst)
	}
	return nil, unsupported("send/event")
}

func (r *Router) SendProduct(data *send_service.ProductStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendProduct(data, inst)
	}
	res, err := r.cloud.SendProduct(context.Background(), inst, channel.ProductMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		CatalogID: data.CatalogId, ProductRetailerID: firstNonEmpty(data.ProductRetailerId, data.RetailerId, data.ProductId),
		Body: data.Body, Footer: data.Footer, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

// SendCatalog is a Cloud-only catalog message.
func (r *Router) SendCatalog(data *send_service.CatalogStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return nil, unsupported("send/catalog")
	}
	res, err := r.cloud.SendCatalog(context.Background(), inst, channel.CatalogMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Body: data.Body, Footer: data.Footer, ThumbnailProductRetailerID: data.ThumbnailProductRetailerId,
		ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

// SendContactRequest is a Cloud-only contact request message.
func (r *Router) SendContactRequest(data *send_service.ContactRequestStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return nil, unsupported("send/contact-request")
	}
	res, err := r.cloud.SendContactRequest(context.Background(), inst, channel.ContactRequestMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Body: data.Body, ReplyTo: data.Quoted.MessageID, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (r *Router) SendStatusText(data *send_service.StatusTextStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendStatusText(data, inst)
	}
	return nil, unsupported("send/status/text")
}

func (r *Router) SendStatusMediaUrl(data *send_service.StatusMediaStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendStatusMediaUrl(data, inst)
	}
	return nil, unsupported("send/status/media")
}

func (r *Router) SendStatusMediaFile(data *send_service.StatusMediaStruct, fileData []byte, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		return r.baileys.SendStatusMediaFile(data, fileData, inst)
	}
	return nil, unsupported("send/status/media")
}

func (r *Router) SendTemplate(data *send_service.TemplateStruct, inst *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	if !isCloud(inst) {
		if r.baileys != nil {
			return r.baileys.SendTemplate(data, inst)
		}
		return nil, unsupported("send/template")
	}
	res, err := r.cloud.SendTemplate(context.Background(), inst, channel.TemplateMessage{
		To: data.Number, Recipient: data.Recipient, RecipientType: data.RecipientType,
		Name: data.Name, Language: data.Language, Components: data.Components, OperationID: data.Id,
	})
	return toMessageSend(res, err), err
}

func toMessageSend(res channel.Result, err error) *send_service.MessageSendStruct {
	if err != nil {
		return nil
	}
	out := &send_service.MessageSendStruct{
		Info: types.MessageInfo{
			ID:        types.MessageID(res.ID),
			Timestamp: res.Timestamp,
		},
		Channel: "cloud",
		Cloud: &send_service.CloudSendInfo{
			WAMID: res.ID, Recipient: res.Recipient, WAID: res.WAID,
		},
	}
	if res.WAID != "" {
		out.Info.Chat = types.NewJID(res.WAID, types.DefaultUserServer)
		out.Info.Sender = out.Info.Chat
	}
	return out
}

// Compile-time check that errors.As on the typed API error remains possible.
var _ = errors.As
