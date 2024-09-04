package websocket_handler

import (
	"fmt"
	"net/http"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
	"go.mau.fi/whatsmeow/types"
)

type WebsocketHandler interface {
	HandleWS(ctx *gin.Context)
}

type websocketHandler struct {
	clientPointer map[string]whatsmeow_service.ClientInfo
	upgrader      websocket.Upgrader
	clientMap     map[*websocket.Conn]bool
	client        map[string]*websocket.Conn
}

func (w *websocketHandler) HandleWS(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	info, ok := w.clientPointer[instance.Id]
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "no session found"})
		return
	}

	w.upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := w.upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		logger.LogError("error upgrade connection: %v", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	w.clientMap[conn] = true
	w.client[instance.Id] = conn

	info.WSConn = conn

	logger.LogInfo("before %v", w.clientPointer[instance.Id])
	w.clientPointer[instance.Id] = info
	logger.LogInfo("after %v", w.clientPointer[instance.Id])

	handleWebSocketMessages(conn, instance.Id, w)
}

func handleWebSocketMessages(conn *websocket.Conn, userID string, s *websocketHandler) {
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read: ", err)
			return
		}
		fmt.Println("msg: ", string(msg))

		handleWebSocketMessage(conn, userID, msgType, msg, s)
	}
}

func handleWebSocketMessage(conn *websocket.Conn, userID string, msgType int, msg []byte, s *websocketHandler) {
	switch msgType {
	case websocket.TextMessage:
		switch string(msg) {
		case "ping":
			handlePing(conn)
		case "status":
			handleStatus(conn, userID, s)
		case "close":
			handleClose(conn, s)
		}
	}
}

func handlePing(conn *websocket.Conn) {
	err := conn.WriteMessage(websocket.TextMessage, []byte("pong"))
	if err != nil {
		fmt.Println("write: ", err)
	}
}

func handleStatus(conn *websocket.Conn, userID string, s *websocketHandler) {
	info, ok := s.clientPointer[userID]
	if !ok {
		fmt.Println("not ok client is empty for userid: ", userID)
		return
	}

	if info.WAClient == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("not logged in"))
		return
	}

	isConnected := info.WAClient.IsConnected()
	isLoggedIn := info.WAClient.IsLoggedIn()
	var myJid *types.JID
	if isLoggedIn {
		myJid = info.WAClient.Store.ID
	}

	response := map[string]interface{}{"Connected": isConnected, "LoggedIn": isLoggedIn, "Jid": myJid}
	err := conn.WriteJSON(response)
	if err != nil {
		fmt.Println("write: ", err)
	}
}

func handleClose(conn *websocket.Conn, s *websocketHandler) {
	for c := range s.clientMap {
		if c == conn {
			delete(s.clientMap, c)
			break
		}
	}

	for k, v := range s.clientPointer {
		if v.WSConn == conn {
			info, ok := s.clientPointer[k]
			if !ok {
				fmt.Println("cant close client for user: ", k)
			}

			info.WSConn = nil
			s.clientPointer[k] = info
			break
		}
	}

	err := conn.Close()
	if err != nil {
		fmt.Println("close: ", err)
	}

}

func NewWebsocketHandler(
	clientPointer map[string]whatsmeow_service.ClientInfo,
	upgrader websocket.Upgrader,
	clientMap map[*websocket.Conn]bool,
	client map[string]*websocket.Conn,
) WebsocketHandler {
	return &websocketHandler{
		clientPointer: clientPointer,
		upgrader:      upgrader,
		clientMap:     clientMap,
		client:        client,
	}
}
