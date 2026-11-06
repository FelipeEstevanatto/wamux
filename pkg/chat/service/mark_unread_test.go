package chat_service

import (
	"errors"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"go.mau.fi/whatsmeow/types"
)

type lastMessageRepo struct {
	message_repository.MessageRepository
	rows []message_model.Message
	err  error
}

func (r lastMessageRepo) ListMessages(string, string, string, int) ([]message_model.Message, error) {
	return r.rows, r.err
}

func TestLastMessageOfBuildsTheAppStateKey(t *testing.T) {
	row := message_model.Message{MessageID: "M", Timestamp: "2026-01-02 03:04:05", SenderJid: "2@s.whatsapp.net"}
	group := types.NewJID("1", types.GroupServer)
	direct := types.NewJID("1", types.DefaultUserServer)

	c := &chatService{messageRepository: lastMessageRepo{rows: []message_model.Message{row}}}
	at, key := c.lastMessageOf("i", group)
	if at.IsZero() || key.GetID() != "M" || key.GetRemoteJID() != group.String() || key.GetParticipant() != row.SenderJid {
		t.Fatalf("at %v key %v", at, key)
	}

	if _, key = c.lastMessageOf("i", direct); key.GetParticipant() != "" {
		t.Fatalf("direct chat has a participant: %v", key)
	}
}

func TestLastMessageOfIsEmptyWithoutHistory(t *testing.T) {
	for name, c := range map[string]*chatService{
		"no repository": {},
		"no rows":       {messageRepository: lastMessageRepo{}},
		"read failed":   {messageRepository: lastMessageRepo{err: errors.New("down")}},
		"bad timestamp": {messageRepository: lastMessageRepo{rows: []message_model.Message{{Timestamp: "?"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if at, key := c.lastMessageOf("i", types.NewJID("1", types.DefaultUserServer)); !at.IsZero() || key != nil {
				t.Fatalf("at %v key %v", at, key)
			}
		})
	}
}
