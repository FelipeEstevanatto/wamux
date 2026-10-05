package benchmarks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Scenario: per-request authentication. WaMux looks an instance up by
// HMAC-SHA256(token) and caches the result; apime verifies a JWT and then
// checks roles (RBAC). These benchmarks compare the raw verification cost of
// each model (not the surrounding middleware).
//
//		go test -run=^$ -bench='Auth' ./benchmarks/ -benchtime=200000x

var (
	benchAuthSecret = []byte("bench-server-secret")
	benchAuthTokens = func() []string {
		out := make([]string, 256)
		for i := range out {
			out[i] = "instance-token-" + itoa(i)
		}
		return out
	}()
)

func benchTokenHash(token string) string {
	m := hmac.New(sha256.New, benchAuthSecret)
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

// BenchmarkAuthTokenHash models WaMux's cached instance lookup: hash the token,
// then hit an in-memory index.
func BenchmarkAuthTokenHash(b *testing.B) {
	index := make(map[string]int, len(benchAuthTokens))
	for i, tok := range benchAuthTokens {
		index[benchTokenHash(tok)] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := index[benchTokenHash(benchAuthTokens[i%len(benchAuthTokens)])]; !ok {
			b.Fatal("token not found")
		}
	}
}

// --- HS256 JWT, implemented with stdlib so the benchmark adds no dependency ---

var (
	errBadJWT     = errors.New("auth: malformed or tampered token")
	errExpiredJWT = errors.New("auth: token expired")
)

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signJWT(sub string, exp time.Time) string {
	header := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"sub": sub, "exp": exp.Unix()})
	body := header + "." + b64url(payload)
	m := hmac.New(sha256.New, benchAuthSecret)
	m.Write([]byte(body))
	return body + "." + b64url(m.Sum(nil))
}

func verifyJWT(tok string) (string, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return "", errBadJWT
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errBadJWT
	}
	m := hmac.New(sha256.New, benchAuthSecret)
	m.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, m.Sum(nil)) {
		return "", errBadJWT
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errBadJWT
	}
	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return "", errBadJWT
	}
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return "", errExpiredJWT
	}
	return claims.Sub, nil
}

func BenchmarkAuthJWT(b *testing.B) {
	tokens := make([]string, len(benchAuthTokens))
	for i, tok := range benchAuthTokens {
		tokens[i] = signJWT(tok, time.Now().Add(time.Hour))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := verifyJWT(tokens[i%len(tokens)]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAuthJWTRBAC adds the role lookup apime performs after verification.
func BenchmarkAuthJWTRBAC(b *testing.B) {
	tokens := make([]string, len(benchAuthTokens))
	roles := make(map[string][]string, len(benchAuthTokens))
	for i, tok := range benchAuthTokens {
		tokens[i] = signJWT(tok, time.Now().Add(time.Hour))
		roles[tok] = []string{"instance:read", "instance:send"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub, err := verifyJWT(tokens[i%len(tokens)])
		if err != nil {
			b.Fatal(err)
		}
		if len(roles[sub]) == 0 {
			b.Fatal("no roles")
		}
	}
}

func TestAuthSemantics(t *testing.T) {
	// Token hash: deterministic, and a wrong token misses.
	h := benchTokenHash("abc")
	if h != benchTokenHash("abc") {
		t.Fatal("token hash is not deterministic")
	}
	if h == benchTokenHash("abd") {
		t.Fatal("different tokens collided")
	}

	// JWT: valid, tampered, expired.
	tok := signJWT("user-1", time.Now().Add(time.Hour))
	sub, err := verifyJWT(tok)
	if err != nil || sub != "user-1" {
		t.Fatalf("valid token rejected: sub=%q err=%v", sub, err)
	}
	if _, err := verifyJWT(tok + "x"); !errors.Is(err, errBadJWT) {
		t.Fatalf("tampered token accepted: %v", err)
	}
	expired := signJWT("user-1", time.Now().Add(-time.Minute))
	if _, err := verifyJWT(expired); !errors.Is(err, errExpiredJWT) {
		t.Fatalf("expired token accepted: %v", err)
	}
}
