package whatsmeow_service

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// representativePayload builds a JSON payload shaped like the Message webhook,
// with a base64 thumbnail — the largest field on the wire and the reason a full
// generic-map parse is expensive.
func representativePayload() []byte {
	thumb := base64.StdEncoding.EncodeToString(make([]byte, 10*1024))
	payload := map[string]interface{}{
		"event":      "Message",
		"instanceId": "inst-1",
		"data": map[string]interface{}{
			"Info": map[string]interface{}{
				"Chat":   "1234567890@g.us",
				"Sender": "5514999999999@s.whatsapp.net",
			},
			"Message": map[string]interface{}{
				"imageMessage": map[string]interface{}{
					"JPEGThumbnail": thumb,
					"caption":       "hi",
					"mediaKey":      base64.StdEncoding.EncodeToString(make([]byte, 32)),
				},
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return b
}

func TestParseWebhookEvent(t *testing.T) {
	cases := []struct {
		name         string
		json         string
		wantEvent    string
		wantDataChat string
		wantInfoChat string
		wantOK       bool
	}{
		{"message", `{"event":"Message","data":{"Info":{"Chat":"123@g.us"}}}`, "Message", "", "123@g.us", true},
		{"receipt", `{"event":"Receipt","data":{"Chat":"123@s.whatsapp.net"}}`, "Receipt", "123@s.whatsapp.net", "", true},
		{"sendmessage", `{"event":"SendMessage","data":{"Info":{"Chat":"123@g.us"}}}`, "SendMessage", "", "123@g.us", true},
		{"missing event", `{"data":{"Info":{"Chat":"x"}}}`, "", "", "", false},
		{"not an object", `"nope"`, "", "", "", false},
		{"data not an object", `{"event":"Connected","data":"x"}`, "Connected", "", "", true},
		{"invalid json", `{`, "", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, dataChat, infoChat, ok := parseWebhookEvent([]byte(tc.json))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if event != tc.wantEvent || dataChat != tc.wantDataChat || infoChat != tc.wantInfoChat {
				t.Fatalf("got event=%q dataChat=%q infoChat=%q", event, dataChat, infoChat)
			}
		})
	}
}

func BenchmarkParseWebhookEventStruct(b *testing.B) {
	payload := representativePayload()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, ok := parseWebhookEvent(payload); !ok {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseWebhookEventFullMap(b *testing.B) {
	payload := representativePayload()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}
