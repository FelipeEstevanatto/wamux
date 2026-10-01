package jsonx

import (
	"testing"

	stdjson "encoding/json"
)

// A representative event payload: the shape actually marshalled per message in
// the webhook/queue path (nesting + a media-ish map + a message body).
func samplePayload() map[string]any {
	return map[string]any{
		"event":         "Message",
		"instanceToken": "00000000-0000-0000-0000-000000000000",
		"instanceId":    "2a2ddce2-99cf-44d1-b36b-addfefeb6596",
		"instanceName":  "Instancia 1",
		"data": map[string]any{
			"Info": map[string]any{
				"ID":        "3EB06DB6CA32F41FC71F1F",
				"Chat":      "5514997732472@s.whatsapp.net",
				"Sender":    "5514991421911@s.whatsapp.net",
				"Timestamp": "2026-09-30T21:20:01-03:00",
				"IsFromMe":  false,
				"IsGroup":   false,
				"PushName":  "Alice Souza",
			},
			"Message": map[string]any{
				"conversation": "hello there, this is a representative message body for the benchmark",
			},
			"MessageType": "text",
			"mediaUrl":    "https://minio.local/wamux-media/3EB06DB6CA32F41FC71F1F.jpg",
			"mimetype":    "image/jpeg",
		},
	}
}

// Marshal is what the app would call; it is swapped to the fast encoder below.
func BenchmarkMarshalStd(b *testing.B) {
	p := samplePayload()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := stdjson.Marshal(p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalFast(b *testing.B) {
	p := samplePayload()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalStd(b *testing.B) {
	raw, _ := stdjson.Marshal(samplePayload())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out map[string]any
		if err := stdjson.Unmarshal(raw, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalFast(b *testing.B) {
	raw, _ := stdjson.Marshal(samplePayload())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var out map[string]any
		if err := Unmarshal(raw, &out); err != nil {
			b.Fatal(err)
		}
	}
}
