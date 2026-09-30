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
	if repo.instance != "inst-9" || repo.limit != 25 {
		t.Fatalf("instance/limit = %q/%d", repo.instance, repo.limit)
	}
}
