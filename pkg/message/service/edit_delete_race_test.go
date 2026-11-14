package message_service

import (
	"testing"
	"time"

	config "github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"go.mau.fi/whatsmeow/types"
)

type raceRepo struct {
	message_repository.MessageRepository
	original *message_model.Message
	deleted  *message_model.Message
	inserted []message_model.Message
}

func (r *raceRepo) GetMessageByIDForInstance(_, _ string) (*message_model.Message, error) {
	return r.original, nil
}

func (r *raceRepo) MarkMessageDeleted(row message_model.Message, _ []string, _ bool) (bool, error) {
	r.deleted = &row
	return true, nil
}

func (r *raceRepo) InsertMessage(m message_model.Message) error {
	r.inserted = append(r.inserted, m)
	return nil
}

func raceService(t *testing.T, repo *raceRepo) *messageService {
	t.Helper()
	cfg := &config.Config{LogDirectory: t.TempDir()}
	logs := logger_wrapper.NewLoggerManager(cfg)
	t.Cleanup(func() { _ = logs.GetLogger("i").Close() })
	return &messageService{messageRepository: repo, loggerWrapper: logs}
}

// The outbound row is written by a goroutine after the send returns. A delete
// that arrives before that write must still create the local tombstone.
func TestDeleteBeforePersistWritesTombstone(t *testing.T) {
	repo := &raceRepo{} // original not persisted yet
	svc := raceService(t, repo)

	svc.tombstoneRevoked("i", "msg-1", types.NewJID("5511999999999", types.DefaultUserServer))

	if repo.deleted == nil {
		t.Fatal("no tombstone was written for a just-sent message")
	}
	if repo.deleted.MessageID != "msg-1" {
		t.Fatalf("tombstone target = %q", repo.deleted.MessageID)
	}
}

// Same race for an edit: without the original row, the edit used to be dropped
// and the async original later showed the old text.
func TestEditBeforePersistRecordsEdit(t *testing.T) {
	repo := &raceRepo{}
	svc := raceService(t, repo)
	chat := types.NewJID("5511999999999", types.DefaultUserServer)

	svc.recordEdit("i", "msg-1", "edit-1", "new text", time.Now(), chat, types.NewJID("5511888888888", types.DefaultUserServer))

	if len(repo.inserted) != 1 {
		t.Fatalf("inserted %d rows, want 1", len(repo.inserted))
	}
	row := repo.inserted[0]
	if row.MessageType != "edit" || row.QuotedMessageID != "msg-1" || row.TextContent != "new text" {
		t.Fatalf("edit row = %+v", row)
	}
	if row.ChatJid != chat.String() || !row.IsFromMe {
		t.Fatalf("edit row chat/fromMe = %q/%v", row.ChatJid, row.IsFromMe)
	}
}

func TestValidateEditTarget(t *testing.T) {
	now := time.Now()
	chat := "5511999999999@s.whatsapp.net"
	fresh := &message_model.Message{
		ChatJid: chat, IsFromMe: true, MessageType: "text",
		Timestamp: now.Add(-time.Minute).Format("2006-01-02 15:04:05"),
	}
	if err := validateEditTarget(fresh, []string{chat}, now); err != nil {
		t.Fatalf("fresh edit: %v", err)
	}

	old := *fresh
	old.Timestamp = now.Add(-16 * time.Minute).Format("2006-01-02 15:04:05")
	if err := validateEditTarget(&old, []string{chat}, now); err != ErrEditExpired {
		t.Fatalf("old edit = %v, want ErrEditExpired", err)
	}

	other := *fresh
	other.IsFromMe = false
	if err := validateEditTarget(&other, []string{chat}, now); err != ErrEditRestricted {
		t.Fatalf("inbound edit = %v, want ErrEditRestricted", err)
	}

	deleted := *fresh
	deleted.MessageType = message_model.TypeDeleted
	if err := validateEditTarget(&deleted, []string{chat}, now); err != ErrEditRestricted {
		t.Fatalf("deleted edit = %v, want ErrEditRestricted", err)
	}

	if err := validateEditTarget(fresh, []string{"5511888888888@s.whatsapp.net"}, now); err != ErrEditRestricted {
		t.Fatalf("wrong chat edit = %v, want ErrEditRestricted", err)
	}
}
