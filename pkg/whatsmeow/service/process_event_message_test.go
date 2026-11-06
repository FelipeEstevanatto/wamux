package whatsmeow_service

import (
	"bytes"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// inbound is a text message from a contact, in the shape whatsmeow delivers it.
func inbound(t *testing.T, id, text string) *events.Message {
	t.Helper()
	contact := mustJID(t, testContact)
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: contact, Sender: contact},
			ID:            id,
			PushName:      "Contact",
			Timestamp:     testTime,
			Type:          "text",
		},
		Message: &waE2E.Message{Conversation: proto.String(text)},
	}
}

func inGroup(t *testing.T, evt *events.Message) *events.Message {
	t.Helper()
	evt.Info.Chat = mustJID(t, testGroup)
	evt.Info.IsGroup = true
	return evt
}

func TestInboundTextIsDeliveredStoredAndSentToTypebot(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inbound(t, "MSG1", "hello"))

	d := h.deliveries(t, 1)[0]
	if d.queue != testInstanceID+".message" {
		t.Errorf("queue = %q", d.queue)
	}
	got := d.event(t)
	wantField(t, got, "Message", "event")
	wantField(t, got, testInstanceID, "instanceId")
	wantField(t, got, "test", "instanceName")
	wantField(t, got, "MSG1", "data", "Info", "ID")
	wantField(t, got, "hello", "data", "Message", "conversation")

	stored := h.messages.stored()
	if len(stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(stored))
	}
	m := stored[0]
	if m.MessageID != "MSG1" || m.InstanceId != testInstanceID || m.Status != "Received" || m.IsFromMe {
		t.Errorf("stored %+v", m)
	}
	if m.ChatJid != testContact || m.Source != "5511999990001" || m.TextContent != "hello" {
		t.Errorf("stored %+v", m)
	}
	if m.Timestamp != "2026-01-02 03:04:05" {
		t.Errorf("timestamp = %q", m.Timestamp)
	}

	calls := h.svc.typebotCalls()
	if len(calls) != 1 || calls[0] != (typebotCall{testContact, "Contact", "hello", false}) {
		t.Errorf("typebot calls = %+v", calls)
	}
}

func TestExtendedTextReachesTypebot(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("linked")}}
	h.one(t, evt)
	if calls := h.svc.typebotCalls(); len(calls) != 1 || calls[0].text != "linked" {
		t.Errorf("typebot calls = %+v", calls)
	}
}

func TestMediaDoesNotReachTypebot(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("a caption")}}
	h.one(t, evt)
	if calls := h.svc.typebotCalls(); len(calls) != 0 {
		t.Errorf("typebot calls = %+v", calls)
	}
}

func TestOwnMessageFromAnotherDeviceIsStoredAsSent(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "from my phone")
	evt.Info.IsFromMe = true
	h.one(t, evt)
	if m := h.messages.stored()[0]; m.Status != "Sent" || !m.IsFromMe {
		t.Errorf("stored %+v", m)
	}
}

func TestHistorySavingCanBeTurnedOff(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveMessages = false
	h.one(t, inbound(t, "MSG1", "hello"))
	if n := len(h.messages.stored()); n != 0 {
		t.Errorf("stored %d messages", n)
	}
}

func TestIgnoreRules(t *testing.T) {
	broadcast := func(t *testing.T) *events.Message {
		evt := inbound(t, "MSG1", "status update")
		evt.Info.Chat = mustJID(t, "status@broadcast")
		return evt
	}
	group := func(t *testing.T) *events.Message { return inGroup(t, inbound(t, "MSG1", "hi all")) }

	cases := []struct {
		name        string
		setup       func(*eventHarness)
		event       func(*testing.T) *events.Message
		wantTypebot int
	}{
		{"instance ignores groups", func(h *eventHarness) { h.cli.Instance.IgnoreGroups = true }, group, 0},
		{"instance ignores status", func(h *eventHarness) { h.cli.Instance.IgnoreStatus = true }, broadcast, 0},
		// The global switches silence delivery only: Typebot has already run.
		{"global ignores groups", func(h *eventHarness) { h.cli.config.EventIgnoreGroup = true }, group, 1},
		{"global ignores status", func(h *eventHarness) { h.cli.config.EventIgnoreStatus = true }, broadcast, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			tc.setup(h)
			h.none(t, tc.event(t))
			if n := len(h.messages.stored()); n != 0 {
				t.Errorf("stored %d messages", n)
			}
			if n := len(h.svc.typebotCalls()); n != tc.wantTypebot {
				t.Errorf("typebot calls = %d, want %d", n, tc.wantTypebot)
			}
		})
	}
}

func TestIgnoreRulesLeaveOtherChatsAlone(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.IgnoreGroups = true
	h.cli.Instance.IgnoreStatus = true
	h.one(t, inbound(t, "MSG1", "direct message"))
}

func TestLIDSenderIsSwappedToPhoneNumber(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "hi")
	lid := mustJID(t, testContactLID)
	evt.Info.Chat, evt.Info.Sender = lid, lid
	evt.Info.SenderAlt = mustJID(t, testContact)

	got := h.one(t, evt)
	wantField(t, got, testContact, "data", "Info", "Chat")
	wantField(t, got, testContact, "data", "Info", "Sender")
	wantField(t, got, testContactLID, "data", "Info", "SenderAlt")
	if m := h.messages.stored()[0]; m.ChatJid != testContact || m.SenderJid != testContact {
		t.Errorf("stored %+v", m)
	}
}

// In a group the chat is the group, whatever form the sender takes.
func TestLIDSwapKeepsGroupChat(t *testing.T) {
	h := newEventHarness(t)
	evt := inGroup(t, inbound(t, "MSG1", "hi"))
	evt.Info.Sender = mustJID(t, testContactLID)
	evt.Info.SenderAlt = mustJID(t, testContact)

	got := h.one(t, evt)
	wantField(t, got, testGroup, "data", "Info", "Chat")
	wantField(t, got, testContact, "data", "Info", "Sender")
}

func TestDeviceSuffixIsRemovedFromSender(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "hi")
	evt.Info.Sender = mustJID(t, "5511999990001:12@s.whatsapp.net")
	got := h.one(t, evt)
	wantField(t, got, testContact, "data", "Info", "Sender")
}

func TestMessageSentFromPhoneIsFiledUnderTheRecipient(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "sent from my phone")
	own := mustJID(t, "5511999990000@s.whatsapp.net")
	evt.Info.Chat, evt.Info.Sender, evt.Info.IsFromMe = own, own, true
	evt.Info.DeviceSentMeta = &types.DeviceSentMeta{DestinationJID: testContact}

	got := h.one(t, evt)
	wantField(t, got, testContact, "data", "Info", "Chat")
	wantField(t, got, testContact, "data", "Recipient")
	wantField(t, got, testContact, "data", "Chat")
	if m := h.messages.stored()[0]; m.ChatJid != testContact {
		t.Errorf("stored under %q", m.ChatJid)
	}
}

func TestMessageSentToGroupFromPhoneStaysInTheGroup(t *testing.T) {
	h := newEventHarness(t)
	evt := inGroup(t, inbound(t, "MSG1", "hi"))
	evt.Info.IsFromMe = true
	// A different JID from the chat, so a wrongly rewritten chat would show.
	evt.Info.DeviceSentMeta = &types.DeviceSentMeta{DestinationJID: testContact}

	got := h.one(t, evt)
	wantField(t, got, testGroup, "data", "Info", "Chat")
	wantField(t, got, testContact, "data", "Recipient")
	if field(got, "data", "Chat") != nil {
		t.Errorf("data.Chat = %v, want it unset for a group", field(got, "data", "Chat"))
	}
}

func TestDisappearingTimerIsLearned(t *testing.T) {
	t.Run("from a settings change", func(t *testing.T) {
		h := newEventHarness(t)
		evt := inbound(t, "MSG1", "")
		evt.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
			Type:                waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum(),
			EphemeralExpiration: proto.Uint32(604800),
		}}
		h.cli.processEvent(evt)
		if s, ok := GetCachedChatEphemeral(testInstanceID, evt.Info.Chat); !ok || s != 604800 {
			t.Errorf("cached timer = %d, %v", s, ok)
		}
	})
	t.Run("from an ordinary message", func(t *testing.T) {
		h := newEventHarness(t)
		evt := inbound(t, "MSG1", "")
		evt.Info.Chat = mustJID(t, "5511999990002@s.whatsapp.net")
		evt.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String("hi"),
			ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(86400)},
		}}
		h.one(t, evt)
		if s, ok := GetCachedChatEphemeral(testInstanceID, evt.Info.Chat); !ok || s != 86400 {
			t.Errorf("cached timer = %d, %v", s, ok)
		}
	})
}

func TestEmptyMessageIsIgnored(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = nil
	h.none(t, evt)
	if n := len(h.messages.stored()); n != 0 {
		t.Errorf("stored %d messages", n)
	}
}

func TestEditAndRevokeAreFlagged(t *testing.T) {
	cases := []struct {
		name string
		kind waE2E.ProtocolMessage_Type
		flag string
		want string
	}{
		{"edit", waE2E.ProtocolMessage_MESSAGE_EDIT, "IsEdit", "edit"},
		{"revoke", waE2E.ProtocolMessage_REVOKE, "IsRevoke", "revoke"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			evt := inbound(t, "MSG1", "")
			evt.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				Type: tc.kind.Enum(),
				Key:  &waCommon.MessageKey{ID: proto.String("MSG0")},
			}}
			got := h.one(t, evt)
			wantField(t, got, true, "data", tc.flag)
			wantField(t, got, tc.want, "data", "messageType")
		})
	}
}

func TestUninterestingProtocolMessagesAreIgnored(t *testing.T) {
	cases := map[string]*waE2E.ProtocolMessage{
		"revoke without a key": {Type: waE2E.ProtocolMessage_REVOKE.Enum()},
		"history notification": {Type: waE2E.ProtocolMessage_HISTORY_SYNC_NOTIFICATION.Enum()},
		"unknown type":         {Type: waE2E.ProtocolMessage_Type(9999).Enum()},
	}
	for name, protocol := range cases {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			evt := inbound(t, "MSG1", "")
			evt.Message = &waE2E.Message{ProtocolMessage: protocol}
			h.none(t, evt)
			if n := len(h.messages.stored()); n != 0 {
				t.Errorf("stored %d messages", n)
			}
		})
	}
}

func TestQuotedReplyIsSurfaced(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG2", "")
	evt.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("reply"),
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String("MSG1"),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("original")},
		},
	}}
	got := h.one(t, evt)
	wantField(t, got, true, "data", "isQuoted")
	wantField(t, got, "MSG1", "data", "quoted", "stanzaID")
	wantField(t, got, "original", "data", "quoted", "quotedMessage", "conversation")
	if m := h.messages.stored()[0]; m.QuotedMessageID != "MSG1" {
		t.Errorf("stored quoted id %q", m.QuotedMessageID)
	}
}

func TestThumbnailsAndRawMessageAreRemoved(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{JPEGThumbnail: []byte("thumbnail")}}
	evt.RawMessage = &waE2E.Message{Conversation: proto.String("raw")}
	got := h.one(t, evt)
	if field(got, "data", "RawMessage") != nil {
		t.Error("RawMessage reached the payload")
	}
	if field(got, "data", "Message", "imageMessage", "JPEGThumbnail") != nil {
		t.Error("thumbnail reached the payload")
	}
}

func TestMediaIsNotFetchedUnlessWebhookFilesIsOn(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Mimetype: proto.String("image/jpeg")}}
	got := h.one(t, evt)
	if field(got, "data", "Message", "base64") != nil || field(got, "data", "Message", "mediaUrl") != nil {
		t.Errorf("media fields present: %v", field(got, "data", "Message"))
	}
}

// A failed download must cost the attachment, never the message.
func TestFailedDownloadStillDeliversTheMessage(t *testing.T) {
	media := map[string]*waE2E.Message{
		"image":    {ImageMessage: &waE2E.ImageMessage{}},
		"audio":    {AudioMessage: &waE2E.AudioMessage{}},
		"video":    {VideoMessage: &waE2E.VideoMessage{}},
		"document": {DocumentMessage: &waE2E.DocumentMessage{Mimetype: proto.String("application/pdf")}},
		"sticker":  {StickerMessage: &waE2E.StickerMessage{}},
		"child image": {AssociatedChildMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}},
		}},
	}
	for name, message := range media {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			h.cli.config.WebhookFiles = true
			evt := inbound(t, "MSG1", "")
			evt.Message = message
			got := h.one(t, evt)
			wantField(t, got, "MSG1", "data", "Info", "ID")
			if field(got, "data", "Message", "base64") != nil {
				t.Error("base64 present after a failed download")
			}
			m := h.messages.stored()[0]
			if m.MediaUrl != "" || m.MediaMimetype != "" {
				t.Errorf("stored media fields after a failed download: %+v", m)
			}
		})
	}
}

func TestGroupMessageCarriesCachedGroupData(t *testing.T) {
	h := newEventHarness(t)
	group := mustJID(t, testGroup)
	h.cli.groupInfoCache.Set(testInstanceID+"|"+group.String(), &types.GroupInfo{
		JID:       group,
		GroupName: types.GroupName{Name: "Team"},
	}, 0)
	got := h.one(t, inGroup(t, inbound(t, "MSG1", "hi")))
	wantField(t, got, "Team", "data", "groupData", "Name")
}

// Group metadata is an extra: failing to fetch it must not lose the message.
func TestGroupMessageSurvivesMissingGroupData(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, inGroup(t, inbound(t, "MSG1", "hi")))
	wantField(t, got, "MSG1", "data", "Info", "ID")
	if field(got, "data", "groupData") != nil {
		t.Error("unexpected groupData")
	}
}

func TestReferralIsSurfaced(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text: proto.String("from an ad"),
		ContextInfo: &waE2E.ContextInfo{ExternalAdReply: &waE2E.ContextInfo_ExternalAdReplyInfo{
			SourceID: proto.String("ad-1"),
		}},
	}}
	got := h.one(t, evt)
	if field(got, "data", "referral") == nil {
		t.Fatal("referral missing from the payload")
	}
}

func TestButtonClicksEmitASecondEvent(t *testing.T) {
	cases := []struct {
		name     string
		message  *waE2E.Message
		buttonID string
		text     string
		kind     string
	}{
		{
			name: "legacy buttons",
			message: &waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
				SelectedButtonID: proto.String("b1"),
				Response:         &waE2E.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: "Yes"},
			}},
			buttonID: "b1", text: "Yes", kind: "buttons_response",
		},
		{
			name: "native flow",
			message: &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
				InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
					NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
						Name:       proto.String("quick_reply"),
						ParamsJSON: proto.String(`{"id":"b2","display_text":"Maybe"}`),
					},
				},
			}},
			buttonID: "b2", text: "Maybe", kind: "native_flow_response",
		},
		{
			name: "template",
			message: &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{
				SelectedID:          proto.String("b3"),
				SelectedDisplayText: proto.String("No"),
			}},
			buttonID: "b3", text: "No", kind: "template_button_reply",
		},
		{
			name: "list",
			message: &waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{
				Title:             proto.String("Row"),
				SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{SelectedRowID: proto.String("b4")},
			}},
			buttonID: "b4", text: "Row", kind: "list_response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			evt := inbound(t, "MSG1", "")
			evt.Message = tc.message
			h.cli.processEvent(evt)

			ds := h.deliveries(t, 2)
			byEvent(t, ds, "Message")
			click := byEvent(t, ds, "ButtonClick")
			wantField(t, click, tc.buttonID, "data", "buttonId")
			wantField(t, click, tc.text, "data", "buttonText")
			wantField(t, click, tc.kind, "data", "type")
			wantField(t, click, testInstanceID, "instanceId")
		})
	}
}

func templateClick(t *testing.T) *events.Message {
	t.Helper()
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{
		SelectedID: proto.String("b1"),
	}}
	return evt
}

func TestButtonClickSaysWhoClicked(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(templateClick(t))
	click := byEvent(t, h.deliveries(t, 2), "ButtonClick")
	wantField(t, click, "5511999990001", "data", "phone")
	wantField(t, click, testContact, "data", "jid")
	wantField(t, click, "Contact", "data", "pushName")
	wantField(t, click, "MSG1", "data", "messageId")
	wantField(t, click, testContact, "data", "chat")
	wantField(t, click, false, "data", "fromMe")
	wantField(t, click, float64(testTime.Unix()), "data", "timestamp")
}

// The phone comes from the swapped sender, and the device suffix must not leak.
func TestButtonClickFromLIDContact(t *testing.T) {
	h := newEventHarness(t)
	evt := templateClick(t)
	lid := mustJID(t, testContactLID)
	evt.Info.Chat, evt.Info.Sender = lid, lid
	evt.Info.SenderAlt = mustJID(t, "5511999990001:3@s.whatsapp.net")
	h.cli.processEvent(evt)
	click := byEvent(t, h.deliveries(t, 2), "ButtonClick")
	wantField(t, click, "5511999990001", "data", "phone")
	wantField(t, click, testContact, "data", "jid")
	wantField(t, click, testContact, "data", "chat")
}

// A LID number is not a phone number, so it must not be presented as one.
func TestButtonClickWithUnknownPhone(t *testing.T) {
	h := newEventHarness(t)
	evt := templateClick(t)
	lid := mustJID(t, testContactLID)
	evt.Info.Chat, evt.Info.Sender = lid, lid
	h.cli.processEvent(evt)
	click := byEvent(t, h.deliveries(t, 2), "ButtonClick")
	wantField(t, click, "", "data", "phone")
	wantField(t, click, testContactLID, "data", "jid")
}

func TestButtonClickInGroupNamesChatAndClicker(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inGroup(t, templateClick(t)))
	click := byEvent(t, h.deliveries(t, 2), "ButtonClick")
	wantField(t, click, testGroup, "data", "chat")
	wantField(t, click, testContact, "data", "jid")
}

func TestMalformedNativeFlowParamsStillEmitAClick(t *testing.T) {
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String("quick_reply"),
				ParamsJSON: proto.String(`not json`),
			},
		},
	}}
	h.cli.processEvent(evt)
	click := byEvent(t, h.deliveries(t, 2), "ButtonClick")
	wantField(t, click, "", "data", "buttonId")
	wantField(t, click, "not json", "data", "extraData", "paramsJSON")
}

func TestGlobalQueuesReceiveMessages(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.AmqpGlobalEnabled = true
	h.one(t, inbound(t, "MSG1", "hi"))
	select {
	case got := <-h.svc.globals:
		if got != "Message" {
			t.Errorf("global event = %q", got)
		}
	case <-testTimeout():
		t.Fatal("nothing reached the global queues")
	}
}

// Through the whole inbound path, because the preview is stripped from the message
// before the event is built: reading it any later finds nothing.
func TestInboundDocumentKeepsItsPreviewAndPageCount(t *testing.T) {
	preview := []byte{0xff, 0xd8, 0xff, 0xe0}
	h := newEventHarness(t)
	evt := inbound(t, "MSG1", "")
	evt.Message = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		FileName:      proto.String("report.pdf"),
		FileLength:    proto.Uint64(2048),
		PageCount:     proto.Uint32(11),
		Mimetype:      proto.String("application/pdf"),
		JPEGThumbnail: preview,
	}}

	got := h.one(t, evt)

	m := h.messages.stored()[0]
	if !bytes.Equal(m.FileThumbnail, preview) {
		t.Errorf("stored preview = %v", m.FileThumbnail)
	}
	if m.FileName != "report.pdf" || m.FileSize != 2048 || m.PageCount != 11 {
		t.Errorf("stored %+v", m)
	}
	// The webhook stays as small as it was.
	if field(got, "data", "Message", "documentMessage", "JPEGThumbnail") != nil {
		t.Error("the preview reached the webhook")
	}
}
