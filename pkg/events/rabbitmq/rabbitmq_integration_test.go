package rabbitmq_producer

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

// These run against a real broker: set WAMUX_TEST_AMQP_URL.
func realProducer(t testing.TB) (*rabbitMQProducer, *amqp.Channel) {
	t.Helper()
	url := os.Getenv("WAMUX_TEST_AMQP_URL")
	if url == "" {
		t.Skip("set WAMUX_TEST_AMQP_URL to run RabbitMQ tests")
	}
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger("i").Close() })
	p := NewRabbitMQProducer(nil, false, nil, nil, url, logs).(*rabbitMQProducer)

	reader, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	channel, err := reader.Channel()
	if err != nil {
		t.Fatal(err)
	}
	return p, channel
}

func queueFor(t testing.TB) string { return fmt.Sprintf("wamux-test-%d", time.Now().UnixNano()) }

func drop(t testing.TB, channel *amqp.Channel, queue string) {
	t.Cleanup(func() { _, _ = channel.QueueDelete(queue, false, false, false) })
}

func depth(t *testing.T, channel *amqp.Channel, queue string) int {
	t.Helper()
	q, err := channel.QueueDeclarePassive(queue, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
	if err != nil {
		t.Fatal(err)
	}
	return q.Messages
}

// "true" passes the check in sendToQueueOrWebhook, and used to publish nothing.
func TestEveryEnabledValuePublishes(t *testing.T) {
	for _, value := range []string{"enabled", "true", "global"} {
		t.Run(value, func(t *testing.T) {
			p, channel := realProducer(t)
			queue := queueFor(t)
			drop(t, channel, queue)
			if err := p.Produce(queue, []byte(`{}`), value, "i"); err != nil {
				t.Fatal(err)
			}
			// The confirm means the broker has it, so it can be read back at once.
			if got := depth(t, channel, queue); got != 1 {
				t.Fatalf("queue holds %d messages", got)
			}
		})
	}
}

func TestDisabledValuePublishesNothing(t *testing.T) {
	p, _ := realProducer(t)
	if err := p.Produce("wamux-test-disabled", []byte(`{}`), "disabled", "i"); err != nil {
		t.Fatal(err)
	}
	if p.conn != nil {
		t.Fatal("connected for a disabled instance")
	}
}

func TestChannelsAreReused(t *testing.T) {
	p, channel := realProducer(t)
	queue := queueFor(t)
	drop(t, channel, queue)
	for i := 0; i < 20; i++ {
		if err := p.Produce(queue, []byte(`{}`), "enabled", "i"); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(p.idle); got != 1 {
		t.Fatalf("idle channels = %d, want the one being reused", got)
	}
}

func TestConcurrentPublishesAllArrive(t *testing.T) {
	p, channel := realProducer(t)
	queue := queueFor(t)
	drop(t, channel, queue)

	const n = 200
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- p.Produce(queue, []byte(`{}`), "enabled", "i")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := depth(t, channel, queue); got != n {
		t.Fatalf("queue holds %d of %d", got, n)
	}
	if len(p.idle) > publishChannels {
		t.Fatalf("idle channels = %d", len(p.idle))
	}
}

func TestPublishRecoversFromALostConnection(t *testing.T) {
	p, channel := realProducer(t)
	p.maxRetries = 1 // a retry would hide a dead channel being reused
	queue := queueFor(t)
	drop(t, channel, queue)
	if err := p.Produce(queue, []byte(`{}`), "enabled", "i"); err != nil {
		t.Fatal(err)
	}

	p.mu.Lock()
	p.conn.Close() // the idle channel dies with it
	p.mu.Unlock()

	if err := p.Produce(queue, []byte(`{}`), "enabled", "i"); err != nil {
		t.Fatal(err)
	}
	if got := depth(t, channel, queue); got != 2 {
		t.Fatalf("queue holds %d messages", got)
	}
}

// A queue that exists with other arguments makes the declare fail, which closes the
// channel. That must surface as an error, and the dead channel must not be reused.
func TestDeclareFailureIsReported(t *testing.T) {
	p, channel := realProducer(t)
	p.maxRetries = 1
	queue := queueFor(t)
	drop(t, channel, queue)
	if _, err := channel.QueueDeclare(queue, false, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Produce(queue, []byte(`{}`), "enabled", "i"); err == nil {
		t.Fatal("expected an error")
	}
	if len(p.idle) != 0 {
		t.Fatal("a failed channel went back in the pool")
	}
}

func BenchmarkProduce(b *testing.B) {
	p, channel := realProducer(b)
	queue := queueFor(b)
	drop(b, channel, queue)
	payload := make([]byte, 2048)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.Produce(queue, payload, "enabled", "i"); err != nil {
			b.Fatal(err)
		}
	}
}
