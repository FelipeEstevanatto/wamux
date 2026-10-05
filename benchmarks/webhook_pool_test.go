package benchmarks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Scenario: outbound webhook delivery. apime normalizes each event and fans it
// out through a worker pool; WaMux signs and dispatches. These benchmarks
// compare sequential delivery with a pooled fan-out against a real HTTP
// receiver, plus the normalization (payload build + marshal) cost.

const benchDeliveryLatency = 2 * time.Millisecond

func benchWebhookReceiver(tb testing.TB, latency time.Duration) (*httptest.Server, *atomic.Int64) {
	tb.Helper()
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if latency > 0 {
			time.Sleep(latency)
		}
		count.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	tb.Cleanup(srv.Close)
	return srv, &count
}

func postWebhook(client *http.Client, url string, body []byte) error {
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// BenchmarkWebhookNormalize is the per-event normalization: build the payload
// map and marshal it (what apime's normalizer does before fan-out).
func BenchmarkWebhookNormalize(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(webhookPayloadMap()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWebhookSyncDelivery(b *testing.B) {
	srv, count := benchWebhookReceiver(b, benchDeliveryLatency)
	client := srv.Client()
	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	req.Header.Set("Content-Type", "application/json")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req.Body = io.NopCloser(bytes.NewReader(webhookPayload))
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	b.StopTimer()
	if got := count.Load(); got != int64(b.N) {
		b.Fatalf("delivered %d, want %d", got, b.N)
	}
}

func BenchmarkWebhookPoolDelivery(b *testing.B) {
	const workers = 8
	srv, count := benchWebhookReceiver(b, benchDeliveryLatency)
	client := srv.Client()
	jobs := make(chan []byte, b.N+workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for body := range jobs {
				if err := postWebhook(client, srv.URL, body); err != nil {
					b.Error(err)
					return
				}
			}
		}()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jobs <- webhookPayload
	}
	close(jobs)
	wg.Wait()
	b.StopTimer()
	if got := count.Load(); got != int64(b.N) {
		b.Fatalf("delivered %d, want %d", got, b.N)
	}
}

// TestWebhookDispatchDeliversAll validates that both the synchronous and the
// pooled path deliver every event exactly once.
func TestWebhookDispatchDeliversAll(t *testing.T) {
	const n = 50
	srv, count := benchWebhookReceiver(t, 0)
	client := srv.Client()

	for i := 0; i < n; i++ {
		if err := postWebhook(client, srv.URL, webhookPayload); err != nil {
			t.Fatalf("sync delivery: %v", err)
		}
	}
	if got := count.Load(); got != n {
		t.Fatalf("sync delivered %d, want %d", got, n)
	}

	count.Store(0)
	jobs := make(chan []byte, n)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for body := range jobs {
				if err := postWebhook(client, srv.URL, body); err != nil {
					t.Errorf("pool delivery: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- webhookPayload
	}
	close(jobs)
	wg.Wait()
	if got := count.Load(); got != n {
		t.Fatalf("pool delivered %d, want %d", got, n)
	}
}
