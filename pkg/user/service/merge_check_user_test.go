package user_service

import "testing"

// mergeCheckUserResults combines the formatJid=true attempt with the
// formatJid=false retry: a user the first pass failed to resolve but the retry
// found must be reported as found, while everything else keeps the original
// (authoritative) result.
func TestMergeCheckUserResults(t *testing.T) {
	u := &userService{}

	t.Run("nil retry returns original", func(t *testing.T) {
		original := &CheckUserCollection{Users: []User{{Query: "a", IsInWhatsapp: true}}}
		got := u.mergeCheckUserResults(original, nil)
		if got != original {
			t.Fatal("expected the original collection back")
		}
	})

	t.Run("retry result used when original missed", func(t *testing.T) {
		original := &CheckUserCollection{Users: []User{
			{Query: "a", IsInWhatsapp: false},
			{Query: "b", IsInWhatsapp: true},
		}}
		retry := &CheckUserCollection{Users: []User{
			{Query: "a", IsInWhatsapp: true, RemoteJID: "a@s.whatsapp.net"},
		}}

		got := u.mergeCheckUserResults(original, retry)
		if len(got.Users) != 2 {
			t.Fatalf("got %d users, want 2", len(got.Users))
		}
		if !got.Users[0].IsInWhatsapp || got.Users[0].RemoteJID != "a@s.whatsapp.net" {
			t.Fatalf("retry result not used: %+v", got.Users[0])
		}
		if got.Users[1].Query != "b" {
			t.Fatalf("order not preserved: %+v", got.Users)
		}
	})

	t.Run("original wins when it already found the user", func(t *testing.T) {
		original := &CheckUserCollection{Users: []User{{Query: "a", IsInWhatsapp: true, RemoteJID: "orig"}}}
		retry := &CheckUserCollection{Users: []User{{Query: "a", IsInWhatsapp: true, RemoteJID: "retry"}}}

		got := u.mergeCheckUserResults(original, retry)
		if got.Users[0].RemoteJID != "orig" {
			t.Fatalf("original should win: %+v", got.Users[0])
		}
	})

	t.Run("retry miss does not override", func(t *testing.T) {
		original := &CheckUserCollection{Users: []User{{Query: "a", IsInWhatsapp: false}}}
		retry := &CheckUserCollection{Users: []User{{Query: "a", IsInWhatsapp: false}}}

		got := u.mergeCheckUserResults(original, retry)
		if got.Users[0].IsInWhatsapp {
			t.Fatalf("miss must not become a hit: %+v", got.Users[0])
		}
	})

	t.Run("retry-only users are ignored", func(t *testing.T) {
		original := &CheckUserCollection{Users: []User{{Query: "a"}}}
		retry := &CheckUserCollection{Users: []User{{Query: "z", IsInWhatsapp: true}}}

		got := u.mergeCheckUserResults(original, retry)
		if len(got.Users) != 1 || got.Users[0].Query != "a" {
			t.Fatalf("expected only original users, got %+v", got.Users)
		}
	})
}
