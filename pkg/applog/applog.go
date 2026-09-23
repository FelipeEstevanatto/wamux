// Package applog is the process-wide stdout logger.
//
// It exists so the rest of the codebase can keep the package-level
// LogInfo/LogError/... call style while the underlying library moved from free
// functions (gomessguii/logger v0) to a *Logger instance (v1). The rewrite is
// contained here instead of touching ~60 call sites.
//
// It deliberately has no internal imports: pkg/config logs through it, and
// pkg/logger (the per-instance, file-backed loggers) imports pkg/config, so
// anything with a dependency on those two would create a cycle.
package applog

import (
	"os"
	"strings"

	logger "github.com/gomessguii/logger"
)

// std is the shared logger. Logging to stdout mirrors what the v0 free
// functions did; per-instance file logging lives in pkg/logger.
var std = logger.NewLogger(
	"evolution-go",
	"app",
	debugEnabled(),
	logger.WebhookConfig{},
)

// debugEnabled keeps the historical DEBUG_ENABLED=1 behaviour (v1 reads a field
// instead of the environment).
func debugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_ENABLED"))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func LogInfo(format string, v ...any)  { std.LogInfo(format, v...) }
func LogError(format string, v ...any) { std.LogError(format, v...) }
func LogWarn(format string, v ...any)  { std.LogWarn(format, v...) }
func LogDebug(format string, v ...any) { std.LogDebug(format, v...) }
func LogFatal(format string, v ...any) { std.LogFatal(format, v...) }
