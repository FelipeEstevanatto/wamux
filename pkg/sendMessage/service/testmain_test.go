package send_service

import (
	"os"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/ssrf"
)

// TestMain swaps the package's outbound HTTP clients for permissive ones: the
// production clients are SSRF-guarded and refuse loopback, but the media/link/
// sticker tests spin up httptest servers on 127.0.0.1. The guard itself is
// covered by pkg/ssrf's own tests.
func TestMain(m *testing.M) {
	permissive := ssrf.New(ssrf.WithAllowPrivateNetworks(true))
	mediaHTTPClient = permissive.Client(60 * time.Second)
	stickerHTTPClient = permissive.Client(30 * time.Second)
	productImageHTTPClient = permissive.Client(30 * time.Second)

	os.Exit(m.Run())
}
