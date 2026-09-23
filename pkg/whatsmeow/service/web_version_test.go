package whatsmeow_service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Concurrent callers must collapse into a single HTTP fetch, and the fetch must
// not be performed while holding the cache lock. The server is slow so the
// callers genuinely overlap.
func TestFetchWhatsAppWebVersionCollapsesConcurrentFetches(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, `self.__next_f.push([1,"{\"client_revision\":123456}"])`)
	}))
	defer srv.Close()

	cachedWebVersionMu.Lock()
	oldURL, oldClient := whatsAppWebVersionURL, whatsAppWebVersionClient
	oldVer, oldAt := cachedWebVersion, cachedWebVersionAt
	whatsAppWebVersionURL = srv.URL
	whatsAppWebVersionClient = srv.Client()
	cachedWebVersion = nil
	cachedWebVersionAt = time.Time{}
	cachedWebVersionMu.Unlock()

	defer func() {
		cachedWebVersionMu.Lock()
		whatsAppWebVersionURL, whatsAppWebVersionClient = oldURL, oldClient
		cachedWebVersion, cachedWebVersionAt = oldVer, oldAt
		cachedWebVersionMu.Unlock()
	}()

	const callers = 8
	var wg sync.WaitGroup
	results := make(chan *clientVersion, callers)
	errs := make(chan error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := fetchWhatsAppWebVersion()
			if err != nil {
				errs <- err
				return
			}
			results <- v
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("fetch failed: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected exactly 1 HTTP request, got %d", got)
	}
	for v := range results {
		if v == nil || v.Patch != 123456 {
			t.Fatalf("unexpected version: %+v", v)
		}
	}
}

// A fresh cache must be served without any HTTP request at all.
func TestFetchWhatsAppWebVersionUsesCache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = io.WriteString(w, `{"client_revision":777}`)
	}))
	defer srv.Close()

	cachedWebVersionMu.Lock()
	oldURL, oldClient := whatsAppWebVersionURL, whatsAppWebVersionClient
	oldVer, oldAt := cachedWebVersion, cachedWebVersionAt
	whatsAppWebVersionURL = srv.URL
	whatsAppWebVersionClient = srv.Client()
	cachedWebVersion = &clientVersion{Major: 2, Minor: 3000, Patch: 999}
	cachedWebVersionAt = time.Now()
	cachedWebVersionMu.Unlock()

	defer func() {
		cachedWebVersionMu.Lock()
		whatsAppWebVersionURL, whatsAppWebVersionClient = oldURL, oldClient
		cachedWebVersion, cachedWebVersionAt = oldVer, oldAt
		cachedWebVersionMu.Unlock()
	}()

	v, err := fetchWhatsAppWebVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v.Patch != 999 {
		t.Fatalf("expected cached version, got %+v", v)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("expected no HTTP request for a fresh cache, got %d", got)
	}
}
