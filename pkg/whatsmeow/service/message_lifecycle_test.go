package whatsmeow_service

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func revokeOf(t *testing.T, id, target string) *events.Message {
	t.Helper()
	evt := inbound(t, id, "")
	evt.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String(target)},
	}}
	return evt
}

func TestInboundRevokeTombstonesTheOriginal(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(revokeOf(t, "REVOKE1", "TARGET"))

	got := h.messages.revoked
	if len(got) != 1 {
		t.Fatalf("revoked %d times", len(got))
	}
	if got[0].row.MessageID != "TARGET" || !got[0].direct || got[0].row.IsFromMe {
		t.Errorf("call %+v", got[0])
	}
	if len(got[0].chats) == 0 || got[0].chats[0] != testContact {
		t.Errorf("chats %v", got[0].chats)
	}

	stored := h.messages.stored()
	if len(stored) != 1 || stored[0].MessageType != "delete" || stored[0].QuotedMessageID != "TARGET" {
		t.Errorf("revoke row %+v", stored)
	}
}

func TestGroupRevokeIsNotLimitedToTheSender(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inGroup(t, revokeOf(t, "REVOKE1", "TARGET")))

	if got := h.messages.revoked; len(got) != 1 || got[0].direct {
		t.Fatalf("calls %+v", got)
	}
}

func TestRevokeOnAnotherDeviceClearsTheRetryCache(t *testing.T) {
	h := newEventHarness(t)
	contact := mustJID(t, testContact)
	internals := h.cli.WAClient.DangerousInternals()
	sent := &waE2E.Message{Conversation: proto.String("secret")}
	if err := internals.AddRecentMessage(context.Background(), contact, "TARGET", sent, nil); err != nil {
		t.Fatal(err)
	}

	if internals.GetRecentMessage(contact, "TARGET").IsEmpty() {
		t.Fatal("the cache should hold the message before the revoke")
	}

	evt := revokeOf(t, "REVOKE1", "TARGET")
	evt.Info.IsFromMe = true
	h.cli.processEvent(evt)

	if !internals.GetRecentMessage(contact, "TARGET").IsEmpty() {
		t.Fatal("a retry receipt would resend the deleted message")
	}
}

func TestRepeatedMessageFiresOnce(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inbound(t, "MSG1", "hello"))
	h.cli.processEvent(inbound(t, "MSG1", "hello"))

	h.deliveries(t, 1)
	if got := h.svc.typebotCalls(); len(got) != 1 {
		t.Errorf("typebot called %d times", len(got))
	}
}

func TestSameIdInAnotherChatIsNotARepeat(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inbound(t, "MSG1", "hello"))
	h.cli.processEvent(inGroup(t, inbound(t, "MSG1", "hello")))

	h.deliveries(t, 2)
}

func historyBatch(messages ...*waWeb.WebMessageInfo) *waHistorySync.HistorySync {
	conversation := &waHistorySync.Conversation{ID: proto.String(testContact)}
	for _, msg := range messages {
		conversation.Messages = append(conversation.Messages, &waHistorySync.HistorySyncMsg{Message: msg})
	}
	return &waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{conversation},
	}
}

func webMessage(id string, fromMe bool, text string) *waWeb.WebMessageInfo {
	return &waWeb.WebMessageInfo{
		Key:              &waCommon.MessageKey{ID: proto.String(id), FromMe: proto.Bool(fromMe), RemoteJID: proto.String(testContact)},
		MessageTimestamp: proto.Uint64(uint64(testTime.Unix())),
		Message:          &waE2E.Message{Conversation: proto.String(text)},
	}
}

func TestHistorySyncIsStored(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveHistorySync = true

	read := webMessage("OUT", true, "sent earlier")
	read.Status = waWeb.WebMessageInfo_READ.Enum()
	stub := webMessage("STUB", false, "")
	stub.Message = nil
	revoke := webMessage("REV", false, "")
	revoke.Message = revokeOf(t, "REV", "IN").Message

	h.cli.processEvent(&events.HistorySync{Data: historyBatch(webMessage("IN", false, "received earlier"), read, stub, revoke)})

	rows := h.messages.history
	if len(rows) != 2 {
		t.Fatalf("stored %+v", rows)
	}
	in, out := rows[0], rows[1]
	if in.MessageID != "IN" || in.TextContent != "received earlier" || in.Status != "Received" || in.ChatJid != testContact || in.SenderJid != testContact {
		t.Errorf("inbound %+v", in)
	}
	if in.Timestamp != testTime.Local().Format("2006-01-02 15:04:05") {
		t.Errorf("timestamp %s", in.Timestamp)
	}
	if out.MessageID != "OUT" || !out.IsFromMe || out.Status != "Read" || out.SenderJid != mustJID(t, testOwnJID).ToNonAD().String() {
		t.Errorf("outbound %+v", out)
	}
}

func TestHistorySyncIsNotStoredWhenTurnedOff(t *testing.T) {
	for name, set := range map[string]func(h *eventHarness){
		"history sync off": func(h *eventHarness) { h.cli.config.DatabaseSaveHistorySync = false },
		"messages off": func(h *eventHarness) {
			h.cli.config.DatabaseSaveHistorySync = true
			h.cli.config.DatabaseSaveMessages = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			set(h)
			h.cli.processEvent(&events.HistorySync{Data: historyBatch(webMessage("IN", false, "x"))})
			if len(h.messages.history) != 0 {
				t.Fatalf("stored %+v", h.messages.history)
			}
		})
	}
}
