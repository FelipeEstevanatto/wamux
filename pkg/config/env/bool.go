package config_env

import (
	"os"
	"strings"
)

// Bool reads a boolean environment variable. An unset or empty variable — or a
// value that is not recognised — yields def. Recognised true values are
// 1/true/yes/on/y/t and false values are 0/false/no/off/n/f (case-insensitive,
// surrounding whitespace ignored).
//
// This is the single place boolean flags are parsed, so every flag accepts the
// same spellings. Before this existed, flags were compared with `== "true"` or
// `!= "false"`, which meant `HTTP_COMPRESSION=TRUE` or `SWAGGER_ENABLED=0` were
// silently ignored.
func Bool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on", "y", "t":
		return true
	case "0", "false", "no", "off", "n", "f":
		return false
	default:
		return def
	}
}
