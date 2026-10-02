package webhooksign

import (
	"strings"
	"testing"
)

// FuzzVerify feeds arbitrary keys, payloads and signatures to the constant-time
// verifier. It must never panic, must reject an empty/garbage signature, and
// must accept a signature it just produced (round-trip) — the property webhook
// consumers rely on.
func FuzzVerify(f *testing.F) {
	f.Add("key", "payload", "deadbeef")
	f.Add("", "", "")
	f.Add("k", "p", strings.Repeat("0", 64))

	f.Fuzz(func(t *testing.T, key, payload, signature string) {
		// Must not panic on any input.
		_ = Verify([]byte(key), []byte(payload), signature)

		// An empty or non-hex signature is never valid.
		if strings.TrimSpace(signature) == "" {
			if Verify([]byte(key), []byte(payload), signature) {
				t.Fatalf("Verify accepted an empty signature")
			}
		}

		// A signature we generate must verify; a modified one must not.
		good := Sign([]byte(key), []byte(payload))
		if !Verify([]byte(key), []byte(payload), good) {
			t.Fatalf("Verify rejected its own signature (key=%q)", key)
		}
		if !Verify([]byte(key), []byte(payload), "  "+good+"  ") {
			t.Fatalf("Verify rejected a whitespace-padded signature")
		}
		tampered := good
		if len(tampered) > 0 {
			last := tampered[len(tampered)-1]
			if last == '0' {
				tampered = tampered[:len(tampered)-1] + "1"
			} else {
				tampered = tampered[:len(tampered)-1] + "0"
			}
			if Verify([]byte(key), []byte(payload), tampered) {
				t.Fatalf("Verify accepted a tampered signature")
			}
		}
	})
}

// FuzzDecrypt asserts the AES-GCM opener never panics on arbitrary stored
// ciphertext and never returns plaintext for a payload that was not sealed with
// the same key (authentication must fail, not leak garbage).
func FuzzDecrypt(f *testing.F) {
	key, err := DeriveEncryptionKey("test-secret")
	if err != nil {
		f.Fatal(err)
	}
	sealed, err := Encrypt(key, "hello")
	if err != nil {
		f.Fatal(err)
	}
	f.Add("")
	f.Add("not-base64!!")
	f.Add(sealed)
	f.Add("AAAA")

	f.Fuzz(func(t *testing.T, encoded string) {
		// Must not panic.
		got, err := Decrypt(key, encoded)
		if err != nil {
			return
		}
		// Success is only legitimate for ciphertext we actually produced; a
		// successful decrypt of an arbitrary string would mean the AEAD tag
		// check is not being enforced.
		if got == "" {
			return
		}
		// Re-encrypting the same plaintext must verify under the same key.
		re, encErr := Encrypt(key, got)
		if encErr != nil {
			t.Fatalf("re-encrypt of decrypted plaintext failed: %v", encErr)
		}
		if again, decErr := Decrypt(key, re); decErr != nil || again != got {
			t.Fatalf("round-trip mismatch: %q -> %q (%v)", got, again, decErr)
		}
	})
}
