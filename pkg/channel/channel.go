// Package channel defines the WhatsApp-neutral boundary between WaMux and the
// transports it supports. The public API keeps using the sendMessage service;
// a router maps those WhatsApp-shaped requests to either the existing Baileys
// implementation or the Cloud API adapter.
package channel

import (
	"errors"
	"fmt"
	"time"
)

// Kind is the instance integration value stored on instances.integration.
type Kind string

const (
	// KindWhatsAppWeb is the existing whatsmeow/WhatsApp Web implementation.
	KindWhatsAppWeb Kind = "WHATSAPP-BAILEYS"
	// KindCloud is the official Meta WhatsApp Cloud API.
	KindCloud Kind = "WHATSAPP-BUSINESS"
)

// Normalize returns the canonical integration, defaulting to Baileys for empty
// or legacy rows.
func Normalize(v string) Kind {
	switch Kind(v) {
	case KindCloud, KindWhatsAppWeb:
		return Kind(v)
	default:
		return KindWhatsAppWeb
	}
}

// Capabilities describes what a channel can do. A false value may mean either
// "Cloud has no equivalent" or "WaMux has not implemented a Cloud feature";
// CapabilityNote disambiguates the latter.
type Capabilities struct {
	Text            bool `json:"text"`
	Media           bool `json:"media"`
	Templates       bool `json:"templates"`
	Interactive     bool `json:"interactive"`
	Location        bool `json:"location"`
	Contacts        bool `json:"contacts"`
	Reactions       bool `json:"reactions"`
	BSUID           bool `json:"bsuid"`
	TypingIndicator bool `json:"typingIndicator"`
	Groups          bool `json:"groups"`
	Calls           bool `json:"calls"`
	Flows           bool `json:"flows"`
	Albums          bool `json:"albums"`
	Polls           bool `json:"polls"`
	Newsletters     bool `json:"newsletters"`
	Status          bool `json:"status"`
	Presence        bool `json:"presence"`
	History         bool `json:"history"`
}

// CapabilitiesFor returns the capabilities exposed by one channel.
func CapabilitiesFor(kind Kind) Capabilities {
	if Normalize(string(kind)) == KindCloud {
		return Capabilities{
			Text:            true,
			Media:           true,
			Templates:       true,
			Interactive:     true,
			Location:        true,
			Contacts:        true,
			Reactions:       true,
			BSUID:           true,
			TypingIndicator: true,
			// Cloud supports Groups, Calling and Flows, but this release does
			// not implement them. They stay false so the manager hides them.
			Groups:      false,
			Calls:       false,
			Flows:       false,
			Albums:      false,
			Polls:       false,
			Newsletters: false,
			Status:      false,
			Presence:    false,
			History:     false,
		}
	}
	return Capabilities{
		Text: true, Media: true, Templates: false, Interactive: true,
		Location: true, Contacts: true, Reactions: true, BSUID: false,
		TypingIndicator: false, Groups: true, Calls: true, Flows: false,
		Albums: true, Polls: true, Newsletters: true, Status: true,
		Presence: true, History: true,
	}
}

// CapabilityNote explains a false capability when the transport actually
// supports the feature but WaMux has deferred it.
func CapabilityNote(kind Kind, name string) string {
	if Normalize(string(kind)) != KindCloud {
		return ""
	}
	switch name {
	case "groups":
		return "Cloud Groups API exists for Official Business Accounts; not implemented in WaMux"
	case "calls":
		return "Cloud Calling API exists; not implemented in WaMux"
	case "flows":
		return "Cloud Flows exist; not implemented in WaMux"
	case "history":
		return "No general history API; the SMB history webhook is separate"
	}
	return ""
}

var (
	// ErrUnsupported means the transport has no equivalent for this operation.
	ErrUnsupported = errors.New("unsupported_on_channel")
	// ErrNotImplemented means the transport supports the operation but WaMux
	// has not built it yet.
	ErrNotImplemented = errors.New("not_implemented_on_channel")
)

// UnsupportedError identifies an operation with no channel equivalent.
type UnsupportedError struct {
	Channel Kind
	Op      string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("%s is not supported on %s", e.Op, e.Channel)
}
func (e *UnsupportedError) Unwrap() error { return ErrUnsupported }

// NotImplementedError identifies a Cloud-supported, WaMux-deferred operation.
type NotImplementedError struct {
	Channel Kind
	Op      string
}

func (e *NotImplementedError) Error() string {
	return fmt.Sprintf("%s is not implemented in WaMux for %s", e.Op, e.Channel)
}
func (e *NotImplementedError) Unwrap() error { return ErrNotImplemented }

// TextMessage is a transport-neutral outbound text.
type TextMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Body          string
	PreviewURL    bool
	ReplyTo       string
	OperationID   string
}

// MediaMessage is a transport-neutral outbound media message.
type MediaMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Kind          string
	URL           string
	Data          []byte
	Filename      string
	MIME          string
	Caption       string
	ReplyTo       string
	OperationID   string
}

// TemplateMessage is a transport-neutral template send.
type TemplateMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Name          string
	Language      string
	Components    any
	OperationID   string
}

// LocationMessage is a transport-neutral location send.
type LocationMessage struct {
	To        string
	Recipient string
	Name      string
	Latitude  float64
	Longitude float64
	Address   string
	ReplyTo   string
}

// ContactMessage is a transport-neutral contact send.
type ContactMessage struct {
	To        string
	Recipient string
	Contacts  []Contact
	ReplyTo   string
}

// Contact is a minimal contact payload shared by the router and Cloud adapter.
type Contact struct {
	Name   string
	Phones []string
	Emails []string
}

// ReactionMessage is a transport-neutral reaction.
type ReactionMessage struct {
	To        string
	Recipient string
	MessageID string
	Emoji     string
}

// InteractiveButton is one reply/CTA button.
type InteractiveButton struct {
	Type     string // reply, url, copy, call, pix
	Text     string
	ID       string
	URL      string
	Phone    string
	CopyCode string
}

// InteractiveListRow is one selectable list row.
type InteractiveListRow struct {
	Title       string
	Description string
	ID          string
}

// InteractiveSection groups list rows.
type InteractiveSection struct {
	Title string
	Rows  []InteractiveListRow
}

// InteractiveMessage covers the Cloud interactive subset WaMux supports.
type InteractiveMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Header        string
	Body          string
	Footer        string
	ButtonText    string
	Buttons       []InteractiveButton
	Sections      []InteractiveSection
	ReplyTo       string
	OperationID   string
}

// Result is the transport-neutral result of an outbound send.
type Result struct {
	ID          string
	Timestamp   time.Time
	Channel     Kind
	Recipient   string
	WAID        string
	OperationID string
	Raw         map[string]any
}

// CTAURLMessage is an interactive call-to-action link message.
type CTAURLMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Header        string
	Body          string
	Footer        string
	DisplayText   string
	URL           string
	ReplyTo       string
	OperationID   string
}

// ProductMessage is a single catalog product card.
type ProductMessage struct {
	To                string
	Recipient         string
	RecipientType     string
	CatalogID         string
	ProductRetailerID string
	Header            string
	Body              string
	Footer            string
	ReplyTo           string
	OperationID       string
}

// ProductSection groups products in a product-list message.
type ProductSection struct {
	Title    string
	Products []string
}

// ProductListMessage is a multi-product catalog message.
type ProductListMessage struct {
	To            string
	Recipient     string
	RecipientType string
	CatalogID     string
	Header        string
	Body          string
	Footer        string
	Sections      []ProductSection
	ReplyTo       string
	OperationID   string
}

// CatalogMessage opens the business catalog.
type CatalogMessage struct {
	To                         string
	Recipient                  string
	RecipientType              string
	Body                       string
	Footer                     string
	ThumbnailProductRetailerID string
	ReplyTo                    string
	OperationID                string
}

// ContactRequestMessage asks the customer to share a contact.
type ContactRequestMessage struct {
	To            string
	Recipient     string
	RecipientType string
	Body          string
	ReplyTo       string
	OperationID   string
}

// FlowMessage opens a WhatsApp Flow from an interactive message.
type FlowMessage struct {
	To                 string
	Recipient          string
	RecipientType      string
	Header             string
	Body               string
	Footer             string
	FlowID             string
	FlowToken          string
	FlowCTA            string
	FlowAction         string
	FlowActionPayload  any
	FlowMessageVersion string
	ReplyTo            string
	OperationID        string
}
