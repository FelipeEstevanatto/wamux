package auth_middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
)

type Middleware interface {
	Auth(ctx *gin.Context)
	AuthAdmin(ctx *gin.Context)
}

type middleware struct {
	config          *config.Config
	instanceService instance_service.InstanceService
}

// authCacheTTL bounds the token-hash lookup cache in the service layer.
//
// # TIMING
//
// The instance token is looked up by its deterministic HMAC-SHA256 (see
// pkg/tokencrypt), so the database compares hashes, not the secret itself. An
// attacker cannot learn the token byte-by-byte from timing because they would
// have to invert HMAC to relate a timing difference to a candidate token. The
// admin key comparison (AuthAdmin) still uses subtle.ConstantTimeCompare because
// it compares the raw secret in-process.
func (m middleware) Auth(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	instance, err := m.instanceService.GetInstanceByToken(token)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Set("instance", instance)

	ctx.Next()
}

func (m middleware) AuthAdmin(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	// Constant-time comparison so the admin key cannot be recovered byte by
	// byte through response-timing differences.
	if subtle.ConstantTimeCompare([]byte(token), []byte(m.config.GlobalApiKey)) != 1 {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Next()
}

func NewMiddleware(config *config.Config, instanceService instance_service.InstanceService) *middleware {
	return &middleware{config: config, instanceService: instanceService}
}

// RequireAdminKey is a bare Gin middleware that requires the global API key,
// using the same constant-time comparison as AuthAdmin. It is used for routes
// mounted outside the normal router groups (e.g. the optional /debug/pprof
// handlers) so they are not left unauthenticated.
func RequireAdminKey(globalKey string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := ctx.GetHeader("apikey")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(globalKey)) != 1 {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		ctx.Next()
	}
}
