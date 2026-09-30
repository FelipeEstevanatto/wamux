// Package ssrf provides an http.Client/Transport whose dialer refuses to
// connect to private, loopback, link-local, carrier-grade-NAT and multicast
// addresses.
//
// # WHY
//
// Several API endpoints fetch a URL supplied by the caller (media by URL, link
// previews, button/carousel headers, stickers, avatars, group photos). Without
// a check, the server is a proxy: a caller could point it at
// http://169.254.169.254/ (the cloud metadata service), http://127.0.0.1:… (a
// local admin panel) or any host on the internal network, and read the response
// back through the API. That is a server-side request forgery (SSRF).
//
// # HOW
//
// The check runs in the transport's DialContext, not in a pre-flight URL check.
// The dialer resolves the name, validates every address, and connects to a
// validated IP directly (never re-resolving the hostname). This closes two
// holes a pre-flight-only check leaves open:
//
//   - DNS rebinding: the name cannot resolve to a public IP for the check and
//     then to 127.0.0.1 for the actual connection.
//   - Redirects: every hop of a redirect goes through DialContext again, so an
//     allowed public URL cannot bounce the client to an internal one.
//
// Set SSRF_PROTECTION=false to disable the guard (for deployments that
// legitimately fetch media from a private/internal host). It is enabled by
// default.
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrBlockedAddress is returned when a host resolves only to addresses the
// guard refuses to dial.
var ErrBlockedAddress = errors.New("ssrf: refused to connect to a private, loopback or link-local address")

// Option configures a Guard.
type Option func(*Guard)

// WithAllowPrivateNetworks disables the address checks. It exists for tests
// (httptest servers listen on 127.0.0.1) and for the SSRF_PROTECTION=false
// escape hatch; production code should not use it.
func WithAllowPrivateNetworks(allow bool) Option {
	return func(g *Guard) { g.allowPrivate = allow }
}

// Guard validates outbound addresses. The zero value blocks private networks;
// build one with New.
type Guard struct {
	allowPrivate bool
}

// New builds a guard. With no options it blocks private and special-use
// addresses.
func New(opts ...Option) *Guard {
	g := &Guard{}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Default returns the process-wide guard, honouring the SSRF_PROTECTION env
// var (SSRF_PROTECTION=false disables the checks). It is evaluated per call so
// the toggle is honoured wherever it is read from.
func Default() *Guard {
	disabled := strings.EqualFold(strings.TrimSpace(os.Getenv("SSRF_PROTECTION")), "false")
	return New(WithAllowPrivateNetworks(disabled))
}

// DefaultClient returns an *http.Client that enforces the process-wide guard.
func DefaultClient(timeout time.Duration) *http.Client {
	return Default().Client(timeout)
}

// Transport returns a clone of the default transport with the guard's dialer.
func (g *Guard) Transport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = g.dialContext
	return transport
}

// Client returns an http.Client with the guard's transport and the given
// timeout. A timeout of zero keeps the client's default (no timeout).
func (g *Guard) Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: g.Transport(),
		Timeout:   timeout,
	}
}

// BlockedIP reports whether an address is refused by the guard. It is exposed
// so callers and tests can reason about a specific address.
func (g *Guard) BlockedIP(ip net.IP) bool {
	if g.allowPrivate {
		return false
	}
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		return blockedIPv4(ip4)
	}
	return blockedIPv6(ip)
}

func blockedIPv4(ip4 net.IP) bool {
	// Common special-use ranges, plus the private blocks. 169.254.0.0/16 is
	// included explicitly: it is where the cloud metadata services live
	// (169.254.169.254 on AWS/GCP/Azure/OpenStack).
	switch {
	case ip4[0] == 0: // 0.0.0.0/8 "this network"
		return true
	case ip4[0] == 10: // 10.0.0.0/8
		return true
	case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127: // 100.64.0.0/10 CGNAT
		return true
	case ip4[0] == 127: // 127.0.0.0/8 loopback
		return true
	case ip4[0] == 169 && ip4[1] == 254: // 169.254.0.0/16 link-local / metadata
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31: // 172.16.0.0/12
		return true
	case ip4[0] == 192 && ip4[1] == 168: // 192.168.0.0/16
		return true
	case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0: // 192.0.0.0/24 IETF protocol
		return true
	case ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19): // 198.18.0.0/15 benchmark
		return true
	case ip4[0] >= 224: // 224.0.0.0/4 multicast + 240.0.0.0/4 reserved
		return true
	default:
		return false
	}
}

func blockedIPv6(ip net.IP) bool {
	switch {
	case ip.IsLoopback(): // ::1
		return true
	case ip.IsUnspecified(): // ::
		return true
	case ip.IsLinkLocalUnicast(): // fe80::/10
		return true
	case ip.IsLinkLocalMulticast(): // ff02::/16
		return true
	case ip.IsMulticast(): // ff00::/8
		return true
	case len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc: // fc00::/7 unique local
		return true
	default:
		return false
	}
}

// dialContext resolves addr, rejects it when every address is blocked, and
// connects to a validated IP directly so the name is never re-resolved.
func (g *Guard) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrf: unexpected address %q: %w", addr, err)
	}

	// A literal IP needs no resolution.
	if ip := net.ParseIP(host); ip != nil {
		if g.BlockedIP(ip) {
			return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
		}
		return g.dialer().DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ssrf: no addresses found for host %q", host)
	}

	var lastErr error
	blockedAny := false
	for _, resolved := range ips {
		if !addressMatchesNetwork(resolved.IP, network) {
			continue
		}
		if g.BlockedIP(resolved.IP) {
			blockedAny = true
			continue
		}
		conn, err := g.dialer().DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	if blockedAny {
		return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, host)
	}
	return nil, fmt.Errorf("ssrf: no dialable addresses found for host %q", host)
}

func (g *Guard) dialer() *net.Dialer {
	// Bound each attempt so a black-holed address does not pin the goroutine;
	// the http.Client timeout still applies end to end.
	return &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
}

// addressMatchesNetwork reports whether ip can be dialled over network
// ("tcp", "tcp4" or "tcp6").
func addressMatchesNetwork(ip net.IP, network string) bool {
	switch network {
	case "tcp4":
		return ip.To4() != nil
	case "tcp6":
		return ip.To4() == nil
	default:
		return true
	}
}

// ValidateURL is a fast, explicit pre-check for callers that want a clear error
// before starting a request. The dialer remains the authoritative check
// (it also covers redirects and rebinding).
func (g *Guard) ValidateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("ssrf: empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("ssrf: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("ssrf: unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("ssrf: URL has no host")
	}
	if g.allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if g.BlockedIP(ip) {
			return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return fmt.Errorf("ssrf: resolve %q: %w", host, err)
	}
	for _, resolved := range ips {
		if !g.BlockedIP(resolved.IP) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrBlockedAddress, host)
}

// ValidateURL pre-checks a URL against the process-wide guard.
func ValidateURL(raw string) error { return Default().ValidateURL(raw) }
