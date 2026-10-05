package benchmarks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

// Scenario: outbound webhook delivery. Every inbound message is JSON-encoded and
// HMAC-signed before it leaves the process, so this runs once per event on the
// busiest path in the system.

func realisticWebhookPayload() []byte {
	b, err := json.Marshal(webhookPayloadMap())
	if err != nil {
		panic(err)
	}
	return b
}

func webhookPayloadMap() map[string]any {
	return map[string]any{
		"event":         "Message",
		"instanceId":    benchInstanceID,
		"instanceName":  "bench",
		"instanceToken": benchInstanceToken,
		"data": map[string]any{
			"Info": map[string]any{
				"Chat":      "5511999999999@s.whatsapp.net",
				"Sender":    "5511999999999@s.whatsapp.net",
				"IsFromMe":  false,
				"IsGroup":   false,
				"ID":        "3EB0BENCH00000000001",
				"Type":      "text",
				"Timestamp": 1760000000,
			},
			"Message": map[string]any{
				"conversation": strings.Repeat("a realistic message body. ", 40), // ~1 KB
			},
		},
	}
}

var (
	webhookPayload = realisticWebhookPayload()
	webhookKey     = func() []byte {
		k, err := webhooksign.DeriveEncryptionKey("bench-secret-0123456789abcdef")
		if err != nil {
			panic(err)
		}
		return k
	}()
)

func BenchmarkWebhookPayloadMarshal(b *testing.B) {
	payload := webhookPayloadMap()
	b.SetBytes(int64(len(webhookPayload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWebhookSign(b *testing.B) {
	b.SetBytes(int64(len(webhookPayload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = webhooksign.Sign(webhookKey, webhookPayload)
	}
}

func BenchmarkWebhookVerify(b *testing.B) {
	sig := webhooksign.Sign(webhookKey, webhookPayload)
	b.SetBytes(int64(len(webhookPayload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !webhooksign.Verify(webhookKey, webhookPayload, sig) {
			b.Fatal("verify failed")
		}
	}
}

func BenchmarkWebhookKeyEncrypt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := webhooksign.Encrypt(webhookKey, "per-instance-hmac-key"); err != nil {
			b.Fatal(err)
		}
	}
}

// TestWebhookSignVerify validates signing round-trips and tamper rejection.
func TestWebhookSignVerify(t *testing.T) {
	sig := webhooksign.Sign(webhookKey, webhookPayload)
	if !webhooksign.Verify(webhookKey, webhookPayload, sig) {
		t.Fatal("valid signature rejected")
	}
	if webhooksign.Verify(webhookKey, append(webhookPayload, '!'), sig) {
		t.Fatal("tampered payload accepted")
	}
	other, _ := webhooksign.DeriveEncryptionKey("a-different-secret")
	if webhooksign.Verify(other, webhookPayload, sig) {
		t.Fatal("signature accepted under the wrong key")
	}
	if webhooksign.Verify(webhookKey, webhookPayload, "not-hex") {
		t.Fatal("malformed signature accepted")
	}
}

// TestWebhookEncryptDecrypt validates the at-rest key encryption used for
// per-instance signing keys.
func TestWebhookEncryptDecrypt(t *testing.T) {
	const secret = "the-per-instance-webhook-key"
	enc, err := webhooksign.Encrypt(webhookKey, secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := webhooksign.Decrypt(webhookKey, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != secret {
		t.Fatalf("round trip mismatch: got %q want %q", got, secret)
	}
	if _, err := webhooksign.Decrypt(otherKey(t), enc); err == nil {
		t.Fatal("decrypt with the wrong key succeeded")
	}
}

func otherKey(t *testing.T) []byte {
	t.Helper()
	k, err := webhooksign.DeriveEncryptionKey("a-different-secret")
	if err != nil {
		t.Fatal(err)
	}
	return k
}
