package tokencrypt

import "testing"

func TestHashIsDeterministic(t *testing.T) {
	c, err := New("a-secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Hash("tok") != c.Hash("tok") {
		t.Fatalf("hash not deterministic")
	}
	if c.Hash("tok") == c.Hash("tok2") {
		t.Fatalf("different tokens must hash differently")
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	c, _ := New("a-secret")
	enc, err := c.Encrypt("my-token-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "my-token-value" {
		t.Fatalf("ciphertext equals plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "my-token-value" {
		t.Fatalf("round trip = %q", dec)
	}
}

// The ciphertext must not leak the plaintext and must differ between calls (a
// random nonce per encryption is what makes it non-searchable — hence token_hash).
func TestEncryptIsNonDeterministic(t *testing.T) {
	c, _ := New("a-secret")
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatalf("two encryptions of the same token are identical; nonce is not random")
	}
}

func TestDifferentSecretsProduceDifferentHashes(t *testing.T) {
	c1, _ := New("secret-1")
	c2, _ := New("secret-2")
	if c1.Hash("tok") == c2.Hash("tok") {
		t.Fatalf("hash must depend on the key")
	}
}

func TestEmptySecretRejected(t *testing.T) {
	if _, err := New("  "); err == nil {
		t.Fatalf("expected an error for an empty secret")
	}
}

func TestLooksEncrypted(t *testing.T) {
	c, _ := New("a-secret")
	enc, _ := c.Encrypt("some-token")

	if !LooksEncrypted(enc) {
		t.Fatalf("ciphertext should look encrypted: %q", enc)
	}
	if LooksEncrypted("916cd701-fc04-4f4e-816d-a9bb1d53c84e") {
		t.Fatalf("a UUID token must not look encrypted")
	}
	if LooksEncrypted("short") {
		t.Fatalf("a short token must not look encrypted")
	}
}
