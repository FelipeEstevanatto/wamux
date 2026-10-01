package utils

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestNormalizeProxyProtocol(t *testing.T) {
	tests := []struct {
		protocol, port, want string
	}{
		{"socks", "9000", "socks5"},
		{"SOCKS", "9000", "socks5"},
		{"http", "8080", "http"},
		{"https", "8080", "https"},
		{"socks5", "8080", "socks5"},
		{"", "1080", "socks5"},
		{"", "2080", "socks5"},
		{"", "42500", "socks5"},
		{"", "41999", "http"},
		{"", "43001", "http"},
		{"", "8080", "http"},
		{"", "", "http"},
	}
	for _, tt := range tests {
		if got := NormalizeProxyProtocol(tt.protocol, tt.port); got != tt.want {
			t.Errorf("NormalizeProxyProtocol(%q,%q) = %q, want %q", tt.protocol, tt.port, got, tt.want)
		}
	}
}

func TestBuildProxyAddress(t *testing.T) {
	t.Run("requires host and port", func(t *testing.T) {
		if _, err := BuildProxyAddress("http", "", "8080", "", ""); err == nil {
			t.Fatal("expected error for empty host")
		}
		if _, err := BuildProxyAddress("http", "host", "", "", ""); err == nil {
			t.Fatal("expected error for empty port")
		}
	})

	t.Run("plain http", func(t *testing.T) {
		got, err := BuildProxyAddress("http", "proxy.example.com", "8080", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if got != "http://proxy.example.com:8080" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("socks5 inferred from port", func(t *testing.T) {
		got, err := BuildProxyAddress("", "proxy.example.com", "1080", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if got != "socks5://proxy.example.com:1080" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("credentials", func(t *testing.T) {
		got, err := BuildProxyAddress("http", "h", "1", "user", "pass")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "http://user:pass@h:1") {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("user without password", func(t *testing.T) {
		got, err := BuildProxyAddress("http", "h", "1", "user", "")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "http://user@h:1") {
			t.Fatalf("got %q", got)
		}
	})
}

func TestGetMessageType(t *testing.T) {
	text := "hi"
	mimetype := "image/png"
	tests := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"nil", nil, "ignore"},
		{"conversation", &waE2E.Message{Conversation: &text}, "text"},
		{"extended text", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{}}, "text"},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Mimetype: &mimetype}}, "image image/png"},
		{"reaction", &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: &text}}, "reaction"},
		{"reaction remove", &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{}}, "reaction remove"},
		{"contact", &waE2E.Message{ContactMessage: &waE2E.ContactMessage{}}, "contact"},
		{"unknown", &waE2E.Message{}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetMessageType(tt.msg); got != tt.want {
				t.Fatalf("GetMessageType = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("revoke without key is ignored", func(t *testing.T) {
		revoke := waE2E.ProtocolMessage_REVOKE
		msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: &revoke}}
		if got := GetMessageType(msg); got != "ignore" {
			t.Fatalf("got %q, want ignore", got)
		}
	})

	t.Run("edit", func(t *testing.T) {
		edit := waE2E.ProtocolMessage_MESSAGE_EDIT
		msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: &edit}}
		if got := GetMessageType(msg); got != "edit" {
			t.Fatalf("got %q, want edit", got)
		}
	})
}

func TestPrepareNumbersForWhatsAppCheck(t *testing.T) {
	format := true
	raw := false

	t.Run("normalizes to phone part with +", func(t *testing.T) {
		got, err := PrepareNumbersForWhatsAppCheck([]string{"11999999999"}, &format)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "+11999999999" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("keeps full group JID", func(t *testing.T) {
		got, _ := PrepareNumbersForWhatsAppCheck([]string{"120363123456789012@g.us"}, &format)
		if len(got) != 1 || got[0] != "120363123456789012@g.us" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("already-JID strips back to phone part", func(t *testing.T) {
		got, _ := PrepareNumbersForWhatsAppCheck([]string{"11999999999@s.whatsapp.net"}, &format)
		if len(got) != 1 || got[0] != "+11999999999" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("raw mode leaves input untouched", func(t *testing.T) {
		got, _ := PrepareNumbersForWhatsAppCheck([]string{"11999999999", "abc"}, &raw)
		if len(got) != 2 || got[0] != "11999999999" || got[1] != "abc" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("nil formatJid defaults to formatting", func(t *testing.T) {
		got, _ := PrepareNumbersForWhatsAppCheck([]string{"11999999999"}, nil)
		if len(got) != 1 || got[0] != "+11999999999" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("PrepareNumberForWhatsAppCheck returns the first", func(t *testing.T) {
		got, err := PrepareNumberForWhatsAppCheck("11999999999", true)
		if err != nil {
			t.Fatal(err)
		}
		if got != "+11999999999" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestGetObjectFindsNestedValue(t *testing.T) {
	payload := []byte(`{"a":{"b":{"caption":"found"}},"other":1}`)
	if got := GetObject(payload, "caption"); got != "found" {
		t.Fatalf("got %q, want found", got)
	}
	if got := GetObject(payload, "missing"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if got := GetObject([]byte("not json"), "caption"); got != "" {
		t.Fatalf("got %q, want empty on invalid JSON", got)
	}
}

func TestGenerateVC(t *testing.T) {
	vc := GenerateVC(VCardStruct{FullName: "Alice", Organization: "Acme", Phone: "5511999999999"})
	for _, want := range []string{"BEGIN:VCARD", "FN:Alice", "ORG:Acme", "waid=5511999999999", "END:VCARD"} {
		if !strings.Contains(vc, want) {
			t.Fatalf("vcard missing %q:\n%s", want, vc)
		}
	}
}

func TestTimestampToUnixInt(t *testing.T) {
	got, err := TimestampToUnixInt("2021-01-01 00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1609459200 {
		t.Fatalf("got %d, want 1609459200", got)
	}
	if _, err := TimestampToUnixInt("not a time"); err == nil {
		t.Fatal("expected error for malformed timestamp")
	}
}

func TestFindAndRandomString(t *testing.T) {
	if !Find([]string{"a", "b"}, "b") {
		t.Fatal("Find should locate b")
	}
	if Find([]string{"a"}, "z") {
		t.Fatal("Find should not locate z")
	}

	for _, n := range []int{0, 1, 16} {
		s := GenerateRandomString(n)
		if len(s) != n {
			t.Fatalf("GenerateRandomString(%d) length = %d", n, len(s))
		}
		for _, r := range s {
			if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", r) {
				t.Fatalf("unexpected char %q", r)
			}
		}
	}
}

func TestGetStringValue(t *testing.T) {
	s := "x"
	if GetStringValue(&s) != "x" {
		t.Fatal("expected x")
	}
	if GetStringValue(nil) != "" {
		t.Fatal("nil should give empty string")
	}
}

func TestWhatsAppGetUserAgent(t *testing.T) {
	if got := WhatsAppGetUserAgent("android"); got != waCompanionReg.DeviceProps_ANDROID_AMBIGUOUS {
		t.Fatalf("android = %v", got)
	}
	if got := WhatsAppGetUserAgent("IOS-PHONE"); got != waCompanionReg.DeviceProps_IOS_PHONE {
		t.Fatalf("case-insensitive lookup failed: %v", got)
	}
	if got := WhatsAppGetUserAgent("nonsense"); got != waCompanionReg.DeviceProps_UNKNOWN {
		t.Fatalf("unknown = %v", got)
	}
}

func TestUpdateUserInfo(t *testing.T) {
	v := Values{m: map[string]string{}}
	updated, ok := UpdateUserInfo(v, "name", "Alice").(Values)
	if !ok {
		t.Fatal("expected Values back")
	}
	if updated.m["name"] != "Alice" {
		t.Fatalf("value not set: %v", updated.m)
	}
	// A non-Values input is returned unchanged rather than panicking.
	if got := UpdateUserInfo(struct{}{}, "name", "x"); got == nil {
		t.Fatal("non-Values input should be returned as-is")
	}
}
