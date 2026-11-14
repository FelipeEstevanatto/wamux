package nats_producer

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/nats-io/nats.go"
)

// Runs against a real server: set WAMUX_TEST_NATS_URL.
func TestEveryEnabledValuePublishes(t *testing.T) {
	url := os.Getenv("WAMUX_TEST_NATS_URL")
	if url == "" {
		t.Skip("set WAMUX_TEST_NATS_URL to run NATS tests")
	}
	conn, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	defer logs.GetLogger("i").Close()
	p := &natsProducer{conn: conn, loggerWrapper: logs}

	for _, value := range []string{"enabled", "true", "global"} {
		t.Run(value, func(t *testing.T) {
			sub, err := conn.SubscribeSync("wamux.test." + value)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Unsubscribe()
			if err := p.Produce("wamux.test."+value, []byte(`{}`), value, "i"); err != nil {
				t.Fatal(err)
			}
			if _, err := sub.NextMsg(2 * time.Second); err != nil {
				t.Fatalf("nothing arrived: %v", err)
			}
		})
	}
}

// A retry of the same payload inside JetStream's duplicate window must leave
// one stream message, not two.
func TestJetStreamPublishDedupesPayload(t *testing.T) {
	url := os.Getenv("WAMUX_TEST_NATS_URL")
	if url == "" {
		t.Skip("set WAMUX_TEST_NATS_URL to run NATS tests")
	}
	admin, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	js, err := admin.JetStream()
	if err != nil {
		t.Skipf("JetStream unavailable: %v", err)
	}
	stream := fmt.Sprintf("WAMUX_TEST_%d", time.Now().UnixNano())
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     stream,
		Subjects: []string{stream + ".>"},
		Storage:  nats.MemoryStorage,
	}); err != nil {
		t.Skipf("JetStream is not enabled on this server: %v", err)
	}
	defer js.DeleteStream(stream)

	pub, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	defer logs.GetLogger("i").Close()
	p := &natsProducer{conn: pub, js: js, stream: stream, loggerWrapper: logs}

	payload := []byte(`{"event":"Message","id":"same"}`)
	for range 2 {
		if err := p.Produce(stream+".evt", payload, "enabled", "i"); err != nil {
			t.Fatal(err)
		}
	}
	info, err := js.StreamInfo(stream)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("stream has %d messages, want 1 after a duplicate", info.State.Msgs)
	}
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}
