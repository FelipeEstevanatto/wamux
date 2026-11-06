package message_handler

import (
	"mime"
	"net/url"
	"strings"
	"testing"
	"time"
)

func signedParts(t *testing.T, secret, instance, message string, now time.Time) (string, string) {
	t.Helper()
	link, err := url.Parse(downloadLink(secret, instance, message, now))
	if err != nil {
		t.Fatal(err)
	}
	return link.Query().Get("expires"), link.Query().Get("sig")
}

func TestDownloadLinkWorksUntilItExpires(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	expires, sig := signedParts(t, "key", "inst", "msg", now)

	if !validDownload("key", "inst", "msg", expires, sig, now) {
		t.Fatal("a fresh link was refused")
	}
	if !validDownload("key", "inst", "msg", expires, sig, now.Add(downloadLinkTTL-time.Second)) {
		t.Fatal("refused before it expired")
	}
	if validDownload("key", "inst", "msg", expires, sig, now.Add(downloadLinkTTL)) {
		t.Fatal("accepted after it expired")
	}
}

// A link opens one file of one instance, and nothing else.
func TestDownloadLinkCannotBeRepurposed(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	expires, sig := signedParts(t, "key", "inst", "msg", now)

	for name, valid := range map[string]bool{
		"other message":  validDownload("key", "inst", "other", expires, sig, now),
		"other instance": validDownload("key", "other", "msg", expires, sig, now),
		"other key":      validDownload("other", "inst", "msg", expires, sig, now),
		"later expiry":   validDownload("key", "inst", "msg", "9999999999", sig, now),
		"bad expiry":     validDownload("key", "inst", "msg", "soon", sig, now),
		"no signature":   validDownload("key", "inst", "msg", expires, "", now),
		"no instance":    validDownload("key", "", "msg", expires, sig, now),
		"no key":         validDownload("", "inst", "msg", expires, signDownload("", "inst", "msg", 9999999999), now),
	} {
		if valid {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAttachmentHeader(t *testing.T) {
	for name, tc := range map[string]struct{ input, want string }{
		"plain":          {"report.pdf", "report.pdf"},
		"no name":        {"", "fallback"},
		"blank":          {"   ", "fallback"},
		"a path":         {"../../etc/passwd", "passwd"},
		"windows path":   {`C:\temp\a.txt`, "a.txt"},
		"quotes":         {`a"b.txt`, `a"b.txt`},
		"accents":        {"relatório.pdf", "relatório.pdf"},
		"header attempt": {"a\r\nX-Evil: 1.txt", "a\r\nX-Evil: 1.txt"},
	} {
		t.Run(name, func(t *testing.T) {
			header := attachmentHeader(tc.input, "fallback")
			if strings.ContainsAny(header, "\r\n") {
				t.Fatalf("line break in header: %q", header)
			}
			if tc.want == "a\r\nX-Evil: 1.txt" {
				return // Only that it cannot break out of the header matters.
			}
			kind, params, err := mime.ParseMediaType(header)
			if err != nil {
				t.Fatalf("%q: %v", header, err)
			}
			if kind != "attachment" || params["filename"] != tc.want {
				t.Fatalf("%q gave %q %q", tc.input, kind, params["filename"])
			}
		})
	}
}
