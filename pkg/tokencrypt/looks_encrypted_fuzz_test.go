package tokencrypt

import "testing"

// FuzzLooksEncrypted exercises the startup-backfill discriminator on arbitrary
// stored token strings.
//
// Safety property: the decision is only ever used to choose "re-encrypt" vs
// "leave alone", and re-encrypting is idempotent — so a WRONG "looks encrypted"
// (a false negative for backfill) is the dangerous direction only if it would
// skip a plaintext row. The contract the callers rely on is simply that the
// function never panics and is deterministic.
func FuzzLooksEncrypted(f *testing.F) {
	for _, s := range []string{
		"",
		"short",
		"df16caad-d0d2-41b2-bec5-75b90048a0db", // a UUID-style token
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 42 chars, base64 alphabet
		"has symbol!",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", // long base64
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := LooksEncrypted(s)
		// Deterministic: the same input yields the same answer.
		if again := LooksEncrypted(s); again != got {
			t.Fatalf("LooksEncrypted(%q) is non-deterministic", s)
		}
		// A UUID-shaped token is never treated as ciphertext (the documented
		// rule that keeps plaintext UUIDs on the re-encrypt path).
		if looksLikeUUIDish(s) && LooksEncrypted(s) {
			t.Fatalf("LooksEncrypted(%q) claimed a UUID-shaped token is encrypted", s)
		}
	})
}
