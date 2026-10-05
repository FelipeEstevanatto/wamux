package benchmarks

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Scenario: the send API's latency/throughput model.
//
// WaMux sends synchronously: the HTTP handler calls whatsmeow.SendMessage and
// waits for the WhatsApp server ack (up to 75s). apime (open-apime/apime)
// exposes an async outbox at POST /api/instances/:id/messages: the handler
// persists the message as status=queued, enqueues it (in-memory channel or
// Redis list) and returns 202; a pool of workers (default 5) drains the queue
// and calls whatsmeow.
//
// The important property to model: whatsmeow serializes sends per client with
// an internal messageSendLock ("everything will explode if you send a message to
// the same user twice in parallel", send.go). So an outbox does NOT raise a
// single instance's send throughput — it decouples API latency and adds
// durability. The benchmarks below separate those two effects.

const outboxRTT = 20 * time.Millisecond // modeled WhatsApp server round trip

// syncSend models the current WaMux path: hold the per-client lock for the RTT.
func syncSend(lock *sync.Mutex, rtt time.Duration) {
	lock.Lock()
	defer lock.Unlock()
	if rtt > 0 {
		time.Sleep(rtt)
	}
}

// outbox models apime's queue + worker pool. lock stands in for whatsmeow's
// per-client messageSendLock.
type outbox struct {
	jobs chan struct{}
	lock sync.Mutex
	rtt  time.Duration
	sent atomic.Int64
	wg   sync.WaitGroup
}

func newOutbox(buffer, workers int, rtt time.Duration) *outbox {
	if buffer < 1 {
		buffer = 1
	}
	o := &outbox{jobs: make(chan struct{}, buffer), rtt: rtt}
	o.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer o.wg.Done()
			for range o.jobs {
				syncSend(&o.lock, o.rtt)
				o.sent.Add(1)
			}
		}()
	}
	return o
}

func (o *outbox) enqueue() { o.jobs <- struct{}{} }

func (o *outbox) closeAndWait() {
	close(o.jobs)
	o.wg.Wait()
}

// BenchmarkSendSyncLatency is the current WaMux API latency: the request blocks
// for the full WhatsApp round trip.
func BenchmarkSendSyncLatency(b *testing.B) {
	var lock sync.Mutex
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syncSend(&lock, outboxRTT)
	}
}

// BenchmarkSendAsyncEnqueueLatency is apime's 202 path: the request returns as
// soon as the message is queued, while a worker handles the RTT.
func BenchmarkSendAsyncEnqueueLatency(b *testing.B) {
	o := newOutbox(b.N+1, 1, outboxRTT)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.enqueue()
	}
	b.StopTimer() // the drain is the worker's cost, not the request's
	o.closeAndWait()
}

// BenchmarkOutboxDrainThroughput enqueues b.N messages and waits for the pool to
// drain them. Because of the per-client lock, 8 workers are no faster than 1:
// the outbox moves latency off the request, it does not multiply send capacity.
func BenchmarkOutboxDrainThroughput(b *testing.B) {
	const rtt = 1 * time.Millisecond
	for _, workers := range []int{1, 8} {
		b.Run(itoa(workers)+"workers", func(b *testing.B) {
			o := newOutbox(b.N+workers, workers, rtt)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				o.enqueue()
			}
			o.closeAndWait()
		})
	}
}

// BenchmarkSendParallelNoLock is the counterfactual: with no per-client lock,
// throughput scales with workers. It shows the lock, not the queue, is the
// ceiling.
func BenchmarkSendParallelNoLock(b *testing.B) {
	const rtt = 1 * time.Millisecond
	for _, workers := range []int{1, 8} {
		b.Run(itoa(workers)+"workers", func(b *testing.B) {
			b.SetParallelism(workers)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					time.Sleep(rtt)
				}
			})
		})
	}
}

// TestOutboxSendsAllOnce validates the queue drains every job exactly once.
func TestOutboxSendsAllOnce(t *testing.T) {
	const n = 200
	o := newOutbox(n, 8, 0)
	for i := 0; i < n; i++ {
		o.enqueue()
	}
	o.closeAndWait()
	if got := o.sent.Load(); got != n {
		t.Fatalf("sent %d, want %d", got, n)
	}
}

// TestSyncBlocksAsyncDoesNot validates the latency property the benchmarks
// measure: the sync path blocks for the RTT, the async enqueue returns at once.
func TestSyncBlocksAsyncDoesNot(t *testing.T) {
	var lock sync.Mutex
	start := time.Now()
	syncSend(&lock, outboxRTT)
	if elapsed := time.Since(start); elapsed < outboxRTT {
		t.Fatalf("sync send returned in %v, want >= %v", elapsed, outboxRTT)
	}

	o := newOutbox(1, 1, outboxRTT)
	defer o.closeAndWait()
	start = time.Now()
	o.enqueue()
	if elapsed := time.Since(start); elapsed > outboxRTT/4 {
		t.Fatalf("async enqueue blocked for %v", elapsed)
	}
}
