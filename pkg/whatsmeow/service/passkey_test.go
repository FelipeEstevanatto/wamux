package whatsmeow_service

import "testing"

// Passkey* events are part of the #wapk pairing flow. They must reach both the
// dedicated PASSKEY subscription and the QRCODE subscription, and must not be
// dropped when neither is set (issue #105).
func TestShouldForwardPasskey(t *testing.T) {
	cases := []struct {
		name          string
		subscriptions []string
		want          bool
	}{
		{"dedicated PASSKEY subscription", []string{"MESSAGE", "PASSKEY"}, true},
		{"QRCODE subscription (pairing flow)", []string{"QRCODE"}, true},
		{"case-insensitive", []string{"passkey"}, true},
		{"unrelated subscription is not forwarded", []string{"MESSAGE", "CONNECTION"}, false},
		{"empty subscription is not forwarded", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldForwardPasskey(tc.subscriptions); got != tc.want {
				t.Fatalf("shouldForwardPasskey(%v) = %v, want %v", tc.subscriptions, got, tc.want)
			}
		})
	}
}
