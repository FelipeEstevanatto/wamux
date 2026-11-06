package message_repository

import (
	"fmt"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

func searchRows(t *testing.T, repo MessageRepository, rows ...message_model.Message) {
	t.Helper()
	for _, row := range rows {
		if err := repo.InsertMessage(row); err != nil {
			t.Fatalf("insert %s: %v", row.MessageID, err)
		}
	}
}

func searchRow(id, chat, second, text string) message_model.Message {
	return message_model.Message{
		MessageID: "sem-" + id, InstanceId: "sem-search", ChatJid: chat, SenderJid: chat,
		Timestamp: "2026-10-01 12:00:" + second, Status: "Sent", MessageType: "text", TextContent: text,
	}
}

func searchIDs(rows []message_model.Message) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.MessageID)
	}
	return ids
}

func TestSearchMessagesOnRealPostgres(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	searchRows(t, repo,
		searchRow("a", "c1", "01", "The Invoice is ready"),
		searchRow("b", "c1", "02", "invoice paid"),
		searchRow("c", "c1", "03", "nothing here"),
		searchRow("d", "c2", "04", "invoice in another chat"),
		searchRow("e", "c1", "05", "100% done"),
		searchRow("f", "c1", "06", "1000 done"),
		searchRow("g", "c1", "07", "snake_case"),
		searchRow("h", "c1", "08", "snakeXcase"),
		searchRow("i", "c1", "09", `back\slash`),
		searchRow("j", "c1", "10", "Ação concluída"),
	)

	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"ignores case, newest first", "INVOICE", []string{"sem-b", "sem-a"}},
		{"percent is literal", "100%", []string{"sem-e"}},
		{"underscore is literal", "snake_case", []string{"sem-g"}},
		{"backslash is literal", `back\slash`, []string{"sem-i"}},
		{"ignores case in accented text", "AÇÃO", []string{"sem-j"}},
		{"no match", "absent", []string{}},
		{"quote cannot break out", `' OR 1=1 --`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.SearchMessages("sem-search", "c1", tc.text, "", 50)
			if err != nil {
				t.Fatalf("SearchMessages: %v", err)
			}
			if ids := searchIDs(got); fmt.Sprint(ids) != fmt.Sprint(tc.want) {
				t.Fatalf("got %v, want %v", ids, tc.want)
			}
		})
	}
}

func TestSearchMessagesStaysInItsInstance(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	other := searchRow("other", "c1", "01", "secret deal")
	other.InstanceId = "sem-other"
	searchRows(t, repo, other)

	got, err := repo.SearchMessages("sem-search", "c1", "secret", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("found another instance's message: %v", searchIDs(got))
	}
}

// A page must end on a whole second, or the next page skips what shares it.
func TestSearchMessagesPagesEveryRowOnce(t *testing.T) {
	repo := NewMessageRepository(realDB(t))
	var rows []message_model.Message
	for i := range 9 {
		rows = append(rows, searchRow(fmt.Sprintf("p%d", i), "c3", fmt.Sprintf("%02d", 20+i/3), "paged match"))
	}
	searchRows(t, repo, rows...)

	seen := map[string]int{}
	before := ""
	for page := 0; page < 10; page++ {
		got, err := repo.SearchMessages("sem-search", "c3", "match", before, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			break
		}
		for _, id := range searchIDs(got) {
			seen[id]++
		}
		before = got[len(got)-1].Timestamp
	}

	if len(seen) != 9 {
		t.Fatalf("saw %d of 9 rows: %v", len(seen), seen)
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("%s came back %d times", id, count)
		}
	}
}
