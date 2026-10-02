package utils

import (
	"strings"
	"testing"
)

// FuzzCreateJID feeds arbitrary operator input through the JID builder. The
// invariants a caller depends on:
//
//   - it never panics on any byte sequence;
//   - a successful result is a well-formed JID (non-empty, with one "@");
//   - it is idempotent — re-running it on its own output returns the same JID,
//     which matters because the value round-trips through the API;
//   - a phone-number JID's user part is an optional single leading "+" followed
//     by digits only. CreateJID deliberately keeps the "+" (the display form
//     that IsOnWhatsApp expects; CanonicalJID strips it for RAW protocol nodes),
//     so the invariant is "at most one, at the front" — never accumulated.
func FuzzCreateJID(f *testing.F) {
	seeds := []string{
		"", "+", "0", "5511999999999", "+55 (11) 99999-9999",
		"5511999999999@s.whatsapp.net", "120363000000000000", "120363000000000000@g.us",
		"120363000000000000-1610000000", "5215551234567", "5411123456789",
		"abc", "123456789@s.whatsapp.net", "123-456@g.us", "5511999999999:12",
		"status@broadcast", "123@lid", "123@newsletter",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		out, err := CreateJID(input)
		if err != nil {
			return
		}
		if out == "" {
			t.Fatalf("CreateJID(%q) returned an empty JID without error", input)
		}

		// A result always has a non-empty server part. The user part may be
		// empty: CreateJID passes a caller-supplied "…@server" through verbatim
		// (it does not re-validate it), and ParseJID — not CreateJID — is the
		// guard that rejects an empty user. So the invariant here is "has a
		// server", and the idempotence check below is what keeps it honest.
		at := strings.IndexByte(out, '@')
		if at < 0 || at == len(out)-1 {
			t.Fatalf("CreateJID(%q) = %q: no server part", input, out)
		}
		user, server := out[:at], out[at+1:]

		// Only the constructed path yields a phone number: a caller-supplied
		// "…@s.whatsapp.net" is returned verbatim (out == input), so its user
		// part is the caller's responsibility and is not constrained here.
		if server == "s.whatsapp.net" && out != input {
			// An optional single leading "+" then digits only, with no leftover
			// spaces or separators. The "+" must never appear anywhere else, or
			// it would accumulate.
			digits := strings.TrimPrefix(user, "+")
			if strings.Contains(digits, "+") {
				t.Fatalf("CreateJID(%q) = %q: misplaced/accumulated '+'", input, out)
			}
			for _, r := range digits {
				if r < '0' || r > '9' {
					t.Fatalf("CreateJID(%q) = %q: non-digit in a phone-number user part", input, out)
				}
			}
		}

		// Idempotence: the canonical form must map to itself.
		again, err := CreateJID(out)
		if err != nil {
			t.Fatalf("CreateJID(%q) failed on its own output %q: %v", input, out, err)
		}
		if again != out {
			t.Fatalf("CreateJID not idempotent: %q -> %q -> %q", input, out, again)
		}
	})
}

// FuzzParseJID asserts ParseJID never panics and, when it reports success, that
// the resolved JID has a non-empty user (or is a broadcast address, which is the
// one documented exception).
func FuzzParseJID(f *testing.F) {
	for _, s := range []string{
		"", "0", "5511999999999", "+5511999999999", "5511999999999@s.whatsapp.net",
		"status@broadcast", "120363000000000000@g.us", "abc", "@", "@s.whatsapp.net",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		jid, ok := ParseJID(input)
		if !ok {
			return
		}
		if jid.User == "" && !strings.Contains(input, "@broadcast") {
			t.Fatalf("ParseJID(%q) reported success with an empty user: %+v", input, jid)
		}
	})
}
