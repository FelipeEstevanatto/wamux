package whatsmeow_service

import (
	"os"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
)

const otherContact = "5511999990002@s.whatsapp.net"

// realStorageHarness runs events through processEvent into a real repository, so the
// chat scope is tested as the event handler builds it. Needs EVO_BENCH_POSTGRES_DSN.
func realStorageHarness(t *testing.T) (*eventHarness, message_repository.MessageRepository) {
	t.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN to run against real storage")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&message_model.Message{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clean := func() { db.Exec("DELETE FROM messages WHERE instance_id = ?", testInstanceID) }
	clean()
	t.Cleanup(func() {
		clean()
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	repo := message_repository.NewMessageRepository(db)
	h := newEventHarness(t)
	h.cli.messageRepository = repo
	return h, repo
}

func fromContact(t *testing.T, evt *events.Message, contact string) *events.Message {
	t.Helper()
	jid := mustJID(t, contact)
	evt.Info.Chat, evt.Info.Sender = jid, jid
	return evt
}

func storedText(t *testing.T, repo message_repository.MessageRepository, id string) (string, string) {
	t.Helper()
	row, err := repo.GetMessageByIDForInstance(testInstanceID, id)
	if err != nil || row == nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return row.MessageType, row.TextContent
}

func TestForgedRevokeFromAnotherContactChangesNothing(t *testing.T) {
	h, repo := realStorageHarness(t)
	h.cli.processEvent(inbound(t, "VICTIM", "keep me"))

	h.cli.processEvent(fromContact(t, revokeOf(t, "FORGED", "VICTIM"), otherContact))

	if kind, text := storedText(t, repo, "VICTIM"); kind != "text" || text != "keep me" {
		t.Fatalf("a stranger's revoke changed the message: %s %q", kind, text)
	}
}

func TestContactCannotRevokeOurMessage(t *testing.T) {
	h, repo := realStorageHarness(t)
	mine := inbound(t, "MINE", "keep me")
	mine.Info.IsFromMe = true
	h.cli.processEvent(mine)

	h.cli.processEvent(revokeOf(t, "FORGED", "MINE"))

	if kind, text := storedText(t, repo, "MINE"); kind != "text" || text != "keep me" {
		t.Fatalf("the contact revoked our message: %s %q", kind, text)
	}
}

func TestRealRevokeRemovesTheMessageAndItsEdit(t *testing.T) {
	h, repo := realStorageHarness(t)
	h.cli.processEvent(inbound(t, "ORIGINAL", "first secret"))
	edit := inbound(t, "EDIT", "")
	edit.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String("ORIGINAL")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("second secret")},
	}}
	h.cli.processEvent(edit)
	if _, text := storedText(t, repo, "EDIT"); text != "second secret" {
		t.Fatalf("the edit was not stored: %q", text)
	}

	h.cli.processEvent(revokeOf(t, "REVOKE", "ORIGINAL"))

	if kind, text := storedText(t, repo, "ORIGINAL"); kind != message_model.TypeDeleted || text != "" {
		t.Errorf("original: %s %q", kind, text)
	}
	if _, text := storedText(t, repo, "EDIT"); text != "" {
		t.Errorf("the edit still reads %q", text)
	}
}
