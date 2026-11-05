package message_repository

import (
	"fmt"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

func TestStatusRankOrder(t *testing.T) {
	if !(statusRank("Sent") == statusRank("Received") && statusRank("Sent") < statusRank("Delivered") && statusRank("Delivered") < statusRank("Read")) {
		t.Fatal("delivery states are out of order")
	}
}

func TestCollapseDuplicates(t *testing.T) {
	rows := []message_model.Message{
		{InstanceId: "i", MessageID: "a", Status: "Read"},
		{InstanceId: "i", MessageID: "b", Status: "Sent"},
		{InstanceId: "i", MessageID: "a", Status: "Delivered", TextContent: "latest"},
		{InstanceId: "other", MessageID: "a", Status: "Sent"},
	}
	got := collapseDuplicates(rows)
	if len(got) != 3 {
		t.Fatalf("got %d rows: %+v", len(got), got)
	}
	if got[0].Status != "Read" || got[0].TextContent != "latest" {
		t.Errorf("merged row = %+v, want the latest row at the furthest status", got[0])
	}
	if got[2].InstanceId != "other" {
		t.Error("an id from another instance is a different message")
	}
}

func statusOf(t *testing.T, repo MessageRepository, id string) *message_model.Message {
	t.Helper()
	m, err := repo.GetMessageByIDForInstance("sem", id)
	if err != nil || m == nil {
		t.Fatalf("read %s: %v, %v", id, m, err)
	}
	return m
}

func statusRow(id, status string) message_model.Message {
	return message_model.Message{MessageID: id, InstanceId: "sem", Status: status}
}

func TestLateReceiptDoesNotMoveStatusBack(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	id := "sem-order-receipts"
	for _, m := range []message_model.Message{contentRow(id, "hi"), statusRow(id, "Read"), statusRow(id, "Delivered")} {
		if err := repo.InsertMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	if got := statusOf(t, repo, id); got.Status != "Read" {
		t.Fatalf("status = %q, want Read", got.Status)
	}
}

// A send records "Sent" after the call returns, by which time a receipt may have
// landed. The body must still be written.
func TestContentRowAfterReceiptKeepsStatusAndWritesBody(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	id := "sem-order-content"
	if err := repo.InsertMessage(statusRow(id, "Delivered")); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertMessages([]message_model.Message{contentRow(id, "the body")}); err != nil {
		t.Fatal(err)
	}
	got := statusOf(t, repo, id)
	if got.Status != "Delivered" || got.TextContent != "the body" {
		t.Fatalf("got status %q text %q", got.Status, got.TextContent)
	}
}

func TestStatusStillMovesForward(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	id := "sem-order-forward"
	for _, m := range []message_model.Message{contentRow(id, "hi"), statusRow(id, "Delivered"), statusRow(id, "Read")} {
		if err := repo.InsertMessages([]message_model.Message{m}); err != nil {
			t.Fatal(err)
		}
	}
	if got := statusOf(t, repo, id); got.Status != "Read" {
		t.Fatalf("status = %q, want Read", got.Status)
	}
}

// Before, this failed the whole batch: Postgres will not upsert one row twice.
func TestBatchWithRepeatedIDSucceeds(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	id := "sem-order-dup"
	if err := repo.InsertMessage(contentRow(id, "hi")); err != nil {
		t.Fatal(err)
	}
	err := repo.InsertMessages([]message_model.Message{statusRow(id, "Read"), statusRow(id, "Delivered")})
	if err != nil {
		t.Fatalf("batch failed: %v", err)
	}
	if got := statusOf(t, repo, id); got.Status != "Read" {
		t.Fatalf("status = %q, want Read", got.Status)
	}
}

// Walking the history with the last timestamp as the cursor must return every
// message exactly once, including a burst sharing one second across a page break.
func TestPaginationKeepsMessagesSharingASecond(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	const chat = "sem-page@s.whatsapp.net"

	want := map[string]bool{}
	add := func(id, ts string) {
		row := contentRow("sem-page-"+id, id)
		row.ChatJid, row.Timestamp = chat, ts
		if err := repo.InsertMessage(row); err != nil {
			t.Fatal(err)
		}
		want[row.MessageID] = true
	}
	for i := 0; i < 3; i++ {
		add(fmt.Sprintf("new%d", i), "2026-10-01 12:00:03")
	}
	for i := 0; i < 7; i++ {
		add(fmt.Sprintf("burst%d", i), "2026-10-01 12:00:02")
	}
	for i := 0; i < 3; i++ {
		add(fmt.Sprintf("old%d", i), "2026-10-01 12:00:01")
	}

	got := map[string]int{}
	before := ""
	for page := 0; page < 20; page++ {
		rows, err := repo.ListMessages("sem", chat, before, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			got[r.MessageID]++
		}
		before = rows[len(rows)-1].Timestamp
	}
	for id := range want {
		if got[id] != 1 {
			t.Errorf("%s returned %d times", id, got[id])
		}
	}
}
