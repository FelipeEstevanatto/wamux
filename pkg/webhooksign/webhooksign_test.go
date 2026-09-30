package webhooksign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestDeriveEncryptionKeyIsStableAndFixedSize(t *testing.T) {
	a, err := DeriveEncryptionKey("some operator secret")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	b, err := DeriveEncryptionKey("some operator secret")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(a) != 32 {
		t.Fatalf("key length = %d, want 32", len(a))
	}
	if string(a) != string(b) {
		t.Fatal("derivation is not deterministic")
	}

	other, _ := DeriveEncryptionKey("some other secret")
	if string(a) == string(other) {
		t.Fatal("different secrets produced the same key")
	}
}

func TestDeriveEncryptionKeyRejectsEmpty(t *testing.T) {
	if _, err := DeriveEncryptionKey(""); err == nil {
		t.Fatal("expected an error for an empty secret")
	}
	if _, err := DeriveEncryptionKey("   "); err == nil {
		t.Fatal("expected an error for a blank secret")
	}
}

func TestGenerateKeyLengthAndUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(key) != 2*MinKeyLength {
			t.Fatalf("key %q has length %d, want %d", key, len(key), 2*MinKeyLength)
		}
		if seen[key] {
			t.Fatalf("duplicate key generated: %q", key)
		}
		seen[key] = true
	}
}

// Sign must match a hand-rolled HMAC-SHA256, because that is exactly what every
// consumer computes when verifying the header.
func TestSignMatchesStdlibHMAC(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte(`{"event":"Message","data":{"id":"abc"}}`)

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	got := Sign(key, payload)
	if got != want {
		t.Fatalf("Sign = %s, want %s", got, want)
	}
	if !Verify(key, payload, got) {
		t.Fatal("Verify rejected a valid signature")
	}
}

func TestVerifyRejectsWrongKeyTamperingAndGarbage(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	payload := []byte(`{"event":"Message"}`)
	sig := Sign(key, payload)

	if Verify([]byte("different-key-different-key-000"), payload, sig) {
		t.Fatal("signature verified under the wrong key")
	}
	if Verify(key, []byte(`{"event":"Tampered"}`), sig) {
		t.Fatal("signature verified over tampered payload")
	}
	if Verify(key, payload, "not-hex") {
		t.Fatal("malformed signature was accepted")
	}
	// The header is trimmed before decoding, so surrounding whitespace is OK.
	if !Verify(key, payload, "  "+sig+"\n") {
		t.Fatal("signature with surrounding whitespace was rejected")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	encKey, _ := DeriveEncryptionKey("unit-test-encryption-secret")
	plaintext := "a-per-instance-signing-key-that-is-32+chars"

	sealed, err := Encrypt(encKey, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if sealed == plaintext || strings.Contains(sealed, plaintext) {
		t.Fatal("ciphertext contains the plaintext")
	}

	got, err := Decrypt(encKey, sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("round trip = %q, want %q", got, plaintext)
	}
}

func TestEncryptUsesRandomNonce(t *testing.T) {
	encKey, _ := DeriveEncryptionKey("unit-test-encryption-secret")
	first, _ := Encrypt(encKey, "same plaintext")
	second, _ := Encrypt(encKey, "same plaintext")
	if first == second {
		t.Fatal("two encryptions produced identical ciphertext (nonce reuse)")
	}
}

func TestDecryptFailsWithWrongKeyOrTamperedCiphertext(t *testing.T) {
	encKey, _ := DeriveEncryptionKey("unit-test-encryption-secret")
	otherKey, _ := DeriveEncryptionKey("a-completely-different-secret")
	sealed, _ := Encrypt(encKey, "secret-value")

	if _, err := Decrypt(otherKey, sealed); err == nil {
		t.Fatal("decrypt succeeded with the wrong key")
	}

	tampered := []byte(sealed)
	tampered[len(tampered)-2] ^= 0x01
	if _, err := Decrypt(encKey, string(tampered)); err == nil {
		t.Fatal("decrypt succeeded on tampered ciphertext")
	}

	if _, err := Decrypt(encKey, "short"); err == nil {
		t.Fatal("decrypt succeeded on truncated ciphertext")
	}
	if _, err := Decrypt(encKey, "!!!not base64!!!"); err == nil {
		t.Fatal("decrypt succeeded on non-base64 input")
	}
}

func TestEncryptRejectsWrongKeySize(t *testing.T) {
	if _, err := Encrypt([]byte("too-short"), "x"); err == nil {
		t.Fatal("expected an error for a non-32-byte key")
	}
}
