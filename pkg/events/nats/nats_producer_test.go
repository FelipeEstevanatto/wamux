package nats_producer

import (
	"context"
	"testing"
	"time"
)

func TestJetStreamOptionStaysOffWithoutURL(t *testing.T) {
	p := NewNatsProducer("", true, nil, nil, WithJetStream("WAMUX_EVENTS")).(*natsProducer)
	if p.stream != "WAMUX_EVENTS" {
		t.Fatalf("stream = %q", p.stream)
	}
	if p.js != nil {
		t.Fatal("JetStream must stay nil without a connection")
	}
}

func TestDrainWithoutConnectionIsNoop(t *testing.T) {
	p := &natsProducer{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestNewNatsProducerDoesNotConnectWithoutURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"Empty URL", ""},
		{"Whitespace URL", "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			producer, ok := NewNatsProducer(tt.url, true, []string{"event1"}, nil).(*natsProducer)
			if !ok {
				t.Fatal("constructor returned an unexpected producer type")
			}
			if producer.conn != nil {
				t.Fatal("NATS connection must stay nil when NATS_URL is empty")
			}
			if producer.natsGlobalEnabled {
				t.Fatal("NATS global publishing must stay disabled without a URL")
			}
			if producer.natsGlobalEvents != nil {
				t.Fatal("natsGlobalEvents must be nil when NATS_URL is empty")
			}
		})
	}
}
