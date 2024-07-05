package routes

import (
	"net/http"

	"github.com/Zapbox-API/evolution-go/pkg/middlewares"
	sessions_handler "github.com/Zapbox-API/evolution-go/pkg/sessions/handler"
	"github.com/gin-gonic/gin"
)

type Routes struct {
	sessionHandler sessions_handler.SessionHandler
	middleware     middlewares.Middleware
}

func (r *Routes) AssignRoutes(eng *gin.Engine) {
	eng.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	session := eng.Group("/session")
	{
		session.Use(r.middleware.AuthAdmin)
		{
			session.POST("/init", r.sessionHandler.Init)
			session.GET("/all", r.sessionHandler.All)
			session.DELETE("/delete/:id", r.sessionHandler.Delete)
			session.DELETE("/proxy/:id", r.sessionHandler.DeleteProxy)
		}

		session.Use(r.middleware.Auth)
		{
			session.POST("/connect", r.sessionHandler.Connect)
			session.POST("/disconnect", r.sessionHandler.Disconnect)
			session.DELETE("/logout", r.sessionHandler.Logout)
			session.GET("/status", r.sessionHandler.Status)
			session.GET("/qr", r.sessionHandler.Qr)
			session.POST("/pair", r.sessionHandler.Pair)
		}

	}

}

func NewRouter(sessionHandler sessions_handler.SessionHandler, middleware middlewares.Middleware) *Routes {
	return &Routes{sessionHandler: sessionHandler, middleware: middleware}
}
