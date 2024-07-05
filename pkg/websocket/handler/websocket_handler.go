package websocket_handler

import "github.com/gin-gonic/gin"

type WebsocketHandler interface {
	HandleWS(ctx *gin.Context)
}

type websocketHandler struct {
}

// HandleWS implements WebsocketHandler.
func (w *websocketHandler) HandleWS(ctx *gin.Context) {
	panic("unimplemented")
}

func NewWebsocketHandler() WebsocketHandler {
	return &websocketHandler{}
}
