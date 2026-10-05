package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/tests"
)

// Scenario: sending one message to a group with N devices.
//
// whatsmeow encrypts the message body ONCE with the group SenderKey (skmsg) and
// only pairwise-encrypts the small SenderKeyDistributionMessage per device
// (send.go sendGroup -> prepareMessageNode -> encryptMessageForDevices). The
// naive model in some external analyses ("encrypt the full body for every
// participant") is ~3x slower and does not match the library.

// BenchmarkGroupBodyEncrypt isolates the one per-message SenderKey encryption.
// It is independent of group size.
func BenchmarkGroupBodyEncrypt(b *testing.B) {
	ctx := context.Background()
	s := newBenchSender(nil)
	msg := benchMsg()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.groupCipher.Encrypt(ctx, msg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGroupSendSim is a full group send for N devices:
// 1 body encryption + 1 SKDM build + N pairwise SKDM encryptions.
func BenchmarkGroupSendSim(b *testing.B) {
	for _, n := range []int{1, 10, 100, 300, 600} {
		b.Run(fmt.Sprintf("devices=%d", n), func(b *testing.B) {
			ctx := context.Background()
			s := newBenchSender(nil)
			ciphers := s.sessions(benchRemotes(n))
			msg := benchMsg()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				body, err := s.groupCipher.Encrypt(ctx, msg)
				if err != nil {
					b.Fatal(err)
				}
				_ = body
				skdm := latestSKDM(b, s)
				for _, c := range ciphers {
					if _, err := c.Encrypt(ctx, skdm); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkNaiveFullBodyPerDevice models the counterfactual: no sender key, so
// the full body is re-encrypted for every device. Included only for contrast.
func BenchmarkNaiveFullBodyPerDevice(b *testing.B) {
	for _, n := range []int{100, 300, 600} {
		b.Run(fmt.Sprintf("devices=%d", n), func(b *testing.B) {
			ctx := context.Background()
			s := newBenchSender(nil)
			ciphers := s.sessions(benchRemotes(n))
			msg := benchMsg()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, c := range ciphers {
					if _, err := c.Encrypt(ctx, msg); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// TestGroupSendRoundTrip validates the exact path the benchmarks exercise: a
// sender-key message encrypted by the sender must decrypt on a receiver that
// processed the SenderKeyDistributionMessage.
func TestGroupSendRoundTrip(t *testing.T) {
	ctx := context.Background()

	alice := newBenchSender(nil)
	skdm, err := alice.groupBuilder.Create(ctx, alice.groupName)
	if err != nil {
		t.Fatalf("create sender key: %v", err)
	}

	// Bob receives the SKDM (pairwise-encrypted in production) and builds his
	// receiving cipher for Alice's sender key.
	bobStore := tests.NewInMemorySenderKey()
	bobBuilder := groups.NewGroupSessionBuilder(bobStore, benchSer)
	if err := bobBuilder.Process(ctx, alice.groupName, skdm); err != nil {
		t.Fatalf("process skdm: %v", err)
	}
	bobCipher := groups.NewGroupCipher(bobBuilder, alice.groupName, bobStore)

	plaintext := []byte("hello group, this is the round-trip validation")
	ct, err := alice.groupCipher.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Emulate the wire.
	skm, ok := ct.(*protocol.SenderKeyMessage)
	if !ok {
		t.Fatalf("unexpected ciphertext type %T", ct)
	}
	wire, err := protocol.NewSenderKeyMessageFromBytes(skm.SignedSerialize(), benchSer.SenderKeyMessage)
	if err != nil {
		t.Fatalf("parse sender key message: %v", err)
	}

	got, err := bobCipher.Decrypt(ctx, wire)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}
