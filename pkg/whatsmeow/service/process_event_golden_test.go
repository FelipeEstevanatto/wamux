package whatsmeow_service

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Webhook payloads are the public contract with every integration. These files
// make any change to them show up in review. If a change is intended, regenerate
// with: go test ./pkg/whatsmeow/service/ -run Golden -update-golden
var updateGolden = flag.Bool("update-golden", false, "rewrite the golden webhook payloads")

func TestGoldenWebhookPayloads(t *testing.T) {
	contact := mustJID(t, testContact)
	cases := map[string]func(t *testing.T) interface{}{
		"message_text": func(t *testing.T) interface{} { return inbound(t, "MSG1", "hello") },
		"message_group": func(t *testing.T) interface{} {
			return inGroup(t, inbound(t, "MSG1", "hi all"))
		},
		"message_lid_sender": func(t *testing.T) interface{} {
			evt := inbound(t, "MSG1", "hi")
			lid := mustJID(t, testContactLID)
			evt.Info.Chat, evt.Info.Sender, evt.Info.SenderAlt = lid, lid, contact
			return evt
		},
		"message_reply": func(t *testing.T) interface{} {
			evt := inbound(t, "MSG2", "")
			evt.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String("reply"),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:      proto.String("MSG1"),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("original")},
				},
			}}
			return evt
		},
		"message_image": func(t *testing.T) interface{} {
			evt := inbound(t, "MSG1", "")
			evt.Message = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
				Mimetype:      proto.String("image/jpeg"),
				Caption:       proto.String("a photo"),
				JPEGThumbnail: []byte("thumbnail"),
			}}
			return evt
		},
		"receipt_read":      func(t *testing.T) interface{} { return receipt(t, types.ReceiptTypeRead, "A", "B") },
		"receipt_delivered": func(t *testing.T) interface{} { return receipt(t, types.ReceiptTypeDelivered, "A") },
		"presence_offline": func(t *testing.T) interface{} {
			return &events.Presence{From: contact, Unavailable: true, LastSeen: testTime}
		},
		"connected":    func(t *testing.T) interface{} { return &events.Connected{} },
		"disconnected": func(t *testing.T) interface{} { return &events.Disconnected{} },
		"pair_success": func(t *testing.T) interface{} {
			return &events.PairSuccess{ID: mustJID(t, testOwnJID), Platform: "android"}
		},
		"call_offer": func(t *testing.T) interface{} {
			return &events.CallOffer{BasicCallMeta: types.BasicCallMeta{
				From: contact, Timestamp: testTime, CallCreator: contact, CallID: "C1",
			}}
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			h.cli.processEvent(build(t))
			if name == "disconnected" {
				h.wantReconnect(t)
			}
			checkGolden(t, name, h.deliveries(t, 1)[0].payload)
		})
	}
}

func checkGolden(t *testing.T, name string, payload []byte) {
	t.Helper()
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, payload, "", "  "); err != nil {
		t.Fatal(err)
	}
	pretty.WriteByte('\n')

	path := filepath.Join("testdata", "webhook", name+".json")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (generate it with -update-golden)", err)
	}
	if !bytes.Equal(pretty.Bytes(), want) {
		t.Errorf("payload changed:\n--- want\n%s\n--- got\n%s", want, pretty.Bytes())
	}
}
