package ssrf

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBlockedIPClassifiesAddresses(t *testing.T) {
	guard := New()

	blocked := []string{
		"127.0.0.1",
		"127.1.2.3",
		"10.0.0.1",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.1",
		"169.254.169.254", // cloud metadata
		"100.64.0.1",      // CGNAT
		"0.0.0.0",
		"224.0.0.1", // multicast
		"255.255.255.255",
		"192.0.0.5",        // IETF protocol assignments
		"198.18.0.1",       // benchmark
		"::1",              // IPv6 loopback
		"fe80::1",          // IPv6 link-local
		"fc00::1",          // IPv6 unique local
		"fd12:3456::1",     // IPv6 unique local
		"ff02::1",          // IPv6 multicast
		"::",               // unspecified
		"::ffff:127.0.0.1", // IPv4-mapped loopback
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("test address %q did not parse", raw)
		}
		if !guard.BlockedIP(ip) {
			t.Errorf("BlockedIP(%s) = false, want true", raw)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",
		"172.15.0.1", // just below the private 172.16/12 block
		"172.32.0.1", // just above it
		"100.63.255.255",
		"100.128.0.1",
		"2606:4700:4700::1111", // public IPv6
	}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("test address %q did not parse", raw)
		}
		if guard.BlockedIP(ip) {
			t.Errorf("BlockedIP(%s) = true, want false", raw)
		}
	}
}

func TestAllowPrivateNetworksDisablesChecks(t *testing.T) {
	guard := New(WithAllowPrivateNetworks(true))
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "::1", "fe80::1"} {
		if guard.BlockedIP(net.ParseIP(raw)) {
			t.Errorf("BlockedIP(%s) = true with checks disabled", raw)
		}
	}
}

// The client the API actually uses must refuse to reach an httptest server on
// loopback.
func TestClientRefusesLoopbackServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New().Client(2 * time.Second)
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("guarded client reached a loopback server")
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("error = %v, want ErrBlockedAddress", err)
	}
}

// A hostname that resolves to loopback must be refused too (this is the path a
// rebinding or "localhost" trick takes).
func TestClientRefusesHostnameResolvingToLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Rewrite the URL host to "localhost" so the dialer must resolve a name.
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	client := New().Client(2 * time.Second)
	_, err = client.Get("http://localhost:" + port + "/")
	if err == nil {
		t.Fatal("guarded client reached localhost")
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("error = %v, want ErrBlockedAddress", err)
	}
}

func TestClientAllowsLoopbackWhenPermitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := New(WithAllowPrivateNetworks(true)).Client(2 * time.Second)
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("permissive client failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestValidateURL(t *testing.T) {
	guard := New()

	if err := guard.ValidateURL("http://127.0.0.1:8080/"); err == nil {
		t.Error("ValidateURL accepted a loopback literal")
	}
	if err := guard.ValidateURL("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("ValidateURL accepted the metadata address")
	}
	if err := guard.ValidateURL("file:///etc/passwd"); err == nil {
		t.Error("ValidateURL accepted a non-http scheme")
	}
	if err := guard.ValidateURL(""); err == nil {
		t.Error("ValidateURL accepted an empty URL")
	}
	if err := guard.ValidateURL("http://"); err == nil {
		t.Error("ValidateURL accepted a hostless URL")
	}
	if err := guard.ValidateURL("http://localhost/"); err == nil {
		t.Error("ValidateURL accepted localhost")
	}
	if err := guard.ValidateURL("http://93.184.216.34/"); err != nil {
		t.Errorf("ValidateURL rejected a public IP: %v", err)
	}

	permissive := New(WithAllowPrivateNetworks(true))
	if err := permissive.ValidateURL("http://127.0.0.1/"); err != nil {
		t.Errorf("permissive ValidateURL rejected loopback: %v", err)
	}
}

func TestDefaultHonoursEnvToggle(t *testing.T) {
	t.Setenv("SSRF_PROTECTION", "false")
	if err := Default().ValidateURL("http://127.0.0.1/"); err != nil {
		t.Fatalf("SSRF_PROTECTION=false should disable the guard: %v", err)
	}

	t.Setenv("SSRF_PROTECTION", "")
	if err := Default().ValidateURL("http://127.0.0.1/"); err == nil {
		t.Fatal("an unset SSRF_PROTECTION should keep the guard enabled")
	}

	t.Setenv("SSRF_PROTECTION", "true")
	if err := Default().ValidateURL("http://10.0.0.1/"); err == nil {
		t.Fatal("SSRF_PROTECTION=true should keep the guard enabled")
	}
}
