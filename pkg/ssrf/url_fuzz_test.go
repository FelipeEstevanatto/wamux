package ssrf

import (
	"net/url"
	"testing"
)

// FuzzValidateURL feeds arbitrary caller-supplied URLs through the SSRF
// pre-check. The guard uses the permissive option so the check returns before
// any DNS resolution — this keeps the fuzzer offline and fast while still
// exercising the parsing and scheme/host validation.
//
// Invariant: when ValidateURL accepts a URL, it is an absolute http/https URL
// with a non-empty host. A panic, or acceptance of a malformed URL, is a bug.
func FuzzValidateURL(f *testing.F) {
	for _, s := range []string{
		"",
		"http://example.com",
		"https://example.com/path?q=1",
		"ftp://example.com",
		"http://",
		"//example.com",
		"http://127.0.0.1",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:80/",
		"not a url",
		"http://user:pass@host:8080/x",
		"javascript:alert(1)",
	} {
		f.Add(s)
	}

	g := New(WithAllowPrivateNetworks(true))

	f.Fuzz(func(t *testing.T, raw string) {
		if err := g.ValidateURL(raw); err != nil {
			return
		}
		// Accepted: it must parse and be a well-formed http(s) URL with a host.
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("ValidateURL accepted an unparseable URL %q: %v", raw, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			t.Fatalf("ValidateURL accepted scheme %q for %q", u.Scheme, raw)
		}
		if u.Hostname() == "" {
			t.Fatalf("ValidateURL accepted a host-less URL %q", raw)
		}
	})
}
