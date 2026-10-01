// Package tokencrypt stores instance API tokens encrypted at rest while still
// allowing O(1) lookup by the token itself.
//
// WHY A DEDICATED PACKAGE (AND WHY NOT JUST webhooksign.Encrypt)
//
// Instance tokens are the primary credential: every instance-scoped request
// presents one in the `apikey` header, and these were stored in plaintext.
// A database dump therefore handed over full access to every tenant.
//
// The obvious fix — AES-GCM encrypt the column, like the HMAC/S3 secrets —
// breaks authentication, because AES-GCM uses a random nonce, so the same token
// encrypts to a different ciphertext each time. You cannot look a row up by
// ciphertext.
//
// So we store two things:
//
//   - token_hash: HMAC-SHA256(token) under a server-side key. Deterministic, so
//     it is indexable and equality-searchable, and it reveals nothing about the
//     token (HMAC, not a bare digest, so a stolen DB cannot brute-force short
//     tokens without the key).
//   - token_enc:  AES-256-GCM(token), random nonce. Used to recover the
//     plaintext for display/backup; never searched by.
//
// Authentication computes the HMAC of the presented token and looks the row up
// by token_hash. The plaintext is never stored.
//
// # KEY MANAGEMENT
//
// The key is derived (SHA-256) from DATA_ENCRYPTION_KEY, GLOBAL_ENCRYPTION_KEY,
// or — last resort, for zero-config installs — GLOBAL_API_KEY. Same precedence
// as the other secrets, so an operator who set one of those needs no new config.
package tokencrypt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

// ErrNoKey is returned when no encryption key could be derived.
var ErrNoKey = errors.New("tokencrypt: no encryption key available")

// Codec encrypts tokens and computes their lookup hash.
type Codec struct {
	// encKey is the 32-byte AES-256-GCM key for reversible storage.
	encKey []byte
	// hashKey is the HMAC key for the deterministic lookup hash. Derived with a
	// different label so it is independent of encKey.
	hashKey []byte
}

// New builds a codec from an arbitrary-length secret. Both the encryption key
// and the HMAC key are derived from it with distinct labels, so knowing one does
// not reveal the other.
func New(secret string) (*Codec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrNoKey
	}
	encKey, err := webhooksign.DeriveEncryptionKey("tokencrypt:enc:" + secret)
	if err != nil {
		return nil, err
	}
	hashKey, err := webhooksign.DeriveEncryptionKey("tokencrypt:hash:" + secret)
	if err != nil {
		return nil, err
	}
	return &Codec{encKey: encKey, hashKey: hashKey}, nil
}

// Hash returns the deterministic, hex-encoded HMAC-SHA256 of a token. This is
// what the auth lookup queries by.
func (c *Codec) Hash(token string) string {
	if c == nil {
		return ""
	}
	mac := hmac.New(sha256.New, c.hashKey)
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// Encrypt returns the AES-256-GCM ciphertext (base64) for at-rest storage.
func (c *Codec) Encrypt(token string) (string, error) {
	if c == nil {
		return "", ErrNoKey
	}
	return webhooksign.Encrypt(c.encKey, token)
}

// Decrypt reverses Encrypt.
func (c *Codec) Decrypt(ciphertext string) (string, error) {
	if c == nil {
		return "", ErrNoKey
	}
	return webhooksign.Decrypt(c.encKey, ciphertext)
}

// LooksEncrypted reports whether s looks like a stored ciphertext rather than a
// plaintext token. Tokens are operator-chosen and almost always contain a dash
// or underscore (UUIDs); AES-GCM ciphertext rendered as standard base64 can too,
// so the discriminator is the UUID-shaped/dash form plus the minimum length.
//
// This is only used by the startup backfill to decide whether a row still holds
// plaintext; when in doubt it re-encrypts (idempotent), so a false "not
// encrypted" is harmless.
func LooksEncrypted(s string) bool {
	// base64 of nonce(12)+ciphertext+tag(16) is >= 28 bytes -> >= 40 base64 chars.
	if len(s) < 40 || !isLikelyBase64(s) {
		return false
	}
	// UUID-style tokens are exactly 36 chars (rejected by the length check) or
	// shorter; a long dash-form token is the ambiguous case and we treat it as
	// NOT encrypted so it gets backfilled.
	return !looksLikeUUIDish(s)
}

// looksLikeUUIDish reports whether s has the 8-4-4-4-12 dash grouping of a UUID.
func looksLikeUUIDish(s string) bool {
	if len(s) != 36 {
		return false
	}
	for _, i := range []int{8, 13, 18, 23} {
		if s[i] != '-' {
			return false
		}
	}
	return true
}

func isLikelyBase64(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+' || r == '/' || r == '=':
		default:
			return false
		}
	}
	return true
}
