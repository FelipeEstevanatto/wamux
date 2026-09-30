// Package webhooksign implements HMAC-SHA256 signing of outbound webhook
// payloads and the AES-256-GCM encryption used to keep per-instance signing
// keys at rest.
//
// The design mirrors what most webhook consumers expect (and what WuzAPI
// ships): every HTTP webhook delivery carries an "x-hmac-signature" header
// holding the lowercase hex HMAC-SHA256 of the raw request body under a key
// that is private to the instance (or a process-global fallback). Consumers
// recompute the digest and compare it constant-time before trusting the body.
//
// The package is intentionally dependency-free so it can be unit-tested
// without a database or a running server.
package webhooksign

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SignatureHeader is the request header carrying the hex HMAC-SHA256 of the
// body. HTTP header names are case-insensitive; this spelling matches the
// documented contract.
const SignatureHeader = "x-hmac-signature"

// MinKeyLength is the shortest HMAC key the API accepts. 32 bytes (256 bits)
// matches the HMAC-SHA256 output size and gives a comfortable security margin.
// Keys longer than this are hashed down to the digest size internally.
const MinKeyLength = 32

// ErrEmptySecret is returned when DeriveEncryptionKey is handed nothing to
// derive from.
var ErrEmptySecret = errors.New("webhooksign: secret must not be empty")

// DeriveEncryptionKey turns an arbitrary-length secret into a 32-byte AES key.
// SHA-256 gives a fixed-size, uniformly distributed key for any input, so an
// operator can use a passphrase of any length (or a hex/base64 blob) and the
// same secret always yields the same key.
func DeriveEncryptionKey(secret string) ([]byte, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrEmptySecret
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}

// GenerateKey returns a new random key suitable for signing, rendered as a
// 64-character lowercase hex string (32 random bytes). It is what the API
// returns to an operator who asks the server to make one.
func GenerateKey() (string, error) {
	b := make([]byte, MinKeyLength)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("webhooksign: generate key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Sign returns the lowercase hex HMAC-SHA256 of payload under key.
func Sign(key, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is the valid HMAC-SHA256 of payload under
// key. Comparison is constant-time; malformed (non-hex) signatures are
// rejected without leaking how far the comparison got.
func Verify(key, payload []byte, signature string) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), provided)
}

// Encrypt seals plaintext with AES-256-GCM and returns base64(nonce||ciphertext),
// which is safe to store in a text column. The random nonce is prepended so no
// separate nonce column is needed.
func Encrypt(encKey []byte, plaintext string) (string, error) {
	gcm, err := newGCM(encKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("webhooksign: read nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A wrong key or a tampered payload fails
// authentication rather than returning garbage.
func Decrypt(encKey []byte, encoded string) (string, error) {
	gcm, err := newGCM(encKey)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("webhooksign: decode ciphertext: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("webhooksign: ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("webhooksign: decrypt: %w", err)
	}
	return string(plaintext), nil
}

func newGCM(encKey []byte) (cipher.AEAD, error) {
	if len(encKey) != 32 {
		return nil, fmt.Errorf("webhooksign: encryption key must be 32 bytes, got %d", len(encKey))
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("webhooksign: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("webhooksign: new gcm: %w", err)
	}
	return gcm, nil
}
