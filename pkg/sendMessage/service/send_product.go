package send_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// Catalog product card (waE2E.ProductMessage).
//
// Unlike the catalog CRUD (the `w:biz:catalog` IQ, which Meta disabled), this is
// a regular MESSAGE -- same send path as text/media -- so it works with the
// current protocol. Products are created/managed in the official app or Meta
// Commerce Manager; the API only posts the card into the conversation.
// ============================================================================

// ProductStruct is the body of POST /send/product.
type ProductStruct struct {
	Number string `json:"number" example:"5511999999999"`
	Id     string `json:"id,omitempty" example:"3EB0A1B2C3D4E5F6A7B8C9"`

	// Product data shown on the card. ProductId must be the product's ID in your
	// catalog -- get it from the official app / Commerce Manager.
	ProductId   string `json:"productId" example:"1234567890123456"`
	Title       string `json:"title" example:"Camiseta Evolution GO"`
	Description string `json:"description,omitempty" example:"Camiseta oficial 100% algodao"`
	// Price in thousandths of the currency unit: R$ 10,00 => 10000.
	Price      int64  `json:"price" example:"10000"`
	Currency   string `json:"currency" example:"BRL"`
	RetailerId string `json:"retailerId,omitempty" example:"SKU-001"`
	Url        string `json:"url,omitempty" example:"https://loja.example.com/produto/1234567890123456"`

	// Product image: base64 or an external URL (downloaded and re-uploaded).
	ImageBase64 string `json:"imageBase64,omitempty" example:"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQ..."`
	ImageURL    string `json:"imageUrl,omitempty" example:"https://loja.example.com/imagens/camiseta.jpg"`

	// Catalog owner JID. When empty, the instance's own JID is used.
	BusinessOwnerJid string `json:"businessOwnerJid,omitempty" example:"5511999999999@s.whatsapp.net"`

	Body   string `json:"body,omitempty" example:"Aproveite esta oferta!"`
	Footer string `json:"footer,omitempty" example:"Evolution GO"`

	Delay        int32        `json:"delay" example:"1200"`
	MentionedJID []string     `json:"mentionedJid" example:"5511999999999@s.whatsapp.net"`
	MentionAll   bool         `json:"mentionAll" example:"false"`
	FormatJid    *bool        `json:"formatJid,omitempty" example:"true"`
	Quoted       QuotedStruct `json:"quoted"`
}

const productImageMaxBytes = 16 << 20 // 16 MiB

var productImageHTTPClient = &http.Client{Timeout: 30 * time.Second}

func productImageBytes(data *ProductStruct) ([]byte, error) {
	if data.ImageBase64 != "" {
		b64 := data.ImageBase64
		if strings.HasPrefix(b64, "data:") {
			if i := strings.Index(b64, ","); i > 0 {
				b64 = b64[i+1:]
			}
		}
		img, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("invalid imageBase64: %w", err)
		}
		return img, nil
	}
	if data.ImageURL != "" {
		resp, err := productImageHTTPClient.Get(data.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download imageUrl: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download imageUrl, status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, productImageMaxBytes+1))
		if err != nil {
			return nil, fmt.Errorf("failed to read imageUrl: %w", err)
		}
		if len(body) > productImageMaxBytes {
			return nil, fmt.Errorf("imageUrl exceeds %d byte limit", productImageMaxBytes)
		}
		return body, nil
	}
	return nil, errors.New("image required: provide imageBase64 or imageUrl")
}

// SendProduct sends a catalog product card to a contact.
//
// IMPORTANT: this only builds and sends the message. The card body (image,
// title, price) is self-contained and renders, but the "View" action on the
// recipient's device asks WhatsApp for the catalog product identified by
// (BusinessOwnerJid, ProductId). That lookup needs the sending account to be a
// WhatsApp *Business* account with a catalog, and ProductId to be a real id from
// it. A regular (non-business) account, or a made-up/absent ProductId, will
// deliver the card successfully while "View" opens a broken product page. That
// is expected, not a send failure.
func (s *sendService) SendProduct(data *ProductStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if data.ProductId == "" {
		return nil, errors.New("productId is required")
	}
	if data.Title == "" {
		return nil, errors.New("title is required")
	}
	if data.Currency == "" {
		return nil, errors.New("currency is required")
	}

	// Product image: standard image upload (natively supported by whatsmeow).
	imgBytes, err := productImageBytes(data)
	if err != nil {
		return nil, err
	}
	uploaded, err := client.Upload(context.Background(), imgBytes, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload product image: %w", err)
	}
	productImage := &waE2E.ImageMessage{
		Mimetype:      proto.String("image/jpeg"),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
	}

	// Catalog owner: the instance itself by default.
	ownerJid := data.BusinessOwnerJid
	if ownerJid == "" {
		if client.Store == nil || client.Store.ID == nil {
			return nil, errors.New("instance not logged in")
		}
		ownerJid = client.Store.ID.ToNonAD().String()
	}

	snapshot := &waE2E.ProductMessage_ProductSnapshot{
		ProductImage:      productImage,
		ProductID:         proto.String(data.ProductId),
		Title:             proto.String(data.Title),
		CurrencyCode:      proto.String(data.Currency),
		PriceAmount1000:   proto.Int64(data.Price),
		ProductImageCount: proto.Uint32(1),
	}
	if data.Description != "" {
		snapshot.Description = proto.String(data.Description)
	}
	if data.RetailerId != "" {
		snapshot.RetailerID = proto.String(data.RetailerId)
	}
	if data.Url != "" {
		snapshot.URL = proto.String(data.Url)
	}

	productMsg := &waE2E.ProductMessage{
		Product:          snapshot,
		BusinessOwnerJID: proto.String(ownerJid),
	}
	if data.Body != "" {
		productMsg.Body = proto.String(data.Body)
	}
	if data.Footer != "" {
		productMsg.Footer = proto.String(data.Footer)
	}

	msg := &waE2E.Message{ProductMessage: productMsg}

	message, err := s.SendMessage(instance, msg, "ProductMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
	if err != nil {
		return nil, err
	}

	return message, nil
}
