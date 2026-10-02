package whatsmeow_service

import "testing"

func TestSplitSource(t *testing.T) {
	cases := []struct {
		in     string
		user   string
		server string
	}{
		{"5511999999999@s.whatsapp.net", "5511999999999", "s.whatsapp.net"},
		{"123456789012345678@g.us", "123456789012345678", "g.us"},
		{"987654321@lid", "987654321", "lid"},
		{"5511999999999:12@s.whatsapp.net", "5511999999999", "s.whatsapp.net"},
		{"120363000000000000", "120363000000000000", ""}, // bare group id
		{"", "", ""},
	}
	for _, c := range cases {
		user, server := splitSource(c.in)
		if user != c.user || server != c.server {
			t.Errorf("splitSource(%q) = (%q, %q), want (%q, %q)", c.in, user, server, c.user, c.server)
		}
	}
}

func TestIsAllDigits(t *testing.T) {
	yes := []string{"120363000000000000", "0", "5511999999999"}
	no := []string{"", "abc", "5511a999", "s.whatsapp.net", "123-456"}
	for _, s := range yes {
		if !isAllDigits(s) {
			t.Errorf("isAllDigits(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isAllDigits(s) {
			t.Errorf("isAllDigits(%q) = true, want false", s)
		}
	}
}
