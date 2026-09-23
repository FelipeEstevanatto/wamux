// Package applog holds the process-wide logger instance.
//
// gomessguii/logger v1 replaced the v0 package-level LogInfo/LogError/... free
// functions with methods on a *Logger, so there is no global logger any more:
// this package owns the single instance every package-level call site uses, and
// call sites invoke the library's methods on it directly.
//
// It deliberately imports nothing internal: pkg/config logs through it and
// pkg/logger imports pkg/config, so anything else would create an import cycle.
package applog

import (
	"os"
	"strings"

	"github.com/gomessguii/logger"
)

// Logger is the process-wide logger, used for startup, configuration, producers
// and other process-level messages. It writes to stdout; per-instance file
// logging lives in pkg/logger.
//
// The v1 logger also supports CaptureExceptionFunc and WebhookConfig (error/warn
// notification to an HTTP endpoint); both are left off here so logging stays
// local, and can be wired from configuration if wanted.
var Logger = logger.NewLogger(
	"evolution-go",
	"app",
	debugEnabled(),
	logger.WebhookConfig{},
)

// debugEnabled keeps the historical DEBUG_ENABLED=1 behaviour: v1 reads the flag
// from the Logger instead of the environment on every call.
func debugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_ENABLED"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}
