package rabbitmq_producer

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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

// rejectAfter makes the broker nack anything past `limit` messages in queue. Only a
// policy can do that: publishOnce declares the queue with fixed arguments, so a
// queue pre-declared with a length limit would fail with PRECONDITION_FAILED.
func rejectAfter(t *testing.T, queue string, limit int) {
	t.Helper()
	management := os.Getenv("WAMUX_TEST_AMQP_MANAGEMENT_URL")
	if management == "" {
		t.Skip("set WAMUX_TEST_AMQP_MANAGEMENT_URL to run broker nack tests")
	}
	parsed, err := url.Parse(management)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	endpoint := fmt.Sprintf("%s://%s/api/policies/%%2F/%s", parsed.Scheme, parsed.Host, queue)

	call := func(method, body string) {
		t.Helper()
		req, err := http.NewRequest(method, endpoint, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.SetBasicAuth(parsed.User.Username(), password)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("%s policy: %s", method, resp.Status)
		}
	}

	call(http.MethodPut, fmt.Sprintf(
		`{"pattern":"^%s$","apply-to":"queues","definition":{"max-length":%d,"overflow":"reject-publish"}}`,
		queue, limit,
	))
	t.Cleanup(func() { call(http.MethodDelete, "") })
}

// A nack must not read as delivered: the event would be lost with no error, and
// the caller would never retry.
func TestBrokerNackIsReportedAsAFailure(t *testing.T) {
	p, channel := realProducer(t)
	queue := queueFor(t)
	drop(t, channel, queue)

	if err := p.publishOnce(queue, []byte("creates the queue")); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	rejectAfter(t, queue, 1)

	// The policy reaches the queue shortly after the API accepts it.
	var err error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if err = p.publishOnce(queue, []byte("over the limit")); err != nil {
			break
		}
	}

	if err == nil || !strings.Contains(err.Error(), "broker recusou") {
		t.Fatalf("err = %v, want a broker refusal", err)
	}
}

// The channel survives a nack, so the next publish after room appears succeeds.
func TestProducerRecoversAfterABrokerNack(t *testing.T) {
	p, channel := realProducer(t)
	queue := queueFor(t)
	drop(t, channel, queue)

	if err := p.publishOnce(queue, []byte("creates the queue")); err != nil {
		t.Fatal(err)
	}
	rejectAfter(t, queue, 1)

	var err error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if err = p.publishOnce(queue, []byte("over the limit")); err != nil {
			break
		}
	}
	if err == nil {
		t.Fatal("the broker never refused a message")
	}

	if _, err := channel.QueuePurge(queue, false); err != nil {
		t.Fatal(err)
	}
	if err := p.publishOnce(queue, []byte("room again")); err != nil {
		t.Fatalf("publish after the nack: %v", err)
	}
}
