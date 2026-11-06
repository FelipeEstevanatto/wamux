package message_service

import (
	"errors"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
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

func TestAPIRevokeOfAnUnknownMessageStoresNothing(t *testing.T) {
	for name, base := range map[string]editRepo{
		"not stored":  {},
		"read failed": {findErr: errors.New("down")},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &revokeRepo{editRepo: base}
			svc := editService(t, &repo.editRepo)
			svc.messageRepository = repo

			svc.tombstoneRevoked("i", "M", types.NewJID("1", types.DefaultUserServer))

			if repo.row != nil {
				t.Fatalf("stored %+v", repo.row)
			}
		})
	}
}
