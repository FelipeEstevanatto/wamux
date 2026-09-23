package whatsmeow_service

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestMessageEphemeralExpiration(t *testing.T) {
	tests := []struct {
		name string
		msg  *waE2E.Message
		want uint32
	}{
		{
			name: "extended text context",
			msg: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(86400)},
			}},
			want: 86400,
		},
		{
			name: "image context",
			msg: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(604800)},
			}},
			want: 604800,
		},
		{
			name: "ephemeral wrapper",
			msg: &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(7776000)},
				}},
			}},
			want: 7776000,
		},
		{
			name: "timer change protocol message",
			msg: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				Type:                waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum(),
				EphemeralExpiration: proto.Uint32(86400),
			}},
			want: 86400,
		},
		{name: "no timer", msg: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{}}, want: 0},
		{name: "nil message", msg: nil, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messageEphemeralExpiration(tt.msg); got != tt.want {
				t.Fatalf("messageEphemeralExpiration() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestChatEphemeralCache(t *testing.T) {
	chat := types.NewJID("5514981170846", types.DefaultUserServer)

	if _, known := GetCachedChatEphemeral("inst-cache-test", chat); known {
		t.Fatal("expected an unknown chat to have no cached timer")
	}

	SetCachedChatEphemeral("inst-cache-test", chat, 86400)
	seconds, known := GetCachedChatEphemeral("inst-cache-test", chat)
	if !known || seconds != 86400 {
		t.Fatalf("got (%d, %v), want (86400, true)", seconds, known)
	}

	// A different instance must not see the value.
	if _, known := GetCachedChatEphemeral("other-instance", chat); known {
		t.Fatal("cache must be per instance")
	}
}
