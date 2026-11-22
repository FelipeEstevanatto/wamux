package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
)

// globalAPIKeyFile holds the generated key, next to the logs on the data volume.
const globalAPIKeyFile = "global-api-key"

// privateNetworks is who may set X-Forwarded-For by default: a reverse proxy on the
// same host, Docker network or LAN. A client reaching the server from a public address
// is never believed, and a proxy's own hop is what gets read, so nothing a client puts
// in the header can win. TRUSTED_PROXIES=none believes nobody.
var privateNetworks = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7",
}

func resolveTrustedProxies(raw string) []string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return privateNetworks
	case "none":
		return nil
	}
	return parseCSV(raw)
}

// resolveGlobalAPIKey returns the configured key or, so a new install needs no setup,
// one generated on first boot and kept on the data volume. The value is never logged.
func resolveGlobalAPIKey(configured, logDirectory string) (string, error) {
	if configured != "" {
		return configured, nil
	}

	path := filepath.Join(filepath.Dir(logDirectory), globalAPIKeyFile)
	saved, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(saved)) != "" {
		return strings.TrimSpace(string(saved)), nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	key := hex.EncodeToString(secret)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", err
	}
	applog.Logger.LogInfo("[CONFIG] GLOBAL_API_KEY is not set, so one was generated. It is in %s", path)
	return key, nil
}

func logDirectoryFromEnv(raw string) string {
	if raw == "" {
		return "./logs"
	}
	return raw
}

func requireAPIKey(key string, err error) string {
	if err != nil {
		applog.Logger.LogFatal("[CONFIG] %v", fmt.Errorf("set GLOBAL_API_KEY, or make the data directory writable: %w", err))
	}
	return key
}

// knownWeakAPIKeys are published examples and placeholders. A deployment that
// keeps one is trivially admin-compromised; warn loudly but do not refuse to
// boot, because the generated key path and existing installs must keep working.
var knownWeakAPIKeys = map[string]struct{}{
	"your-secure-api-key-here": {},
	"change-me":                {},
	"changeme":                 {},
	"password":                 {},
	"secret":                   {},
	"admin":                    {},
	"test":                     {},
	"apikey":                   {},
	"evolution-api":            {},
	"evolution":                {},
}

// warnIfWeakGlobalAPIKey logs a clear warning when GLOBAL_API_KEY is a known
// example value or shorter than 32 characters. The value itself is never logged.
func warnIfWeakGlobalAPIKey(key string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return
	}
	if _, weak := knownWeakAPIKeys[strings.ToLower(trimmed)]; weak {
		applog.Logger.LogWarn("[CONFIG] GLOBAL_API_KEY is a published example/placeholder value. Set a unique random secret (e.g. openssl rand -hex 32).")
		return
	}
	if len(trimmed) < 32 {
		applog.Logger.LogWarn("[CONFIG] GLOBAL_API_KEY is shorter than 32 characters; use a longer random secret (e.g. openssl rand -hex 32).")
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
