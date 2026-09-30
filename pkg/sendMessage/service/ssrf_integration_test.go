package send_service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/ssrf"
)

// The package's outbound fetches must use the SSRF-guarded client in
// production. TestMain installs a permissive client for the other HTTP tests,
// so these restore the real guard for the duration of the test.
func withGuardedMediaClient(t *testing.T) {
	t.Helper()
	oldMedia, oldSticker := mediaHTTPClient, stickerHTTPClient
	// An explicitly enforcing guard, independent of the SSRF_PROTECTION default:
	// this proves the fetch path consults the guard's dialer at all.
	guarded := ssrf.New()
	mediaHTTPClient = guarded.Client(5 * time.Second)
	stickerHTTPClient = guarded.Client(5 * time.Second)
	t.Cleanup(func() {
		mediaHTTPClient = oldMedia
		stickerHTTPClient = oldSticker
	})
}

func TestFetchStickerDataRefusesPrivateURL(t *testing.T) {
	withGuardedMediaClient(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := fetchStickerData(context.Background(), server.URL)
	if err == nil {
		t.Fatal("sticker fetch reached a loopback server")
	}
	if !errors.Is(err, ssrf.ErrBlockedAddress) {
		t.Fatalf("error = %v, want ErrBlockedAddress", err)
	}
}

func TestFetchLinkThumbnailRefusesPrivateURL(t *testing.T) {
	withGuardedMediaClient(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if thumb, _, _ := fetchLinkThumbnail(server.URL); thumb != nil {
		t.Fatal("link thumbnail fetch reached a loopback server")
	}
}
