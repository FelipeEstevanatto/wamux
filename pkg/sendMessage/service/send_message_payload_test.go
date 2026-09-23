package send_service

import (
	"encoding/json"
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// The send path now assigns the typed protobuf message directly instead of
// converting it to a map with a JSON round trip. The emitted payload must be
// unchanged, so compare the two forms as parsed JSON.
func TestSendMessagePayloadEquivalence(t *testing.T) {
	msgs := map[string]*waE2E.Message{
		"text": &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("hello")}},
		"image": &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String("cap"),
			MediaKey:      []byte{1, 2, 3},
			JPEGThumbnail: []byte{9, 8, 7},
		}},
	}

	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			// Old: marshal the message, unmarshal into a map, put the map in the payload.
			raw, err := json.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			var msgMap map[string]interface{}
			if err := json.Unmarshal(raw, &msgMap); err != nil {
				t.Fatal(err)
			}
			oldPayload, err := json.Marshal(map[string]interface{}{"Message": msgMap})
			if err != nil {
				t.Fatal(err)
			}

			// New: assign the typed message directly.
			newPayload, err := json.Marshal(map[string]interface{}{"Message": msg})
			if err != nil {
				t.Fatal(err)
			}

			var oldVal, newVal interface{}
			if err := json.Unmarshal(oldPayload, &oldVal); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(newPayload, &newVal); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(oldVal, newVal) {
				t.Fatalf("payload changed:\nold=%s\nnew=%s", oldPayload, newPayload)
			}
		})
	}
}
