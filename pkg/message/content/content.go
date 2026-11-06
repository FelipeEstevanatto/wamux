// Package content turns a raw whatsmeow message into the small, stable summary
// the message-history readback API stores and returns: a coarse type, the
// visible text/caption and the id of the message it quotes (if any).
//
// It lives in its own package because both the inbound event handler
// (pkg/whatsmeow/service) and the outbound send path (pkg/sendMessage/service)
// need it, and neither should import the other.
package content

import "go.mau.fi/whatsmeow/proto/waE2E"

// Summary is the persisted view of a message.
type Summary struct {
	// Type is one of: text, image, video, audio, document, sticker, location,
	// contact, reaction, edit, delete, poll, poll_update, buttons_response,
	// list_response, interactive_response, buttons, list, interactive, unknown.
	Type string
	// Text is the visible body/caption/name, or the replacement text for edits.
	Text string
	// QuotedID is the id of the message this one quotes/replies to/reacts to,
	// when the message carries a context.
	QuotedID string
	// FileName is a document's name. FileSize is any attachment's size in bytes.
	FileName string
	FileSize int64
	// PageCount is a document's length, when the sender's client reports it.
	PageCount uint32
	// Thumbnail is the small JPEG preview WhatsApp carries in the message itself.
	Thumbnail []byte
	// Forwarded marks a message forwarded from another conversation.
	Forwarded bool
}

// Summarize extracts the summary from a waE2E message. It never panics on a nil
// message or a nil sub-field.
func Summarize(msg *waE2E.Message) Summary {
	if msg == nil {
		return Summary{Type: "unknown"}
	}

	msg = unwrap(msg)
	summary := summarize(msg)
	summary.FileName, summary.FileSize = fileOf(msg)
	summary.PageCount = msg.GetDocumentMessage().GetPageCount()
	summary.Thumbnail = thumbnailOf(msg)
	summary.Forwarded = contextOf(msg).GetIsForwarded()
	return summary
}

// unwrap returns the message inside the wrappers whatsmeow removes from received
// messages. What we send is still wrapped when it is recorded.
func unwrap(msg *waE2E.Message) *waE2E.Message {
	if inner := msg.GetDocumentWithCaptionMessage().GetMessage(); inner != nil {
		return inner
	}
	if inner := msg.GetViewOnceMessage().GetMessage(); inner != nil {
		return inner
	}
	return msg
}

func fileOf(msg *waE2E.Message) (string, int64) {
	switch {
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetFileName(), int64(msg.GetDocumentMessage().GetFileLength())
	case msg.GetImageMessage() != nil:
		return "", int64(msg.GetImageMessage().GetFileLength())
	case msg.GetVideoMessage() != nil:
		return "", int64(msg.GetVideoMessage().GetFileLength())
	case msg.GetAudioMessage() != nil:
		return "", int64(msg.GetAudioMessage().GetFileLength())
	case msg.GetStickerMessage() != nil:
		return "", int64(msg.GetStickerMessage().GetFileLength())
	}
	return "", 0
}

// maxThumbnailBytes keeps previews small enough to live in the message row and travel
// with the history. WhatsApp's own are a few KB.
const maxThumbnailBytes = 100 << 10

// Thumbnail returns a message's preview on its own, for callers that strip it from the
// message before summarizing it.
func Thumbnail(msg *waE2E.Message) []byte {
	if msg == nil {
		return nil
	}
	return thumbnailOf(unwrap(msg))
}

// thumbnailOf returns a preview for the attachments the chat shows as a card.
func thumbnailOf(msg *waE2E.Message) []byte {
	thumbnail := msg.GetDocumentMessage().GetJPEGThumbnail()
	if len(thumbnail) == 0 {
		thumbnail = msg.GetVideoMessage().GetJPEGThumbnail()
	}
	if len(thumbnail) > maxThumbnailBytes {
		return nil
	}
	return thumbnail
}

func contextOf(msg *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage().GetContextInfo()
	case msg.GetLocationMessage() != nil:
		return msg.GetLocationMessage().GetContextInfo()
	case msg.GetContactMessage() != nil:
		return msg.GetContactMessage().GetContextInfo()
	}
	return nil
}

func summarize(msg *waE2E.Message) Summary {
	// Deletes and edits are protocol messages that must be classified before
	// the generic text cases, because the edited content lives on the protocol
	// message, not on the top-level message.
	if p := msg.GetProtocolMessage(); p != nil {
		switch p.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			return Summary{Type: "delete", QuotedID: p.GetKey().GetID()}
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			quoted := ""
			if p.GetKey() != nil {
				quoted = p.GetKey().GetID()
			}
			return Summary{Type: "edit", Text: textOf(p.GetEditedMessage()), QuotedID: quoted}
		}
	}

	if r := msg.GetReactionMessage(); r != nil {
		quoted := ""
		if r.GetKey() != nil {
			quoted = r.GetKey().GetID()
		}
		return Summary{Type: "reaction", Text: r.GetText(), QuotedID: quoted}
	}

	if c := msg.GetConversation(); c != "" {
		return Summary{Type: "text", Text: c}
	}
	if e := msg.GetExtendedTextMessage(); e != nil {
		return Summary{Type: "text", Text: e.GetText(), QuotedID: quotedID(e.GetContextInfo())}
	}
	if img := msg.GetImageMessage(); img != nil {
		return Summary{Type: "image", Text: img.GetCaption(), QuotedID: quotedID(img.GetContextInfo())}
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return Summary{Type: "video", Text: vid.GetCaption(), QuotedID: quotedID(vid.GetContextInfo())}
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		return Summary{Type: "document", Text: doc.GetCaption(), QuotedID: quotedID(doc.GetContextInfo())}
	}
	if audio := msg.GetAudioMessage(); audio != nil {
		return Summary{Type: "audio", QuotedID: quotedID(audio.GetContextInfo())}
	}
	if sticker := msg.GetStickerMessage(); sticker != nil {
		return Summary{Type: "sticker", QuotedID: quotedID(sticker.GetContextInfo())}
	}
	if loc := msg.GetLocationMessage(); loc != nil {
		return Summary{Type: "location", Text: loc.GetName(), QuotedID: quotedID(loc.GetContextInfo())}
	}
	if c := msg.GetContactMessage(); c != nil {
		return Summary{Type: "contact", Text: c.GetDisplayName(), QuotedID: quotedID(c.GetContextInfo())}
	}
	if msg.GetContactsArrayMessage() != nil {
		return Summary{Type: "contact"}
	}

	switch {
	case msg.GetPollCreationMessage() != nil, msg.GetPollCreationMessageV2() != nil, msg.GetPollCreationMessageV3() != nil:
		return Summary{Type: "poll", Text: pollQuestion(msg)}
	case msg.GetEventMessage() != nil:
		return Summary{Type: "event", Text: msg.GetEventMessage().GetName()}
	case msg.GetProductMessage() != nil:
		return Summary{Type: "product", Text: msg.GetProductMessage().GetProduct().GetTitle()}
	case msg.GetPollUpdateMessage() != nil:
		return Summary{Type: "poll_update"}
	case msg.GetButtonsResponseMessage() != nil:
		return Summary{Type: "buttons_response", Text: msg.GetButtonsResponseMessage().GetSelectedDisplayText()}
	case msg.GetListResponseMessage() != nil:
		return Summary{Type: "list_response", Text: msg.GetListResponseMessage().GetTitle()}
	case msg.GetInteractiveResponseMessage() != nil:
		return Summary{Type: "interactive_response"}
	case msg.GetTemplateButtonReplyMessage() != nil:
		return Summary{Type: "buttons_response", Text: msg.GetTemplateButtonReplyMessage().GetSelectedDisplayText()}
	case msg.GetButtonsMessage() != nil:
		return Summary{Type: "buttons", Text: msg.GetButtonsMessage().GetContentText()}
	case msg.GetListMessage() != nil:
		return Summary{Type: "list", Text: msg.GetListMessage().GetDescription()}
	case msg.GetInteractiveMessage() != nil:
		return Summary{Type: "interactive", Text: msg.GetInteractiveMessage().GetBody().GetText()}
	}

	return Summary{Type: "unknown"}
}

func pollQuestion(msg *waE2E.Message) string {
	if name := msg.GetPollCreationMessage().GetName(); name != "" {
		return name
	}
	if name := msg.GetPollCreationMessageV2().GetName(); name != "" {
		return name
	}
	if name := msg.GetPollCreationMessageV3().GetName(); name != "" {
		return name
	}
	return ""
}

func quotedID(ctx *waE2E.ContextInfo) string {
	if ctx == nil {
		return ""
	}
	return ctx.GetStanzaID()
}

func textOf(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if c := msg.GetConversation(); c != "" {
		return c
	}
	if e := msg.GetExtendedTextMessage(); e != nil {
		return e.GetText()
	}
	// An edited media message changes its caption.
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	return msg.GetDocumentMessage().GetCaption()
}
