package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func clientIPSeen(t *testing.T, proxies []string, peer, forwarded string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := engine.SetTrustedProxies(proxies); err != nil {
		t.Fatal(err)
	}
	seen := ""
	engine.GET("/", func(c *gin.Context) { seen = c.ClientIP() })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer + ":40000"
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	engine.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestDefaultProxiesWorkBehindALocalProxy(t *testing.T) {
	proxies := resolveTrustedProxies("")
	for name, tc := range map[string]struct{ peer, forwarded, want string }{
		"proxy on the docker network": {"172.18.0.5", "203.0.113.9", "203.0.113.9"},
		"proxy on the host":           {"127.0.0.1", "203.0.113.9", "203.0.113.9"},
		"proxy on the lan":            {"192.168.1.10", "203.0.113.9", "203.0.113.9"},
		"chained local proxies":       {"10.0.0.2", "203.0.113.9, 10.0.0.3", "203.0.113.9"},
		"direct client":               {"203.0.113.9", "", "203.0.113.9"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := clientIPSeen(t, proxies, tc.peer, tc.forwarded); got != tc.want {
				t.Fatalf("saw %s, want %s", got, tc.want)
			}
		})
	}
}

// Spoofing the header is how a caller would dodge the rate limiter.
func TestDefaultProxiesIgnoreSpoofedHeaders(t *testing.T) {
	proxies := resolveTrustedProxies("")
	if got := clientIPSeen(t, proxies, "203.0.113.9", "198.51.100.1"); got != "203.0.113.9" {
		t.Errorf("a public client chose its own address: %s", got)
	}
	if got := clientIPSeen(t, proxies, "172.18.0.5", "198.51.100.1, 203.0.113.9"); got != "203.0.113.9" {
		t.Errorf("a client behind the proxy chose its own address: %s", got)
	}
}

func TestTrustedProxiesCanBeSetOrTurnedOff(t *testing.T) {
	if got := clientIPSeen(t, resolveTrustedProxies("none"), "172.18.0.5", "203.0.113.9"); got != "172.18.0.5" {
		t.Errorf("none: saw %s", got)
	}
	if got := clientIPSeen(t, resolveTrustedProxies(" None "), "127.0.0.1", "203.0.113.9"); got != "127.0.0.1" {
		t.Errorf("None: saw %s", got)
	}
	explicit := resolveTrustedProxies("198.51.100.7, 10.1.0.0/16")
	if got := clientIPSeen(t, explicit, "198.51.100.7", "203.0.113.9"); got != "203.0.113.9" {
		t.Errorf("listed proxy: saw %s", got)
	}
	if got := clientIPSeen(t, explicit, "192.168.1.10", "203.0.113.9"); got != "192.168.1.10" {
		t.Errorf("unlisted proxy: saw %s", got)
	}
}

func TestGlobalAPIKeyIsGeneratedOnceAndKept(t *testing.T) {
	logs := filepath.Join(t.TempDir(), "data", "logs")

	first, err := resolveGlobalAPIKey("", logs)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 48 {
		t.Fatalf("key has %d characters", len(first))
	}
	second, err := resolveGlobalAPIKey("", logs)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatal("a restart changed the key, which would lock out every token")
	}

	info, err := os.Stat(filepath.Join(filepath.Dir(logs), globalAPIKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %o", info.Mode().Perm())
	}
}

func TestConfiguredGlobalAPIKeyWins(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	got, err := resolveGlobalAPIKey("from-env", logs)
	if err != nil || got != "from-env" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, globalAPIKeyFile)); !os.IsNotExist(err) {
		t.Fatalf("wrote a key file: %v", err)
	}
}

func TestGlobalAPIKeyFailsWithoutAWritableDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGlobalAPIKey("", filepath.Join(blocker, "logs")); err == nil {
		t.Fatal("expected an error")
	}
}
