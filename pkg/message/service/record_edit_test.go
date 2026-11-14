package message_service

import (
	"errors"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"go.mau.fi/whatsmeow/types"
)

type editRepo struct {
	message_repository.MessageRepository
	original *message_model.Message
	findErr  error
	stored   []message_model.Message
}

func (r *editRepo) GetMessageByIDForInstance(string, string) (*message_model.Message, error) {
	return r.original, r.findErr
}

func (r *editRepo) InsertMessage(m message_model.Message) error {
	r.stored = append(r.stored, m)
	return nil
}

func editService(t *testing.T, repo *editRepo) *messageService {
	t.Helper()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger("i").Close() })
	return &messageService{messageRepository: repo, loggerWrapper: logs}
}

func TestRecordEditStoresARowLinkedToTheOriginal(t *testing.T) {
	repo := &editRepo{original: &message_model.Message{Source: "1", ChatJid: "1@s.whatsapp.net", SenderJid: "me@s.whatsapp.net"}}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	chat := types.NewJID("1", types.DefaultUserServer)
	editService(t, repo).recordEdit("i", "ORIGINAL", "EDIT", "new text", at, chat, types.NewJID("me", types.DefaultUserServer))

	if len(repo.stored) != 1 {
		t.Fatalf("stored %d rows", len(repo.stored))
	}
	got := repo.stored[0]
	if got.MessageID != "EDIT" || got.QuotedMessageID != "ORIGINAL" || got.MessageType != "edit" || got.TextContent != "new text" {
		t.Errorf("stored %+v", got)
	}
	if got.ChatJid != "1@s.whatsapp.net" || got.Source != "1" || !got.IsFromMe || got.Timestamp != "2026-01-02 03:04:05" {
		t.Errorf("stored %+v", got)
	}
}

// An unreadable repository is a transient failure and must not invent a row.
// A missing row is the just-sent race: the edit is stored against the target id
// so the later original row is matched by quoted_message_id.
func TestRecordEditWithoutAnOriginal(t *testing.T) {
	chat := types.NewJID("1", types.DefaultUserServer)
	me := types.NewJID("me", types.DefaultUserServer)

	missing := &editRepo{}
	editService(t, missing).recordEdit("i", "ORIGINAL", "EDIT", "new text", time.Now(), chat, me)
	if len(missing.stored) != 1 || missing.stored[0].QuotedMessageID != "ORIGINAL" {
		t.Fatalf("missing original stored %+v", missing.stored)
	}

	failed := &editRepo{findErr: errors.New("down")}
	editService(t, failed).recordEdit("i", "ORIGINAL", "EDIT", "new text", time.Now(), chat, me)
	if len(failed.stored) != 0 {
		t.Fatalf("read failure stored %+v", failed.stored)
	}
}
