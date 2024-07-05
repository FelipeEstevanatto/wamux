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
	routes := eng.Group("/instance")
	{
		routes.Use(r.middleware.AuthAdmin)
		{
			routes.POST("/create", r.sessionHandler.Create)
			routes.GET("/fetchInstances", r.sessionHandler.All)
			routes.DELETE("/delete/:instanceName", r.sessionHandler.Delete)
			routes.DELETE("/proxy/:instanceName", r.sessionHandler.DeleteProxy)
		}

		routes.Use(r.middleware.Auth)
		{
			routes.POST("/connect", r.sessionHandler.Connect)
			routes.GET("/connectionState", r.sessionHandler.Status)
			routes.POST("/disconnect", r.sessionHandler.Disconnect)
			routes.DELETE("/logout", r.sessionHandler.Logout)
			routes.GET("/qr", r.sessionHandler.Qr)
			routes.POST("/pair", r.sessionHandler.Pair)
		}

	}

}

func NewRouter(sessionHandler sessions_handler.SessionHandler, middleware middlewares.Middleware) *Routes {
	return &Routes{sessionHandler: sessionHandler, middleware: middleware}
}
