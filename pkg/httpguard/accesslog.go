package httpguard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Access log modes, from HTTP_ACCESS_LOG.
const (
	AccessLogAll    = "all"
	AccessLogErrors = "errors"
	AccessLogOff    = "off"
)

// AccessLog returns the request logger for mode. An unknown mode means errors.
//
// Logging every request was 22% of CPU under HTTP load, and a line per 2xx says
// little that /metrics does not. The default keeps what is worth reading:
// failures. Health and scrape routes are never logged, and neither are 429s, so a
// flood cannot turn into a flood of log lines.
func AccessLog(mode string) gin.HandlerFunc {
	if mode == AccessLogOff {
		return func(c *gin.Context) { c.Next() }
	}

	return gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: formatAccessLog,
		Skip: func(c *gin.Context) bool {
			if _, health := HealthPaths[c.FullPath()]; health {
				return true
			}
			status := c.Writer.Status()
			if status == http.StatusTooManyRequests {
				return true
			}
			return mode != AccessLogAll && status < http.StatusBadRequest
		},
	})
}

func formatAccessLog(p gin.LogFormatterParams) string {
	return fmt.Sprintf("[HTTP] %s | %3d | %13v | %15s | %-7s %s\n",
		p.TimeStamp.Format("2006/01/02 15:04:05"), p.StatusCode, p.Latency, p.ClientIP, p.Method, redactQuery(p.Path))
}

// redactQuery drops a query string that carries a credential. /ws takes the
// global API key as ?token=, which used to be written to the log in plaintext.
func redactQuery(path string) string {
	base, query, found := strings.Cut(path, "?")
	if found && strings.Contains(strings.ToLower(query), "token") {
		return base + "?<redacted>"
	}
	return path
}
