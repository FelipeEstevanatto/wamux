package send_service

import (
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// participantMentionJID prefers the phone-number JID when available.
// Mentions require @s.whatsapp.net; participant.JID may be @lid.
func participantMentionJID(p types.GroupParticipant) string {
	if !p.PhoneNumber.IsEmpty() {
		return p.PhoneNumber.String()
	}
	return p.JID.String()
}

// mentionToken is how a JID is written in message text: "@" and the user part.
func mentionToken(jid string) string {
	user, _, _ := strings.Cut(jid, "@")
	user, _, _ = strings.Cut(user, ":")
	return "@" + user
}

// hasMentionToken finds token as a whole number, so "@551" is not found in "@5511".
func hasMentionToken(text, token string) bool {
	for rest := text; ; {
		at := strings.Index(rest, token)
		if at < 0 {
			return false
		}
		rest = rest[at+len(token):]
		if rest == "" || rest[0] < '0' || rest[0] > '9' {
			return true
		}
	}
}

// withMentionTokens adds the tokens the text does not have yet. WhatsApp only tags
// someone when the text has their "@number": with MentionedJID alone, nobody is
// tagged. The clients show the token as the contact's name.
func withMentionTokens(text string, mentionedJIDs []string) string {
	var missing []string
	for _, jid := range mentionedJIDs {
		if token := mentionToken(jid); !hasMentionToken(text, token) {
			missing = append(missing, token)
		}
	}
	if len(missing) == 0 {
		return text
	}
	tokens := strings.Join(missing, " ")
	if text == "" {
		return tokens
	}
	return text + "\n\n" + tokens
}

// writeMentionTokens puts the tokens in the text or caption, for the types that have
// one. The others have nowhere to show them, so nobody is tagged by those.
func writeMentionTokens(msg *waE2E.Message, mentionedJIDs []string) {
	switch {
	case msg.ExtendedTextMessage != nil:
		msg.ExtendedTextMessage.Text = proto.String(withMentionTokens(msg.ExtendedTextMessage.GetText(), mentionedJIDs))
	case msg.ImageMessage != nil:
		msg.ImageMessage.Caption = proto.String(withMentionTokens(msg.ImageMessage.GetCaption(), mentionedJIDs))
	case msg.VideoMessage != nil:
		msg.VideoMessage.Caption = proto.String(withMentionTokens(msg.VideoMessage.GetCaption(), mentionedJIDs))
	case msg.DocumentMessage != nil:
		msg.DocumentMessage.Caption = proto.String(withMentionTokens(msg.DocumentMessage.GetCaption(), mentionedJIDs))
	case msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage() != nil:
		doc := msg.DocumentWithCaptionMessage.Message.DocumentMessage
		doc.Caption = proto.String(withMentionTokens(doc.GetCaption(), mentionedJIDs))
	}
}

// setMessageMentionedJIDs writes ContextInfo.MentionedJID for the given message type.
// DocumentMessage may live under DocumentWithCaptionMessage when a caption is set.
func setMessageMentionedJIDs(msg *waE2E.Message, messageType string, mentionedJIDs []string) {
	if msg == nil || len(mentionedJIDs) == 0 {
		return
	}

	writeMentionTokens(msg, mentionedJIDs)

	switch messageType {
	case "ExtendedTextMessage":
		if msg.ExtendedTextMessage == nil {
			return
		}
		if msg.ExtendedTextMessage.ContextInfo == nil {
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "ImageMessage":
		if msg.ImageMessage == nil {
			return
		}
		if msg.ImageMessage.ContextInfo == nil {
			msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.ImageMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "VideoMessage":
		if msg.VideoMessage == nil {
			return
		}
		if msg.VideoMessage.ContextInfo == nil {
			msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.VideoMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "PtvMessage":
		if msg.PtvMessage == nil {
			return
		}
		if msg.PtvMessage.ContextInfo == nil {
			msg.PtvMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.PtvMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "AudioMessage":
		if msg.AudioMessage == nil {
			return
		}
		if msg.AudioMessage.ContextInfo == nil {
			msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.AudioMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "DocumentMessage":
		if msg.DocumentMessage != nil {
			if msg.DocumentMessage.ContextInfo == nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			msg.DocumentMessage.ContextInfo.MentionedJID = mentionedJIDs
		} else if msg.DocumentWithCaptionMessage != nil &&
			msg.DocumentWithCaptionMessage.Message != nil &&
			msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil {
			doc := msg.DocumentWithCaptionMessage.Message.DocumentMessage
			if doc.ContextInfo == nil {
				doc.ContextInfo = &waE2E.ContextInfo{}
			}
			doc.ContextInfo.MentionedJID = mentionedJIDs
		}
	case "PollCreationMessage":
		if msg.PollCreationMessage == nil {
			return
		}
		if msg.PollCreationMessage.ContextInfo == nil {
			msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.PollCreationMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "StickerMessage":
		if msg.StickerMessage == nil {
			return
		}
		if msg.StickerMessage.ContextInfo == nil {
			msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.StickerMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "LocationMessage":
		if msg.LocationMessage == nil {
			return
		}
		if msg.LocationMessage.ContextInfo == nil {
			msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.LocationMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "ContactMessage":
		if msg.ContactMessage == nil {
			return
		}
		if msg.ContactMessage.ContextInfo == nil {
			msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.ContactMessage.ContextInfo.MentionedJID = mentionedJIDs
	case "EventMessage":
		if msg.EventMessage == nil {
			return
		}
		if msg.EventMessage.ContextInfo == nil {
			msg.EventMessage.ContextInfo = &waE2E.ContextInfo{}
		}
		msg.EventMessage.ContextInfo.MentionedJID = mentionedJIDs
	}
}
