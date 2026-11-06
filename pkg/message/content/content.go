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
}

// Summarize extracts the summary from a waE2E message. It never panics on a nil
// message or a nil sub-field.
func Summarize(msg *waE2E.Message) Summary {
	if msg == nil {
		return Summary{Type: "unknown"}
	}

	// whatsmeow unwraps these for received messages, but what we send is still
	// wrapped when it is recorded.
	if inner := msg.GetDocumentWithCaptionMessage().GetMessage(); inner != nil {
		msg = inner
	} else if inner := msg.GetViewOnceMessage().GetMessage(); inner != nil {
		msg = inner
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
