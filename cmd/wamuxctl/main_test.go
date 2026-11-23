package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseArgsAllowsFlagsAnywhere(t *testing.T) {
	inv, err := parseArgs([]string{"cloud", "send", "text", "--to", "5511", "--dry-run", "extra", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.pos) != 4 || inv.pos[0] != "cloud" || inv.pos[3] != "extra" {
		t.Fatalf("pos = %#v", inv.pos)
	}
	if inv.vals["to"] != "5511" || !inv.flags["dry-run"] || !inv.flags["json"] {
		t.Fatalf("parsed = %#v", inv)
	}
}

func TestDryRunMakesNoRequestsAndRedactsToken(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	var out, errOut bytes.Buffer
	code := run([]string{
		"cloud", "send", "text", "--url", srv.URL,
		"--instance-token", "super-secret-instance-token",
		"--to", "5511999999999", "--text", "hello", "--dry-run",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("dry run made %d requests", calls.Load())
	}
	got := out.String()
	if !strings.Contains(got, "DRY RUN POST") || !strings.Contains(got, "/send/text") {
		t.Fatalf("output = %s", got)
	}
	if strings.Contains(got, "super-secret-instance-token") {
		t.Fatalf("token leaked: %s", got)
	}
}

func TestDoctorExitCodes(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "token" {
			t.Fatalf("apikey = %q", r.Header.Get("apikey"))
		}
		_, _ = w.Write([]byte(`{"data":{"tokenValid":true,"appSecretSet":true,"phoneRegistered":true,"subscribed":true}}`))
	}))
	defer healthy.Close()
	var out, errOut bytes.Buffer
	if code := run([]string{"cloud", "doctor", "--url", healthy.URL, "--instance-token", "token"}, &out, &errOut); code != 0 {
		t.Fatalf("healthy code=%d out=%s err=%s", code, out.String(), errOut.String())
	}

	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"tokenValid":false,"lastGraphError":"expired"}}`))
	}))
	defer failed.Close()
	out.Reset()
	errOut.Reset()
	if code := run([]string{"cloud", "doctor", "--url", failed.URL, "--instance-token", "token"}, &out, &errOut); code != 1 {
		t.Fatalf("failed code=%d out=%s err=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "lastGraphError=expired") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestCoverageJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"cloud", "coverage", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"operation":"send text"`) || !strings.Contains(out.String(), `"status":"implemented"`) {
		t.Fatalf("output = %s", out.String())
	}
}
