package message_service

import (
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

func TestWholeSecondPage(t *testing.T) {
	rows := func(ts ...string) []message_model.Message {
		out := make([]message_model.Message, len(ts))
		for i, v := range ts {
			out[i].Timestamp = v
		}
		return out
	}
	cases := []struct {
		name  string
		in    []message_model.Message
		limit int
		want  int
	}{
		{"under the limit", rows("3", "2"), 5, 2},
		{"clean cut", rows("3", "2", "1"), 2, 2},
		{"cut inside a second", rows("3", "2", "2", "2", "1"), 2, 4},
		{"all one second", rows("2", "2", "2"), 1, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(wholeSecondPage(tc.in, tc.limit)); got != tc.want {
				t.Fatalf("got %d rows, want %d", got, tc.want)
			}
		})
	}
}
