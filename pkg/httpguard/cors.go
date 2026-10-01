package httpguard

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware applies an origin allowlist.
//
// The old handler always sent `Access-Control-Allow-Origin: *` together with
// `Access-Control-Allow-Credentials: true`. That pair is invalid per the CORS
// spec — browsers reject a wildcard origin when credentials are allowed — so it
// was simultaneously unsafe and non-functional. Here the origin is echoed back
// only when it is explicitly allowed, which is what credentialed requests need.
//
// allowed may contain:
//   - "*"          → reflect any origin (no credentials). Convenient for local
//     development, but it is opt-in, never the default.
//   - exact origins ("https://app.example.com")
//   - a host suffix wildcard ("https://*.example.com")
//
// An empty allowlist disables cross-origin access entirely (same-origin only),
// which is the safe default for a self-hosted API.
func CORSMiddleware(allowed []string) gin.HandlerFunc {
	exact := make(map[string]struct{}, len(allowed))
	var suffixes []string
	reflectAny := false

	for _, origin := range allowed {
		origin = strings.TrimSpace(strings.ToLower(origin))
		if origin == "" {
			continue
		}
		switch {
		case origin == "*":
			reflectAny = true
		case strings.Contains(origin, "*."):
			// "https://*.example.com" -> match host suffix ".example.com".
			idx := strings.Index(origin, "*.")
			suffixes = append(suffixes, origin[idx+1:]) // keep the leading dot
		default:
			exact[origin] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := strings.ToLower(c.GetHeader("Origin"))

		allowedOrigin, withCredentials := matchOrigin(origin, exact, suffixes, reflectAny)
		if allowedOrigin == "" {
			// Not an allowed origin: no CORS headers. Same-origin requests are
			// unaffected (the browser does not need the headers for them).
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
				return
			}
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Accept, Cache-Control, X-Requested-With, apikey, ApiKey")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		if withCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// matchOrigin returns the value to echo in Access-Control-Allow-Origin and
// whether credentials may be allowed. A reflected wildcard never allows
// credentials (the spec forbids the combination).
func matchOrigin(origin string, exact map[string]struct{}, suffixes []string, reflectAny bool) (string, bool) {
	if origin == "" {
		return "", false
	}
	if _, ok := exact[origin]; ok {
		return origin, true
	}
	for _, suf := range suffixes {
		// origin is "scheme://host": match on the host part's suffix.
		if strings.HasSuffix(origin, suf) {
			return origin, true
		}
	}
	if reflectAny {
		return "*", false
	}
	return "", false
}
