package routes

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/EvolutionAPI/evolution-go/docs"
	chat_handler "github.com/EvolutionAPI/evolution-go/pkg/chat/handler"
	community_handler "github.com/EvolutionAPI/evolution-go/pkg/community/handler"
	group_handler "github.com/EvolutionAPI/evolution-go/pkg/group/handler"
	instance_handler "github.com/EvolutionAPI/evolution-go/pkg/instance/handler"
	label_handler "github.com/EvolutionAPI/evolution-go/pkg/label/handler"
	message_handler "github.com/EvolutionAPI/evolution-go/pkg/message/handler"
	auth_middleware "github.com/EvolutionAPI/evolution-go/pkg/middleware"
	newsletter_handler "github.com/EvolutionAPI/evolution-go/pkg/newsletter/handler"
	send_handler "github.com/EvolutionAPI/evolution-go/pkg/sendMessage/handler"
	server_handler "github.com/EvolutionAPI/evolution-go/pkg/server/handler"
	user_handler "github.com/EvolutionAPI/evolution-go/pkg/user/handler"
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
	serverHandler     server_handler.ServerHandler
}

func (r *Routes) AssignRoutes(eng *gin.Engine) {
	eng.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	eng.POST("/server/ok", r.serverHandler.ServerOk)

	routes := eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.AuthAdmin)
		{
			routes.POST("/create", r.instanceHandler.Create)
			routes.GET("/all", r.instanceHandler.All)
			routes.GET("/info/:instanceId", r.instanceHandler.Info)
			routes.DELETE("/delete/:instanceId", r.instanceHandler.Delete)
			routes.DELETE("/proxy/:instanceId", r.instanceHandler.DeleteProxy)
		}
	}

	routes = eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/connect", r.instanceHandler.Connect)
			routes.GET("/status", r.instanceHandler.Status)
			routes.GET("/qr", r.instanceHandler.Qr)
			routes.POST("/pair", r.instanceHandler.Pair)
			routes.POST("/disconnect", r.instanceHandler.Disconnect)
			routes.DELETE("/logout", r.instanceHandler.Logout)
		}
	}

	routes = eng.Group("/send")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/text", r.sendHandler.SendText)
			routes.POST("/link", r.sendHandler.SendLink)
			routes.POST("/media", r.sendHandler.SendMedia)
			routes.POST("/poll", r.sendHandler.SendPoll)
			routes.POST("/sticker", r.sendHandler.SendSticker)
			routes.POST("/location", r.sendHandler.SendLocation)
			routes.POST("/contact", r.sendHandler.SendContact) // TODO: send multiple contacts
			routes.POST("/button", r.sendHandler.SendButton)
			routes.POST("/list", r.sendHandler.SendList)
			// TODO: send status
		}
	}
	routes = eng.Group("/user")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/info", r.userHandler.GetUser)
			routes.POST("/check", r.userHandler.CheckUser)
			routes.POST("/avatar", r.userHandler.GetAvatar)
			routes.GET("/contacts", r.userHandler.GetContacts)
			routes.GET("/privacy", r.userHandler.GetPrivacy)
			routes.POST("/block", r.userHandler.BlockContact)
			routes.POST("/unblock", r.userHandler.UnblockContact)
			routes.GET("/blocklist", r.userHandler.GetBlockList)
			routes.POST("/profile", r.userHandler.SetProfilePicture)
			routes.POST("/profile-name", r.userHandler.SetProfileName)
		}
	}
	routes = eng.Group("/message")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/react", r.messageHandler.React)
			routes.POST("/presence", r.messageHandler.ChatPresence)
			routes.POST("/markread", r.messageHandler.MarkRead)
			routes.POST("/downloadmedia", r.messageHandler.DownloadMedia)
			routes.POST("/status", r.messageHandler.GetMessageStatus)
			routes.POST("/delete", r.messageHandler.DeleteMessageEveryone)
			routes.POST("/edit", r.messageHandler.EditMessage) // TODO: edit MediaMessage too
		}
	}
	routes = eng.Group("/chat")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/pin", r.chatHandler.ChatPin)         // TODO: not working
			routes.POST("/unpin", r.chatHandler.ChatUnpin)     // TODO: not working
			routes.POST("/archive", r.chatHandler.ChatArchive) // TODO: not working
			routes.POST("/mute", r.chatHandler.ChatMute)       // TODO: not working
			routes.POST("/history-sync", r.chatHandler.HistorySyncRequest)
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
			routes.GET("/myall", r.groupHandler.GetMyGroups) // TODO: not working
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
		serverHandler:     serverHandler,
	}
}
