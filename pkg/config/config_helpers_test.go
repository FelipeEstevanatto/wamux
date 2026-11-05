package config

import (
	"strings"
	"testing"

	config_env "github.com/felipeestevanatto/wamux/pkg/config/env"
)

func TestExtractDBNameAndAdminDSNURL(t *testing.T) {
	dbName, admin, err := extractDBNameAndAdminDSN("postgres://user:pass@host:5432/mydb?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if dbName != "mydb" {
		t.Fatalf("dbName = %q, want mydb", dbName)
	}
	if !strings.HasSuffix(admin, "/postgres?sslmode=disable") {
		t.Fatalf("admin DSN should point at the postgres database, got %q", admin)
	}
	if strings.Contains(admin, "mydb") {
		t.Fatalf("admin DSN still references the target db: %q", admin)
	}
}

func TestExtractDBNameAndAdminDSNKeyValue(t *testing.T) {
	dbName, admin, err := extractDBNameAndAdminDSN("host=h port=5432 user=u password=p dbname=mydb sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if dbName != "mydb" {
		t.Fatalf("dbName = %q, want mydb", dbName)
	}
	// Order is nondeterministic (map iteration), so assert on the field itself.
	if !strings.Contains(admin, "dbname=postgres") {
		t.Fatalf("admin DSN should carry dbname=postgres, got %q", admin)
	}
	if strings.Contains(admin, "dbname=mydb") {
		t.Fatalf("admin DSN still targets mydb: %q", admin)
	}
}

func TestExtractDBNameAndAdminDSNMissingName(t *testing.T) {
	if _, _, err := extractDBNameAndAdminDSN("host=h user=u"); err == nil {
		t.Fatal("expected error when dbname is absent")
	}
	if _, _, err := extractDBNameAndAdminDSN("host=h dbname="); err == nil {
		t.Fatal("expected error when dbname is empty")
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "a", []string{"a"}},
		{"trimmed", " a , b ", []string{"a", "b"}},
		{"drops empties", "a,,b,", []string{"a", "b"}},
		{"all empties", " , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSV(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseCSV(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseCSV(%q) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestValidateAMQPURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		ok   bool
	}{
		{"empty is allowed (disabled)", "", true},
		{"amqp", "amqp://guest:guest@localhost:5672/", true},
		{"amqps", "amqps://host:5671", true},
		{"wrong scheme", "http://host", false},
		{"missing host", "amqp://", false},
		{"unparseable", "://bad", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAMQPURL(tc.url)
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	const key = "EVO_TEST_ENV_INT"
	t.Setenv(key, "42")
	if got := envInt(key, 7); got != 42 {
		t.Fatalf("got %d, want 42", got)
	}

	t.Setenv(key, "0")
	if got := envInt(key, 7); got != 0 {
		t.Fatalf("zero is a valid value, got %d", got)
	}

	for _, raw := range []string{"", "abc", "-1"} {
		t.Setenv(key, raw)
		if got := envInt(key, 7); got != 7 {
			t.Fatalf("envInt(%q) = %d, want fallback 7", raw, got)
		}
	}
}

// whatsmeow treats a level name it does not know as DEBUG, so DEBUG_ENABLED=0 used
// to log every protocol node.
func TestWhatsmeowLogLevel(t *testing.T) {
	cases := map[string]string{
		"": "", "0": "", "false": "", "off": "", "garbage": "",
		"1": "DEBUG", "true": "DEBUG", "debug": "DEBUG",
		"INFO": "INFO", "warn": "WARN", " error ": "ERROR",
	}
	for raw, want := range cases {
		t.Setenv(config_env.WA_DEBUG, raw)
		if got := whatsmeowLogLevel(raw); got != want {
			t.Errorf("whatsmeowLogLevel(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestWebhookBase64MaxBytes(t *testing.T) {
	cases := map[string]int64{"": defaultWebhookBase64MaxBytes, "0": 0, "1048576": 1 << 20, "-1": defaultWebhookBase64MaxBytes, "x": defaultWebhookBase64MaxBytes}
	for raw, want := range cases {
		t.Setenv(config_env.WEBHOOK_BASE64_MAX_BYTES, raw)
		if got := webhookBase64MaxBytes(); got != want {
			t.Errorf("%q: got %d, want %d", raw, got, want)
		}
	}
}

// Media goes with its message unless told otherwise.
func TestMediaRetentionDays(t *testing.T) {
	cases := map[string]int{"": 365, "0": 0, "30": 30, "-1": 365, "x": 365}
	for raw, want := range cases {
		t.Setenv(config_env.MEDIA_RETENTION_DAYS, raw)
		if got := mediaRetentionDays(365); got != want {
			t.Errorf("%q: got %d, want %d", raw, got, want)
		}
	}
}
