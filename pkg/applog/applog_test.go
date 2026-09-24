package applog

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// captureStdLog redirects the standard library logger (which this package writes
// through) into a buffer for the duration of the test.
func captureStdLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
	return &buf
}

func TestLogLevelsEmitExpectedPrefixes(t *testing.T) {
	buf := captureStdLog(t)
	l := NewLogger("evolution-go", "app", true, WebhookConfig{})

	l.LogInfo("hello %s", "world")
	l.LogWarn("careful")
	l.LogError("boom")

	out := buf.String()
	for _, want := range []string{
		"[evolution-go]",
		"[INFO]",
		"[WARN]",
		"[ERR]",
		"hello world",
		"careful",
		"boom",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n---\n%s", want, out)
		}
	}
}

// DEBUG must be dropped unless DebugEnabled is set.
func TestDebugGatedByFlag(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		buf := captureStdLog(t)
		l := NewLogger("svc", "app", false, WebhookConfig{})
		l.LogDebug("should not appear")
		if strings.Contains(buf.String(), "should not appear") {
			t.Fatalf("debug line leaked: %q", buf.String())
		}
	})

	t.Run("enabled", func(t *testing.T) {
		buf := captureStdLog(t)
		l := NewLogger("svc", "app", true, WebhookConfig{})
		l.LogDebug("debug visible")
		if !strings.Contains(buf.String(), "debug visible") {
			t.Fatalf("debug line missing: %q", buf.String())
		}
	})
}

// The exception hook must receive the error for ERROR (and FATAL before exit).
func TestCaptureExceptionFuncCalled(t *testing.T) {
	captureStdLog(t)
	var got error
	l := NewLogger("svc", "ctx", false, WebhookConfig{})
	l.CaptureExceptionFunc = func(err error) { got = err }

	l.LogError("thing %d failed", 7)

	if got == nil {
		t.Fatal("capture hook not called")
	}
	if !strings.Contains(got.Error(), "ctx") || !strings.Contains(got.Error(), "thing 7 failed") {
		t.Fatalf("captured error missing context/message: %v", got)
	}
}

// Webhook delivery: one JSON POST per qualifying line, with the documented shape.
func TestWebhookDelivery(t *testing.T) {
	captureStdLog(t)

	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		got = append(got, m)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := WebhookConfig{URL: srv.URL, SendError: true, SendWarn: true}.WithWebhookClient(srv.Client())
	l := NewLogger("evolution-go", "app", false, cfg)

	l.LogWarn("warn msg")
	l.LogError("error msg")
	l.LogInfo("info msg") // never sent: neither SendError nor SendWarn

	if len(got) != 2 {
		t.Fatalf("expected 2 webhook calls, got %d: %+v", len(got), got)
	}
	if got[0]["serviceName"] != "evolution-go" || got[0]["level"] != "WARN" || got[0]["message"] != "warn msg" {
		t.Fatalf("unexpected warn payload: %+v", got[0])
	}
	if got[1]["level"] != "ERR" || got[1]["message"] != "error msg" {
		t.Fatalf("unexpected error payload: %+v", got[1])
	}
	if _, ok := got[0]["timestamp"]; !ok {
		t.Fatalf("payload missing timestamp: %+v", got[0])
	}
}

// A non-200 response must be reported (and must not recurse into the webhook).
func TestWebhookNon200IsLogged(t *testing.T) {
	buf := captureStdLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := WebhookConfig{URL: srv.URL, SendError: true}.WithWebhookClient(srv.Client())
	l := NewLogger("svc", "app", false, cfg)
	l.LogError("boom")

	if !strings.Contains(buf.String(), "Webhook responded with status") {
		t.Fatalf("non-200 not reported: %q", buf.String())
	}
}

// A hung webhook endpoint must not block the caller forever: a client with a
// timeout is honored.
func TestWebhookTimeoutRespected(t *testing.T) {
	captureStdLog(t)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() { close(block); srv.Close() }()

	cfg := WebhookConfig{URL: srv.URL, SendError: true}.
		WithWebhookClient(&http.Client{Timeout: 100 * time.Millisecond})
	l := NewLogger("svc", "app", false, cfg)

	done := make(chan struct{})
	go func() { l.LogError("boom"); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("LogError blocked on a hung webhook despite the client timeout")
	}
}

// Every level must be safe to call with no variadic args and with odd input.
func TestNoPanicOnOddFormatting(t *testing.T) {
	captureStdLog(t)
	l := NewLogger("svc", "app", true, WebhookConfig{})
	l.LogInfo("no args")
	l.LogWarn("one %s", "arg")

	// Pass format/args through variables so this stays a runtime exercise rather
	// than something go vet rejects: fmt prints "%!(EXTRA ...)" but must not panic.
	format, extra := "plain message", "unused"
	l.LogError(format, extra)
	l.LogDebug("")
}
