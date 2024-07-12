package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	chat_handler "github.com/Zapbox-API/evolution-go/pkg/chat/handler"
	community_handler "github.com/Zapbox-API/evolution-go/pkg/community/handler"
	group_handler "github.com/Zapbox-API/evolution-go/pkg/group/handler"
	instance_handler "github.com/Zapbox-API/evolution-go/pkg/instance/handler"
	label_handler "github.com/Zapbox-API/evolution-go/pkg/label/handler"
	message_handler "github.com/Zapbox-API/evolution-go/pkg/message/handler"
	auth_middleware "github.com/Zapbox-API/evolution-go/pkg/middleware"
	newsletter_handler "github.com/Zapbox-API/evolution-go/pkg/newsletter/handler"
	send_handler "github.com/Zapbox-API/evolution-go/pkg/sendMessage/handler"
	server_handler "github.com/Zapbox-API/evolution-go/pkg/server/handler"
	user_handler "github.com/Zapbox-API/evolution-go/pkg/user/handler"
	websocket_handler "github.com/Zapbox-API/evolution-go/pkg/websocket/handler"
)

type Routes struct {
	authMiddleware    auth_middleware.Middleware
	instanceHandler   instance_handler.InstanceHandler
	userHandler       user_handler.UserHandler
	sendHandler       send_handler.SendHandler
	messageHandler    message_handler.MessageHandler
	chatHandler       chat_handler.ChatHandler
	groupHandler      group_handler.GroupHandler
	communityHandler  community_handler.CommunityHandler
	labelHandler      label_handler.LabelHandler
	newsletterHandler newsletter_handler.NewsletterHandler
	websocketHandler  websocket_handler.WebsocketHandler
	serverHandler     server_handler.ServerHandler
}

func (r *Routes) AssignRoutes(eng *gin.Engine) {
	eng.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	routes := eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.AuthAdmin)
		{
			routes.POST("/create", r.instanceHandler.Create)                     // ok
			routes.GET("/fetchInstances", r.instanceHandler.All)                 // ok
			routes.DELETE("/delete/:instanceName", r.instanceHandler.Delete)     // ok
			routes.DELETE("/proxy/:instanceName", r.instanceHandler.DeleteProxy) // ok
		}

		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/connect", r.instanceHandler.Connect)       // ok
			routes.GET("/status", r.instanceHandler.Status)          // ok
			routes.POST("/disconnect", r.instanceHandler.Disconnect) // ok
			routes.DELETE("/logout", r.instanceHandler.Logout)       // ok
			routes.GET("/qr", r.instanceHandler.Qr)                  // ok
			routes.POST("/pair", r.instanceHandler.Pair)             // ok
		}

	}
	routes = eng.Group("/send")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/text", r.sendHandler.SendText)         // ok
			routes.POST("/link", r.sendHandler.SendLink)         // ok
			routes.POST("/media", r.sendHandler.SendMedia)       // ok
			routes.POST("/poll", r.sendHandler.SendPoll)         // ok
			routes.POST("/sticker", r.sendHandler.SendSticker)   // ok
			routes.POST("/location", r.sendHandler.SendLocation) // ok
			routes.POST("/contact", r.sendHandler.SendContact)   // ok
			routes.POST("/list", r.sendHandler.SendList)         // ok
		}
	}
	routes = eng.Group("/user")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/info", r.userHandler.GetUser)              // ok
			routes.POST("/check", r.userHandler.CheckUser)           // ok
			routes.GET("/avatar", r.userHandler.GetAvatar)           // ok
			routes.GET("/contacts", r.userHandler.GetContacts)       // ok
			routes.GET("/privacy", r.userHandler.GetPrivacy)         // ok
			routes.POST("/block", r.userHandler.BlockContact)        // ok
			routes.POST("/unblock", r.userHandler.UnblockContact)    // ok
			routes.GET("/blocklist", r.userHandler.GetBlockList)     // ok
			routes.POST("/profile", r.userHandler.SetProfilePicture) // ok
		}
	}
	routes = eng.Group("/message")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/react", r.messageHandler.React)
			routes.POST("/presence", r.messageHandler.ChatPresence)
			routes.POST("/markread", r.messageHandler.MarkRead)
			routes.POST("/downloadimage", r.messageHandler.DownloadImage)
			routes.POST("/status", r.messageHandler.GetMessageStatus)
			routes.POST("/delete", r.messageHandler.DeleteMessageEveryone)
			routes.POST("/edit", r.messageHandler.EditMessage)
		}
	}
	routes = eng.Group("/chat")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/pin", r.chatHandler.ChatPin)
			routes.POST("/unpin", r.chatHandler.ChatUnpin)
			routes.POST("/archive", r.chatHandler.ChatArchive)
			routes.POST("/mute", r.chatHandler.ChatMute)
		}
	}
	routes = eng.Group("/group")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.GET("/list", r.groupHandler.ListGroups)
			routes.POST("/info", r.groupHandler.GetGroupInfo)
			routes.POST("/invitelink", r.groupHandler.GetGroupInviteLink)
			routes.POST("/photo", r.groupHandler.SetGroupPhoto)
			routes.POST("/name", r.groupHandler.SetGroupName)
			routes.POST("/create", r.groupHandler.CreateGroup)
			routes.POST("/participant", r.groupHandler.UpdateParticipant)
			routes.GET("/myall", r.groupHandler.GetMyGroups)
			routes.POST("/join", r.groupHandler.JoinGroupLink)
		}
	}
	routes = eng.Group("/community")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/create", r.communityHandler.CreateCommunity)
			routes.POST("/add", r.communityHandler.CommunityAdd)
			routes.POST("/remove", r.communityHandler.CommunityRemove)
		}
	}
	routes = eng.Group("/label")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/chat", r.labelHandler.ChatLabel)
			routes.POST("/message", r.labelHandler.MessageLabel)
			routes.POST("/edit", r.labelHandler.EditLabel)
		}
	}
	routes = eng.Group("/unlabel")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/chat", r.labelHandler.ChatUnlabel)
			routes.POST("/message", r.labelHandler.MessageUnlabel)
		}
	}
	routes = eng.Group("/newsletter")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/create", r.newsletterHandler.CreateNewsletter)
			routes.GET("/list", r.newsletterHandler.ListNewsletter)
			routes.POST("/info", r.newsletterHandler.GetNewsletter)
			routes.POST("/link", r.newsletterHandler.GetNewsletterInvite)
			routes.POST("/subscribe", r.newsletterHandler.SubscribeNewsletter)
			routes.POST("/messages", r.newsletterHandler.GetNewsletterMessages)
		}
	}
	routes.POST("/ws", r.websocketHandler.HandleWS)
	routes.POST("/server/ok", r.serverHandler.ServerOk)

}

func NewRouter(
	authMiddleware auth_middleware.Middleware,
	instanceHandler instance_handler.InstanceHandler,
	userHandler user_handler.UserHandler,
	sendHandler send_handler.SendHandler,
	messageHandler message_handler.MessageHandler,
	chatHandler chat_handler.ChatHandler,
	groupHandler group_handler.GroupHandler,
	communityHandler community_handler.CommunityHandler,
	labelHandler label_handler.LabelHandler,
	newsletterHandler newsletter_handler.NewsletterHandler,
	websocketHandler websocket_handler.WebsocketHandler,
	serverHandler server_handler.ServerHandler,
) *Routes {
	return &Routes{
		authMiddleware:    authMiddleware,
		instanceHandler:   instanceHandler,
		userHandler:       userHandler,
		sendHandler:       sendHandler,
		messageHandler:    messageHandler,
		chatHandler:       chatHandler,
		groupHandler:      groupHandler,
		communityHandler:  communityHandler,
		labelHandler:      labelHandler,
		newsletterHandler: newsletterHandler,
		websocketHandler:  websocketHandler,
		serverHandler:     serverHandler,
	}
}
