// Package applog holds the process-wide logger instance.
//
// This used to be a thin wrapper over github.com/gomessguii/logger. That package
// is published by an EvolutionAPI engineer, and this fork is deliberately
// independent of Evolution tooling, so it is implemented locally instead. The
// observable behaviour is unchanged: the same coloured "[service] [LEVEL] msg"
// line on stderr (via the standard library logger), the same DEBUG_ENABLED
// gating, and the same optional webhook / exception-capture hooks — each a
// little more robust than before (bounded webhook request, no panic on marshal).
//
// It deliberately imports nothing internal: pkg/config logs through it and
// pkg/logger imports pkg/config, so anything else would create an import cycle.
package applog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// LogLevel is the severity of a log line.
type LogLevel string

const (
	// INFO is an informational message.
	INFO LogLevel = "INFO"
	// ERR is an error message.
	ERR LogLevel = "ERR"
	// WARN is a warning.
	WARN LogLevel = "WARN"
	// DEBUG is a debug message, emitted only when DebugEnabled is set.
	DEBUG LogLevel = "DEBUG"
)

// WebhookConfig controls optional HTTP notification of WARN/ERROR/FATAL lines.
type WebhookConfig struct {
	// URL receives a JSON payload for each qualifying line. Empty disables it.
	URL string `json:"url"`
	// SendError sends ERROR lines.
	SendError bool `json:"sendError"`
	// SendFatal sends FATAL lines.
	SendFatal bool `json:"sendFatal"`
	// SendWarn sends WARN lines.
	SendWarn bool `json:"sendWarn"`

	// client bounds the webhook request; nil means a default with a timeout. A
	// missing timeout would let a hung endpoint pin a goroutine forever.
	client *http.Client
}

// AppLogger is a process-level logger. The zero value is usable but DebugDisabled;
// prefer NewLogger.
type AppLogger struct {
	// ServiceName is shown in the "[service]" prefix.
	ServiceName string
	// LogContextName labels captured exceptions.
	LogContextName string
	// DebugEnabled controls whether DEBUG lines are emitted.
	DebugEnabled bool
	// CaptureExceptionFunc, when set, is called with the wrapped error for
	// ERROR/FATAL lines.
	CaptureExceptionFunc func(err error)
	// WebhookConfig contains settings for webhook notifications.
	WebhookConfig WebhookConfig
}

// NewLogger creates a Logger.
func NewLogger(serviceName, logContextName string, debugEnabled bool, webhookConfig WebhookConfig) *AppLogger {
	return &AppLogger{
		ServiceName:    serviceName,
		LogContextName: logContextName,
		DebugEnabled:   debugEnabled,
		WebhookConfig:  webhookConfig,
	}
}

// Log emits one line at the given level. DEBUG is dropped unless enabled.
func (l *AppLogger) Log(logLevel LogLevel, format string, v ...any) {
	if logLevel == DEBUG && !l.DebugEnabled {
		return
	}

	prefix := "\033[44m[INFO]\033[0m " // blue
	switch logLevel {
	case ERR:
		prefix = "\033[41m[ERR]\033[0m " // red background
	case WARN:
		prefix = "\033[43m[WARN]\033[0m " // yellow background
	case DEBUG:
		prefix = "\033[40m\033[37m[DEBUG]\033[0m " // black/grey
	}
	log.Printf(fmt.Sprintf("\033[35m[%s]\033[0m %s%s", l.ServiceName, prefix, format), v...)
}

// LogInfo logs an informational message.
func (l *AppLogger) LogInfo(format string, v ...any) {
	l.Log(INFO, format, v...)
}

// LogError logs an error, invokes the exception hook if set, and optionally
// notifies the webhook.
func (l *AppLogger) LogError(format string, v ...any) {
	l.capture(format, v...)
	l.Log(ERR, format, v...)
	if l.WebhookConfig.SendError {
		l.sendWebhook(ERR, format, v...)
	}
}

// LogFatal logs an error and exits the process with status 1.
func (l *AppLogger) LogFatal(format string, v ...any) {
	l.capture(format, v...)
	l.Log(ERR, format, v...)
	if l.WebhookConfig.SendFatal {
		l.sendWebhook(ERR, format, v...)
	}
	os.Exit(1)
}

// LogWarn logs a warning and optionally notifies the webhook.
func (l *AppLogger) LogWarn(format string, v ...any) {
	l.Log(WARN, format, v...)
	if l.WebhookConfig.SendWarn {
		l.sendWebhook(WARN, format, v...)
	}
}

// LogDebug logs a debug message when debug logging is enabled.
func (l *AppLogger) LogDebug(format string, v ...any) {
	l.Log(DEBUG, format, v...)
}

// capture calls the exception hook, if configured, with the error wrapped in the
// log context. The original behaviour wrapped with %w; this keeps that.
func (l *AppLogger) capture(format string, v ...any) {
	if l.CaptureExceptionFunc == nil {
		return
	}
	l.CaptureExceptionFunc(fmt.Errorf("{%s} => %w", l.LogContextName, fmt.Errorf(format, v...)))
}

// defaultWebhookTimeout bounds one webhook POST.
const defaultWebhookTimeout = 10 * time.Second

// maxWebhookResponseBytes caps what is read back from the endpoint (the body is
// only checked for status, so a broken endpoint must not stream unbounded data).
const maxWebhookResponseBytes = 4 * 1024

// sendWebhook posts one JSON line to the configured endpoint. Failures are
// logged through Log (never sendWebhook) to avoid any recursion.
func (l *AppLogger) sendWebhook(logLevel LogLevel, format string, v ...any) {
	if l.WebhookConfig.URL == "" {
		return
	}

	payload := struct {
		ServiceName    string   `json:"serviceName"`
		LogContextName string   `json:"logContextName"`
		Message        string   `json:"message"`
		Level          LogLevel `json:"level"`
		Timestamp      string   `json:"timestamp"`
	}{
		ServiceName:    l.ServiceName,
		LogContextName: l.LogContextName,
		Message:        fmt.Sprintf(format, v...),
		Level:          logLevel,
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		l.Log(ERR, "Failed to marshal webhook payload: %v", err)
		return
	}

	client := l.WebhookConfig.client
	if client == nil {
		client = &http.Client{Timeout: defaultWebhookTimeout}
	}

	resp, err := client.Post(l.WebhookConfig.URL, "application/json", bytes.NewReader(jsonPayload))
	if err != nil {
		l.Log(ERR, "Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxWebhookResponseBytes))

	if resp.StatusCode != http.StatusOK {
		l.Log(ERR, "Webhook responded with status: %s", resp.Status)
	}
}

// WithWebhookClient returns a copy of the config with a custom HTTP client,
// mainly so tests can inject a short-timeout or stubbed client.
func (c WebhookConfig) WithWebhookClient(client *http.Client) WebhookConfig {
	c.client = client
	return c
}

// Logger is the process-wide logger, used for startup, configuration, producers
// and other process-level messages. It writes to stderr via the standard library
// logger; per-instance file logging lives in pkg/logger.
var Logger = NewLogger(
	"evolution-go",
	"app",
	debugEnabled(),
	WebhookConfig{},
)
func debugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_ENABLED"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}
