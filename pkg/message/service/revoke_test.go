package message_service

import (
	"context"
	"errors"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow/types"
)

type revokeRepo struct {
	editRepo
	row    *message_model.Message
	chats  []string
	direct bool
}

func (r *revokeRepo) MarkMessageDeleted(row message_model.Message, chats []string, direct bool) (bool, error) {
	r.row, r.chats, r.direct = &row, chats, direct
	return true, nil
}

func TestAPIRevokeTombstonesTheStoredMessage(t *testing.T) {
	const chat = "1@s.whatsapp.net"
	for name, tc := range map[string]struct {
		server string
		direct bool
	}{
		"direct": {types.DefaultUserServer, true},
		"group":  {types.GroupServer, false},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &revokeRepo{editRepo: editRepo{original: &message_model.Message{MessageID: "M", ChatJid: chat, IsFromMe: true}}}
			svc := editService(t, &repo.editRepo)
			svc.messageRepository = repo

			svc.tombstoneRevoked("i", "M", types.NewJID("1", tc.server))

			if repo.row == nil || repo.row.MessageID != "M" || repo.direct != tc.direct {
				t.Fatalf("row %+v direct %v", repo.row, repo.direct)
			}
			if len(repo.chats) == 0 || repo.chats[0] != chat {
				t.Fatalf("chats %v", repo.chats)
			}
		})
	}
}

// A just-sent message may not have its row yet. The delete must still leave a
// tombstone, or the async insert would make the deleted message visible again.
// A read failure, on the other hand, must not invent one.
func TestAPIRevokeOfAnUnknownMessage(t *testing.T) {
	t.Run("not stored yet", func(t *testing.T) {
		repo := &revokeRepo{}
		svc := editService(t, &repo.editRepo)
		svc.messageRepository = repo

		svc.tombstoneRevoked("i", "M", types.NewJID("1", types.DefaultUserServer))

		if repo.row == nil || repo.row.MessageID != "M" || repo.row.ChatJid != "1@s.whatsapp.net" {
			t.Fatalf("stored %+v", repo.row)
		}
	})

	t.Run("read failed", func(t *testing.T) {
		repo := &revokeRepo{editRepo: editRepo{findErr: errors.New("down")}}
		svc := editService(t, &repo.editRepo)
		svc.messageRepository = repo

		svc.tombstoneRevoked("i", "M", types.NewJID("1", types.DefaultUserServer))

		if repo.row != nil {
			t.Fatalf("stored %+v", repo.row)
		}
	})
}

type lidResolver struct {
	whatsmeow_service.WhatsmeowService
	alt types.JID
}

func (r lidResolver) SharedAlternateChatJID(context.Context, types.JID) (types.JID, bool) {
	return r.alt, !r.alt.IsEmpty()
}

func TestOfflineInstanceReadsBothFormsOfAChat(t *testing.T) {
	lid := types.NewJID("111", types.HiddenUserServer)
	phone := types.NewJID("5511999990001", types.DefaultUserServer)
	svc := &messageService{whatsmeowService: lidResolver{alt: lid}}

	got := svc.chatJidVariants("i", phone.String())
	if len(got) != 2 || got[1] != lid.String() {
		t.Fatalf("variants %v", got)
	}

	svc.whatsmeowService = lidResolver{alt: phone}
	merged := svc.mergeChatSummaries("i", []message_repository.ChatSummary{
		{ChatJid: phone.String(), Timestamp: "2026-01-02", MessageCount: 2, UnreadCount: 1, LastReadAt: "2026-01-01"},
		{ChatJid: lid.String(), Timestamp: "2026-01-01", MessageCount: 3, UnreadCount: 2, LastReadAt: "2026-01-03"},
	})
	if len(merged) != 1 {
		t.Fatalf("merged %+v", merged)
	}
	if m := merged[0]; m.MessageCount != 5 || m.UnreadCount != 3 || m.LastReadAt != "2026-01-03" || m.Timestamp != "2026-01-02" {
		t.Fatalf("merged %+v", m)
	}
}

func TestChatStaysAsIsWithoutAMapping(t *testing.T) {
	jid := "5511999990001@s.whatsapp.net"
	svc := &messageService{whatsmeowService: lidResolver{}}
	if got := svc.chatJidVariants("i", jid); len(got) != 1 || got[0] != jid {
		t.Fatalf("variants %v", got)
	}
	if got := (&messageService{}).chatJidVariants("i", jid); len(got) != 1 {
		t.Fatalf("no service: %v", got)
	}
}
