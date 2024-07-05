package routes

import (
	"net/http"

	instance_handler "github.com/Zapbox-API/evolution-go/pkg/instances/handler"
	"github.com/Zapbox-API/evolution-go/pkg/middlewares"
	"github.com/gin-gonic/gin"
)

type Routes struct {
	instanceHandler instance_handler.InstanceHandler
	middleware      middlewares.Middleware
}

func (r *Routes) AssignRoutes(eng *gin.Engine) {
	eng.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	routes := eng.Group("/instance")
	{
		routes.Use(r.middleware.AuthAdmin)
		{
			routes.POST("/create", r.instanceHandler.Create)
			routes.GET("/fetchInstances", r.instanceHandler.All)
			routes.DELETE("/delete/:instanceName", r.instanceHandler.Delete)
			routes.DELETE("/proxy/:instanceName", r.instanceHandler.DeleteProxy)
		}

		routes.Use(r.middleware.Auth)
		{
			routes.POST("/connect", r.instanceHandler.Connect)
			routes.GET("/connectionState", r.instanceHandler.Status)
			routes.POST("/disconnect", r.instanceHandler.Disconnect)
			routes.DELETE("/logout", r.instanceHandler.Logout)
			routes.GET("/qr", r.instanceHandler.Qr)
			routes.POST("/pair", r.instanceHandler.Pair)
		}

	}

}

func NewRouter(instanceHandler instance_handler.InstanceHandler, middleware middlewares.Middleware) *Routes {
	return &Routes{instanceHandler: instanceHandler, middleware: middleware}
}
