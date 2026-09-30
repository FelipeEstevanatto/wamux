package message_service

import (
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
)

type recordingRepo struct {
	message_repository.MessageRepository
	instance, chat, before string
	limit                  int
	messages               []message_model.Message
	chats                  []message_repository.ChatSummary
}

func (r *recordingRepo) ListMessages(instanceId, chatJid, before string, limit int) ([]message_model.Message, error) {
	r.instance, r.chat, r.before, r.limit = instanceId, chatJid, before, limit
	return r.messages, nil
}

func (r *recordingRepo) ListChats(instanceId string, limit int) ([]message_repository.ChatSummary, error) {
	r.instance, r.limit = instanceId, limit
	return r.chats, nil
}

func TestGetHistoryNormalizesPhoneToCanonicalJID(t *testing.T) {
	repo := &recordingRepo{messages: []message_model.Message{{MessageID: "m1", TextContent: "hi"}}}
	svc := &messageService{messageRepository: repo}
	instance := &instance_model.Instance{Id: "inst-1"}

	got, err := svc.GetHistory(&HistoryQuery{Chat: "5511999999999", Limit: 10, Before: " 2026-05-09 10:00:00 "}, instance)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(got) != 1 || got[0].MessageID != "m1" {
		t.Fatalf("messages = %+v", got)
	}
	if repo.chat != "5511999999999@s.whatsapp.net" {
		t.Fatalf("chat = %q, want canonical JID", repo.chat)
	}
	if repo.instance != "inst-1" || repo.limit != 10 {
		t.Fatalf("instance/limit = %q/%d", repo.instance, repo.limit)
	}
	if repo.before != "2026-05-09 10:00:00" {
		t.Fatalf("before = %q, want trimmed", repo.before)
	}
}

func TestGetHistoryKeepsGroupJID(t *testing.T) {
	repo := &recordingRepo{}
	svc := &messageService{messageRepository: repo}

	if _, err := svc.GetHistory(&HistoryQuery{Chat: "123456789@g.us"}, &instance_model.Instance{Id: "i"}); err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if repo.chat != "123456789@g.us" {
		t.Fatalf("chat = %q, want the group JID unchanged", repo.chat)
	}
}

func TestGetHistoryRejectsEmptyChat(t *testing.T) {
	repo := &recordingRepo{}
	svc := &messageService{messageRepository: repo}

	if _, err := svc.GetHistory(&HistoryQuery{Chat: "  "}, &instance_model.Instance{Id: "i"}); err == nil {
		t.Fatal("expected an error for an empty chat")
	}
	if repo.chat != "" {
		t.Fatal("repository must not be queried for an invalid chat")
	}
}

func TestListChatsPassesInstanceAndLimit(t *testing.T) {
	repo := &recordingRepo{chats: []message_repository.ChatSummary{{ChatJid: "a@s.whatsapp.net", MessageCount: 2}}}
	svc := &messageService{messageRepository: repo}

	got, err := svc.ListChats(&instance_model.Instance{Id: "inst-9"}, 25)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(got) != 1 || got[0].ChatJid != "a@s.whatsapp.net" {
		t.Fatalf("chats = %+v", got)
	}
	// ListChats overfetches (4x) so LID/PN merges still fill the page.
	if repo.instance != "inst-9" || repo.limit != 100 {
		t.Fatalf("instance/limit = %q/%d (want overfetch 100)", repo.instance, repo.limit)
	}
}

// With no client (no LID store), a device suffixed chat still collapses to the
// bare JID, and duplicate conversations are merged with summed counts.
func TestMergeChatSummariesCollapsesDeviceAndSorts(t *testing.T) {
	svc := &messageService{} // nil clientPointer is handled
	out := svc.mergeChatSummaries("i", []message_repository.ChatSummary{
		{ChatJid: "a@s.whatsapp.net", Timestamp: "2026-05-09 10:00:00", MessageCount: 1},
		{ChatJid: "a:5@s.whatsapp.net", Timestamp: "2026-05-09 11:00:00", MessageCount: 2, MessageID: "m2"},
		{ChatJid: "b@s.whatsapp.net", Timestamp: "2026-05-09 12:00:00", MessageCount: 5},
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 merged chats, got %d: %+v", len(out), out)
	}
	if out[0].ChatJid != "b@s.whatsapp.net" {
		t.Fatalf("newest chat should be first, got %+v", out[0])
	}
	if out[1].ChatJid != "a@s.whatsapp.net" || out[1].MessageCount != 3 {
		t.Fatalf("merged chat = %+v, want a@s.whatsapp.net with count 3", out[1])
	}
	if out[1].MessageID != "m2" {
		t.Fatalf("merged chat should keep the newest message, got %q", out[1].MessageID)
	}
}

func TestMergeMessagesByIDDeduplicatesAndSorts(t *testing.T) {
	msg := func(id, ts string) message_model.Message {
		return message_model.Message{MessageID: id, Timestamp: ts}
	}
	out := mergeMessagesByID([]message_model.Message{
		msg("m1", "2026-05-09 10:00:00"),
		msg("m2", "2026-05-09 11:00:00"),
		msg("m1", "2026-05-09 10:00:00"),
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	if out[0].MessageID != "m2" {
		t.Fatalf("newest first, got %+v", out)
	}
}
