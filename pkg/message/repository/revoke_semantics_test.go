package message_repository

import (
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

const semChat = "c@s.whatsapp.net"

func storedRow(t *testing.T, repo MessageRepository, id string) *message_model.Message {
	t.Helper()
	row, err := repo.GetMessageByIDForInstance("sem", id)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return row
}

func richRow(id string) message_model.Message {
	row := contentRow(id, "secret text")
	row.MessageType = "document"
	row.MediaUrl = "/chat/media/" + id
	row.MediaMimetype = "application/pdf"
	row.FileName = "secret.pdf"
	row.FileSize = 10
	row.PageCount = 2
	row.FileThumbnail = []byte{1, 2}
	return row
}

func wantTombstone(t *testing.T, row *message_model.Message) {
	t.Helper()
	if row.MessageType != message_model.TypeDeleted || row.TextContent != "" || row.MediaUrl != "" ||
		row.MediaMimetype != "" || row.FileName != "" || row.FileSize != 0 || row.PageCount != 0 || len(row.FileThumbnail) != 0 {
		t.Fatalf("not a tombstone: %+v", row)
	}
}

func TestRevokeEmptiesTheMessageAndKeepsItsPlace(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	original := richRow("sem-revoke")
	if err := repo.InsertMessage(original); err != nil {
		t.Fatal(err)
	}

	revoke := contentRow("sem-revoke", "")
	revoke.Timestamp = "2026-10-02 09:00:00"
	applied, err := repo.MarkMessageDeleted(revoke, []string{semChat}, true)
	if err != nil || !applied {
		t.Fatalf("applied %v, err %v", applied, err)
	}

	row := storedRow(t, repo, "sem-revoke")
	wantTombstone(t, row)
	if row.Timestamp != original.Timestamp || row.ChatJid != semChat || row.Status != "Sent" {
		t.Errorf("moved: %+v", row)
	}
}

func TestLateOriginalDoesNotUndoARevoke(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	if _, err := repo.MarkMessageDeleted(contentRow("sem-late", ""), []string{semChat}, true); err != nil {
		t.Fatal(err)
	}

	if err := repo.InsertMessage(richRow("sem-late")); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertMessages([]message_model.Message{richRow("sem-late")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertHistoryMessages([]message_model.Message{richRow("sem-late")}); err != nil {
		t.Fatal(err)
	}

	wantTombstone(t, storedRow(t, repo, "sem-late"))
}

func TestReceiptStillMovesATombstone(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	if err := repo.InsertMessage(richRow("sem-receipt")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkMessageDeleted(contentRow("sem-receipt", ""), []string{semChat}, true); err != nil {
		t.Fatal(err)
	}

	if err := repo.InsertMessage(receiptRow("sem-receipt")); err != nil {
		t.Fatal(err)
	}

	row := storedRow(t, repo, "sem-receipt")
	wantTombstone(t, row)
	if row.Status != "Read" {
		t.Errorf("status = %s", row.Status)
	}
}

// A revoke is only a message: nothing stops a contact sending one for an id from
// another conversation.
func TestRevokeFromAnotherChatIsIgnored(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	if err := repo.InsertMessage(richRow("sem-foreign")); err != nil {
		t.Fatal(err)
	}

	revoke := contentRow("sem-foreign", "")
	revoke.ChatJid = "other@s.whatsapp.net"
	applied, err := repo.MarkMessageDeleted(revoke, []string{"other@s.whatsapp.net"}, true)
	if err != nil || applied {
		t.Fatalf("applied %v, err %v", applied, err)
	}
	if row := storedRow(t, repo, "sem-foreign"); row.TextContent != "secret text" {
		t.Fatalf("changed: %+v", row)
	}
}

func TestDirectChatSidesRevokeOnlyTheirOwnMessages(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	mine := richRow("sem-mine")
	mine.IsFromMe = true
	if err := repo.InsertMessage(mine); err != nil {
		t.Fatal(err)
	}

	fromContact := contentRow("sem-mine", "")
	applied, err := repo.MarkMessageDeleted(fromContact, []string{semChat}, true)
	if err != nil || applied {
		t.Fatalf("applied %v, err %v", applied, err)
	}

	// In a group the same revoke may be an admin's.
	applied, err = repo.MarkMessageDeleted(fromContact, []string{semChat}, false)
	if err != nil || !applied {
		t.Fatalf("group: applied %v, err %v", applied, err)
	}
}

func TestRevokeTakesTheEditsWithIt(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	edit := contentRow("sem-edit", "edited secret")
	edit.MessageType = "edit"
	edit.QuotedMessageID = "sem-edited"
	for _, row := range []message_model.Message{contentRow("sem-edited", "secret"), edit} {
		if err := repo.InsertMessage(row); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.MarkMessageDeleted(contentRow("sem-edited", ""), []string{semChat}, true); err != nil {
		t.Fatal(err)
	}

	if row := storedRow(t, repo, "sem-edit"); row.TextContent != "" || row.MessageType != "delete" {
		t.Fatalf("edit kept: %+v", row)
	}
}

func TestHistoryNeverChangesARowItFinds(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	live := richRow("sem-hist")
	live.Status = "Read"
	if err := repo.InsertMessage(live); err != nil {
		t.Fatal(err)
	}

	older := contentRow("sem-hist", "from history")
	if err := repo.InsertHistoryMessages([]message_model.Message{older, contentRow("sem-hist-new", "recovered")}); err != nil {
		t.Fatal(err)
	}

	row := storedRow(t, repo, "sem-hist")
	if row.TextContent != "secret text" || row.Status != "Read" || row.MediaUrl == "" {
		t.Fatalf("overwrote: %+v", row)
	}
	if row := storedRow(t, repo, "sem-hist-new"); row == nil || row.TextContent != "recovered" {
		t.Fatalf("not stored: %+v", row)
	}
}

func chatSummary(t *testing.T, repo MessageRepository, chat string) ChatSummary {
	t.Helper()
	chats, err := repo.ListChats("sem", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chats {
		if c.ChatJid == chat {
			return c
		}
	}
	t.Fatalf("no chat %s in %+v", chat, chats)
	return ChatSummary{}
}

func unreadRow(id, at string, fromMe bool, kind string) message_model.Message {
	row := contentRow(id, "text")
	row.ChatJid = "sem-unread@s.whatsapp.net"
	row.Timestamp = at
	row.IsFromMe = fromMe
	row.MessageType = kind
	return row
}

func TestUnreadCountsInboundMessagesAfterTheReadPosition(t *testing.T) {
	db := realDB(t)
	const chat = "sem-unread@s.whatsapp.net"
	t.Cleanup(func() { db.Exec("DELETE FROM chat_reads WHERE instance_id = 'sem'") })
	db.Exec("DELETE FROM chat_reads WHERE instance_id = 'sem'")
	repo := NewMessageRepository(db)

	rows := []message_model.Message{
		unreadRow("sem-u1", "2026-10-01 10:00:00", false, "text"),
		unreadRow("sem-u2", "2026-10-01 10:01:00", false, "image"),
		unreadRow("sem-u3", "2026-10-01 10:02:00", true, "text"),
		unreadRow("sem-u4", "2026-10-01 10:03:00", false, "reaction"),
		unreadRow("sem-u5", "2026-10-01 10:04:00", false, "edit"),
	}
	if err := repo.InsertMessages(rows); err != nil {
		t.Fatal(err)
	}

	if got := chatSummary(t, repo, chat); got.UnreadCount != 2 || got.LastReadAt != "" {
		t.Fatalf("a new chat: %+v", got)
	}

	if err := repo.MarkChatsRead("sem", []string{chat}, "2026-10-01 10:00:00"); err != nil {
		t.Fatal(err)
	}
	if got := chatSummary(t, repo, chat); got.UnreadCount != 1 || got.LastReadAt != "2026-10-01 10:00:00" {
		t.Fatalf("after reading the first: %+v", got)
	}

	if err := repo.MarkChatsRead("sem", []string{chat}, "2026-10-01 11:00:00"); err != nil {
		t.Fatal(err)
	}
	if got := chatSummary(t, repo, chat); got.UnreadCount != 0 {
		t.Fatalf("after reading all: %+v", got)
	}

	// A late or stale request never brings messages back as unread.
	if err := repo.MarkChatsRead("sem", []string{chat}, "2026-10-01 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if got := chatSummary(t, repo, chat); got.UnreadCount != 0 || got.LastReadAt != "2026-10-01 11:00:00" {
		t.Fatalf("moved back: %+v", got)
	}
}
