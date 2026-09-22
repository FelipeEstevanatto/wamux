package webhook_producer

import (
	"reflect"
	"testing"
)

func TestSplitWebhookURLs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"disabled", "disabled", nil},
		{"single", "https://a.example/hook", []string{"https://a.example/hook"}},
		{
			"newline comma semicolon",
			"https://a.example/hook\nhttps://b.example/hook, https://c.example/hook;https://d.example/hook",
			[]string{"https://a.example/hook", "https://b.example/hook", "https://c.example/hook", "https://d.example/hook"},
		},
		{
			"json array",
			`["https://a.example/hook","https://b.example/hook"]`,
			[]string{"https://a.example/hook", "https://b.example/hook"},
		},
		{
			"duplicates and blanks dropped",
			"https://a.example/hook\n\nhttps://a.example/hook,https://b.example/hook,",
			[]string{"https://a.example/hook", "https://b.example/hook"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitWebhookURLs(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitWebhookURLs(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}
