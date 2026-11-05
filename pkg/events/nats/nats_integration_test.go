package nats_producer

import (
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
