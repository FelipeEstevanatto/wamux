package whatsmeow_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
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

func TestHistorySyncSkipsWhatRetentionWouldDelete(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveHistorySync = true
	h.cli.config.MessageRetentionDays = 30

	recent := webMessage("RECENT", false, "x")
	recent.MessageTimestamp = proto.Uint64(uint64(time.Now().AddDate(0, 0, -29).Unix()))
	old := webMessage("OLD", false, "x")
	old.MessageTimestamp = proto.Uint64(uint64(time.Now().AddDate(0, 0, -31).Unix()))

	h.cli.processEvent(&events.HistorySync{Data: historyBatch(recent, old)})

	if rows := h.messages.history; len(rows) != 1 || rows[0].MessageID != "RECENT" {
		t.Fatalf("stored %+v", rows)
	}
}

func TestHistorySyncKeepsEverythingWithoutRetention(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveHistorySync = true
	h.cli.config.MessageRetentionDays = 0

	old := webMessage("OLD", false, "x")
	old.MessageTimestamp = proto.Uint64(uint64(time.Now().AddDate(-5, 0, 0).Unix()))
	h.cli.processEvent(&events.HistorySync{Data: historyBatch(old)})

	if len(h.messages.history) != 1 {
		t.Fatalf("stored %+v", h.messages.history)
	}
}

func TestHistorySyncSkipsStatusPosts(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveHistorySync = true
	batch := historyBatch(webMessage("POST", false, "x"))
	batch.Conversations[0].ID = proto.String("status@broadcast")

	h.cli.processEvent(&events.HistorySync{Data: batch})

	if len(h.messages.history) != 0 {
		t.Fatalf("stored %+v", h.messages.history)
	}
}

func TestRecoveredHistoryDoesNotShowAsUnread(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveHistorySync = true
	h.cli.config.MessageRetentionDays = 0

	older := webMessage("OLD", false, "x")
	older.MessageTimestamp = proto.Uint64(uint64(testTime.Add(-time.Hour).Unix()))
	h.cli.processEvent(&events.HistorySync{Data: historyBatch(older, webMessage("NEW", false, "x"))})

	want := testContact + "|" + testTime.Local().Format("2006-01-02 15:04:05")
	if got := h.messages.reads; len(got) != 1 || got[0] != want {
		t.Fatalf("reads %v, want %s", got, want)
	}
}

func TestChatReadOnThePhoneClearsTheBadge(t *testing.T) {
	read := func(value, fullSync bool) *events.MarkChatAsRead {
		return &events.MarkChatAsRead{
			JID:          mustJID(t, testContact),
			Timestamp:    testTime,
			Action:       &waSyncAction.MarkChatAsReadAction{Read: proto.Bool(value)},
			FromFullSync: fullSync,
		}
	}

	h := newEventHarness(t)
	h.cli.processEvent(read(true, false))
	want := testContact + "|" + testTime.Format("2006-01-02 15:04:05")
	if got := h.messages.reads; len(got) != 1 || got[0] != want {
		t.Fatalf("reads %v, want %s", got, want)
	}

	for name, evt := range map[string]*events.MarkChatAsRead{
		"marked unread": read(false, false),
		"old state":     read(true, true),
	} {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			h.cli.processEvent(evt)
			if len(h.messages.reads) != 0 {
				t.Fatalf("reads %v", h.messages.reads)
			}
		})
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

type flakyInsert struct{ failures, calls int }

func (f *flakyInsert) insert() error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("database restarting")
	}
	return nil
}

func TestInsertRetriesATemporaryFailure(t *testing.T) {
	delays := []time.Duration{0, 0}

	recovers := &flakyInsert{failures: 2}
	if err := retryInsert(recovers.insert, delays); err != nil || recovers.calls != 3 {
		t.Fatalf("err %v after %d calls", err, recovers.calls)
	}

	stuck := &flakyInsert{failures: 99}
	if err := retryInsert(stuck.insert, delays); err == nil || stuck.calls != 3 {
		t.Fatalf("err %v after %d calls", err, stuck.calls)
	}

	healthy := &flakyInsert{}
	if err := retryInsert(healthy.insert, delays); err != nil || healthy.calls != 1 {
		t.Fatalf("err %v after %d calls", err, healthy.calls)
	}
}

type lidRepo struct {
	message_repository.MessageRepository
	jids     []string
	rewrites []string
	failOn   string
}

func (r *lidRepo) LIDJids() ([]string, error) { return r.jids, nil }

func (r *lidRepo) RewriteJid(lid, phone, source string) (int64, error) {
	if lid == r.failOn {
		return 0, errors.New("down")
	}
	r.rewrites = append(r.rewrites, lid+">"+phone+">"+source)
	return 2, nil
}

func TestBackfillMovesLIDsWithAKnownPhone(t *testing.T) {
	known := func(lid types.JID) (types.JID, bool) {
		if lid.User == "111" {
			return types.NewJID("5511999990001", types.DefaultUserServer), true
		}
		return types.JID{}, false
	}
	repo := &lidRepo{jids: []string{"111@lid", "222@lid", "not a jid@@"}}

	rewritten, err := backfillLIDMessages(repo, known)
	if err != nil || rewritten != 2 {
		t.Fatalf("rewritten %d, err %v", rewritten, err)
	}
	want := "111@lid>5511999990001@s.whatsapp.net>5511999990001"
	if len(repo.rewrites) != 1 || repo.rewrites[0] != want {
		t.Fatalf("rewrites %v", repo.rewrites)
	}

	broken := &lidRepo{jids: []string{"111@lid"}, failOn: "111@lid"}
	if _, err := backfillLIDMessages(broken, known); err == nil {
		t.Fatal("expected the failure to be reported")
	}
}
