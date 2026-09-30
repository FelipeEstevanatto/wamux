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
	// list_response, interactive_response, unknown.
	Type string
	// Text is the visible body/caption/name, or the replacement text for edits.
	Text string
	// QuotedID is the id of the message this one quotes/replies to/reacts to,
	// when the message carries a context.
	QuotedID string
}

// Summarize extracts the summary from a waE2E message. It never panics on a nil
// message or a nil sub-field.
func Summarize(msg *waE2E.Message) Summary {
	if msg == nil {
		return Summary{Type: "unknown"}
	}

	// Deletes and edits are protocol messages that must be classified before
	// the generic text cases, because the edited content lives on the protocol
	// message, not on the top-level message.
	if p := msg.GetProtocolMessage(); p != nil {
		switch p.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			return Summary{Type: "delete"}
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
		return Summary{Type: "poll"}
	case msg.GetPollUpdateMessage() != nil:
		return Summary{Type: "poll_update"}
	case msg.GetButtonsResponseMessage() != nil:
		return Summary{Type: "buttons_response", Text: msg.GetButtonsResponseMessage().GetSelectedDisplayText()}
	case msg.GetListResponseMessage() != nil:
		return Summary{Type: "list_response", Text: msg.GetListResponseMessage().GetTitle()}
	case msg.GetInteractiveResponseMessage() != nil:
		return Summary{Type: "interactive_response"}
	}

	return Summary{Type: "unknown"}
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
	return msg.GetExtendedTextMessage().GetText()
}
