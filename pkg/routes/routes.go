package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/felipeestevanatto/wamux/docs"
	call_handler "github.com/felipeestevanatto/wamux/pkg/call/handler"
	chat_handler "github.com/felipeestevanatto/wamux/pkg/chat/handler"
	community_handler "github.com/felipeestevanatto/wamux/pkg/community/handler"
	config "github.com/felipeestevanatto/wamux/pkg/config"
	group_handler "github.com/felipeestevanatto/wamux/pkg/group/handler"
	"github.com/felipeestevanatto/wamux/pkg/httpguard"
	instance_handler "github.com/felipeestevanatto/wamux/pkg/instance/handler"
	label_handler "github.com/felipeestevanatto/wamux/pkg/label/handler"
	message_handler "github.com/felipeestevanatto/wamux/pkg/message/handler"
	auth_middleware "github.com/felipeestevanatto/wamux/pkg/middleware"
	newsletter_handler "github.com/felipeestevanatto/wamux/pkg/newsletter/handler"
	poll_handler "github.com/felipeestevanatto/wamux/pkg/poll/handler"
	send_handler "github.com/felipeestevanatto/wamux/pkg/sendMessage/handler"
	server_handler "github.com/felipeestevanatto/wamux/pkg/server/handler"
	typebot_handler "github.com/felipeestevanatto/wamux/pkg/typebot/handler"
	user_handler "github.com/felipeestevanatto/wamux/pkg/user/handler"
)

type Routes struct {
	config                  *config.Config
	authMiddleware          auth_middleware.Middleware
	jidValidationMiddleware *auth_middleware.JIDValidationMiddleware
	instanceHandler         instance_handler.InstanceHandler
	userHandler             user_handler.UserHandler
	sendHandler             send_handler.SendHandler
	messageHandler          message_handler.MessageHandler
	chatHandler             chat_handler.ChatHandler
	groupHandler            group_handler.GroupHandler
	callHandler             call_handler.CallHandler
	communityHandler        community_handler.CommunityHandler
	labelHandler            label_handler.LabelHandler
	newsletterHandler       newsletter_handler.NewsletterHandler
	pollHandler             *poll_handler.PollHandler
	serverHandler           server_handler.ServerHandler
	typebotHandler          typebot_handler.TypebotHandler
	sendGuard               *httpguard.SendGuard
}

func (r *Routes) AssignRoutes(eng *gin.Engine) {
	// CORS and rate limiting are applied in main.go (before the public
	// passkey/license routes) so they cover every route, not just these.

	// Swagger docs. Public (no apikey); enabled by default, disable with
	// SWAGGER_ENABLED=false to avoid exposing the API surface on a public host.
	if r.config == nil || r.config.SwaggerEnabled {
		eng.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	eng.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// Rotas para o gerenciador React (sem autenticação)
	eng.Static("/assets", "./manager/dist/assets")

	// Ajuste nas rotas do manager para suportar client-side routing do React
	eng.GET("/manager/*any", func(c *gin.Context) {
		c.File("manager/dist/index.html")
	})

	eng.GET("/manager", func(c *gin.Context) {
		c.File("manager/dist/index.html")
	})

	eng.GET("/", r.serverHandler.Root)
	eng.GET("/server/ok", r.serverHandler.ServerOk)
	// Observability. /server/health is a compact JSON readiness probe; /metrics
	// is the Prometheus text format. Both are public (no apikey) like the other
	// health endpoints, and expose no message contents — only counts.
	eng.GET("/server/health", r.serverHandler.HealthHandler)
	eng.GET("/metrics", r.serverHandler.MetricsHandler)

	// Self-hosted dashboard: static page served from the same origin (no CORS),
	// plus the system/message metrics it consumes. Auth: Global API Key.
	eng.GET("/dashboard", func(c *gin.Context) {
		c.File("manager/dist/dashboard.html")
	})
	eng.GET("/server/stats", r.authMiddleware.AuthAdmin, r.serverHandler.Stats)

	routes := eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.AuthAdmin)
		{
			routes.POST("/create", r.instanceHandler.Create)
			routes.GET("/all", r.instanceHandler.All)
			routes.GET("/info/:instanceId", r.instanceHandler.Info)
			routes.PUT("/name/:instanceId", r.instanceHandler.Rename)
			routes.DELETE("/delete/:instanceId", r.instanceHandler.Delete)
			routes.POST("/proxy/:instanceId", r.instanceHandler.SetProxy)
			routes.GET("/proxy/:instanceId", r.instanceHandler.GetProxy)
			routes.POST("/proxy/:instanceId/test", r.instanceHandler.TestProxy)
			routes.POST("/proxy/:instanceId/reconnect", r.instanceHandler.ReconnectProxy)
			routes.DELETE("/proxy/:instanceId", r.instanceHandler.DeleteProxy)
			routes.GET("/limits/:instanceId", r.instanceHandler.Limits)
			routes.POST("/forcereconnect/:instanceId", r.instanceHandler.ForceReconnect)
			routes.GET("/logs/:instanceId", r.instanceHandler.GetLogs)
			// Dashboard per-instance summary (own profile picture + contact count).
			routes.GET("/overview/:instanceId", r.serverHandler.InstanceOverview)
		}
	}

	routes = eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/connect", r.instanceHandler.Connect)
			routes.GET("/status", r.instanceHandler.Status)
			routes.GET("/qr", r.instanceHandler.Qr)
			routes.POST("/pair", r.jidValidationMiddleware.ValidateNumberField(), r.instanceHandler.Pair)
			routes.POST("/disconnect", r.instanceHandler.Disconnect)
			routes.POST("/reconnect", r.instanceHandler.Reconnect)
			routes.DELETE("/logout", r.instanceHandler.Logout)
			routes.GET("/:instanceId/advanced-settings", r.instanceHandler.GetAdvancedSettings)
			routes.PUT("/:instanceId/advanced-settings", r.instanceHandler.UpdateAdvancedSettings)
			// Per-instance webhook HMAC signing. The authenticated instance is
			// implied by the token, so no instance id is in the path.
			routes.POST("/hmac", r.instanceHandler.SetHmac)
			routes.GET("/hmac", r.instanceHandler.GetHmac)
			routes.DELETE("/hmac", r.instanceHandler.DeleteHmac)
			// Per-instance S3 media storage.
			routes.POST("/s3", r.instanceHandler.SetS3)
			routes.GET("/s3", r.instanceHandler.GetS3)
			routes.DELETE("/s3", r.instanceHandler.DeleteS3)
			routes.POST("/s3/test", r.instanceHandler.TestS3)
		}
	}

	routes = eng.Group("/send")
	{
		routes.Use(r.authMiddleware.Auth)
		// Per-instance send limits (MAX_INSTANCES-style tenant protection):
		// rate limit + concurrency cap, keyed by the authenticated instance.
		if r.sendGuard != nil {
			routes.Use(r.sendGuard.Middleware())
		}
		{
			routes.POST("/text", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendText)
			routes.POST("/link", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendLink)
			routes.POST("/media", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendMedia)
			routes.POST("/poll", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendPoll)
			routes.POST("/sticker", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendSticker)
			routes.POST("/location", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendLocation)
			routes.POST("/contact", r.jidValidationMiddleware.ValidateContactFields(), r.sendHandler.SendContact) // TODO: send multiple contacts
			routes.POST("/button", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendButton)
			routes.POST("/list", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendList)
			routes.POST("/carousel", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendCarousel)
			routes.POST("/event", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendEvent)
			routes.POST("/product", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendProduct)
			routes.POST("/status/text", r.sendHandler.SendStatusText)
			routes.POST("/status/media", r.sendHandler.SendStatusMedia)
		}
	}
	routes = eng.Group("/user")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/info", r.jidValidationMiddleware.ValidateNumberField(), r.userHandler.GetUser)
			routes.POST("/check", r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.userHandler.CheckUser)
			routes.POST("/avatar", r.jidValidationMiddleware.ValidateNumberField(), r.userHandler.GetAvatar)
			routes.GET("/contacts", r.userHandler.GetContacts)
			routes.POST("/savecontact", r.jidValidationMiddleware.ValidateNumberField(), r.userHandler.SaveContact)
			routes.POST("/contacts", r.userHandler.SaveContact) // legacy alias for /savecontact
			routes.GET("/privacy", r.userHandler.GetPrivacy)
			routes.POST("/privacy", r.userHandler.SetPrivacy)
			routes.POST("/block", r.jidValidationMiddleware.ValidateNumberField(), r.userHandler.BlockContact)
			routes.POST("/unblock", r.jidValidationMiddleware.ValidateNumberField(), r.userHandler.UnblockContact)
			routes.GET("/blocklist", r.userHandler.GetBlockList)
			routes.POST("/profilePicture", r.userHandler.SetProfilePicture)
			routes.POST("/profileName", r.userHandler.SetProfileName)
			routes.POST("/profileStatus", r.userHandler.SetProfileStatus)
			routes.POST("/lid", r.jidValidationMiddleware.ValidateJIDFields("lid", "groupJid"), r.userHandler.ResolveLid)
		}
	}
	routes = eng.Group("/message")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/react", r.jidValidationMiddleware.ValidateJIDFields("number"), r.messageHandler.React)
			routes.POST("/presence", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.ChatPresence)
			routes.POST("/subscribe", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.SubscribePresence)
			routes.POST("/markread", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.MarkRead)
			routes.POST("/markplayed", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.MarkPlayed)
			routes.POST("/downloadmedia", r.messageHandler.DownloadMedia)
			routes.POST("/status", r.messageHandler.GetMessageStatus)
			routes.POST("/delete", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.DeleteMessageEveryone)
			routes.POST("/edit", r.jidValidationMiddleware.ValidateNumberField(), r.messageHandler.EditMessage) // TODO: edit MediaMessage too
		}
	}
	routes = eng.Group("/chat")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/pin", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatPin)
			routes.POST("/unpin", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatUnpin)
			routes.POST("/archive", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatArchive)
			routes.POST("/unarchive", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatUnarchive)
			routes.POST("/mute", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatMute)
			routes.POST("/unmute", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatUnmute)
			routes.POST("/ephemeral", r.chatHandler.SetEphemeralExpiration)
			routes.POST("/history-sync", r.chatHandler.HistorySyncRequest)
			// Read stored history back (requires DATABASE_SAVE_MESSAGES).
			routes.GET("/history", r.messageHandler.GetHistory)
			routes.GET("/chats", r.messageHandler.ListChats)
			routes.GET("/contacts", r.messageHandler.Contacts)
			routes.GET("/senders", r.messageHandler.SenderNames)
			routes.GET("/media/:messageId", r.messageHandler.ServeMedia)
		}
	}
	routes = eng.Group("/group")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.GET("/list", r.groupHandler.ListGroups)
			routes.POST("/info", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.GetGroupInfo)
			routes.POST("/invitelink", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.GetGroupInviteLink)
			routes.POST("/photo", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.SetGroupPhoto)
			routes.POST("/name", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.SetGroupName)
			routes.POST("/description", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.SetGroupDescription)
			routes.POST("/create", r.jidValidationMiddleware.ValidateMultipleNumbers("participants"), r.groupHandler.CreateGroup)
			routes.POST("/participant", r.jidValidationMiddleware.ValidateMultipleNumbers("participants"), r.groupHandler.UpdateParticipant)
			routes.GET("/myall", r.groupHandler.GetMyGroups)
			routes.POST("/join", r.groupHandler.JoinGroupLink)
			routes.POST("/leave", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.LeaveGroup)
			routes.POST("/settings", r.jidValidationMiddleware.ValidateNumberField(), r.groupHandler.UpdateGroupSettings)
			// Join-request management (requires join-approval mode + admin).
			routes.POST("/requestparticipants", r.jidValidationMiddleware.ValidateJIDFields("groupJid"), r.groupHandler.GetGroupRequestParticipants)
			routes.POST("/updaterequestparticipants", r.jidValidationMiddleware.ValidateJIDFields("groupJid"), r.jidValidationMiddleware.ValidateMultipleNumbers("participants"), r.groupHandler.UpdateGroupRequestParticipants)
		}
	}
	routes = eng.Group("/call")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/reject", r.jidValidationMiddleware.ValidateNumberField(), r.callHandler.RejectCall)
		}
	}
	routes = eng.Group("/community")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/create", r.communityHandler.CreateCommunity)
			routes.POST("/add", r.jidValidationMiddleware.ValidateJIDFields("number", "communityId"), r.communityHandler.CommunityAdd)
			routes.POST("/remove", r.jidValidationMiddleware.ValidateJIDFields("number", "communityId"), r.communityHandler.CommunityRemove)
		}
	}
	routes = eng.Group("/label")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/chat", r.jidValidationMiddleware.ValidateNumberField(), r.labelHandler.ChatLabel)
			routes.POST("/message", r.labelHandler.MessageLabel)
			routes.POST("/edit", r.labelHandler.EditLabel)
			routes.GET("/list", r.labelHandler.GetLabels)
		}
	}
	routes = eng.Group("/unlabel")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/chat", r.jidValidationMiddleware.ValidateNumberField(), r.labelHandler.ChatUnlabel)
			routes.POST("/message", r.labelHandler.MessageUnlabel)
		}
	}
	routes = eng.Group("/newsletter")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("/create", r.newsletterHandler.CreateNewsletter)
			routes.GET("/list", r.newsletterHandler.ListNewsletter)
			routes.POST("/info", r.jidValidationMiddleware.ValidateJIDFields("newsletterId"), r.newsletterHandler.GetNewsletter)
			routes.POST("/link", r.jidValidationMiddleware.ValidateJIDFields("newsletterId"), r.newsletterHandler.GetNewsletterInvite)
			routes.POST("/subscribe", r.jidValidationMiddleware.ValidateJIDFields("newsletterId"), r.newsletterHandler.SubscribeNewsletter)
			routes.POST("/messages", r.jidValidationMiddleware.ValidateJIDFields("newsletterId"), r.newsletterHandler.GetNewsletterMessages)
		}
	}

	// NOVO: Rotas de Enquetes (Polls)
	routes = eng.Group("/polls")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.GET("/:pollMessageId/results", r.pollHandler.GetPollResults)
		}
	}

	// Typebot. Auth by instance token: each instance only sees its own bots and
	// sessions.
	routes = eng.Group("/typebot")
	{
		routes.Use(r.authMiddleware.Auth)
		{
			routes.POST("", r.typebotHandler.CreateBot)
			routes.GET("", r.typebotHandler.ListBots)

			// Session routes come before /:id so Gin does not treat "sessions"
			// as a bot id.
			routes.GET("/sessions", r.typebotHandler.ListSessions)
			routes.POST("/changeStatus", r.typebotHandler.ChangeSessionStatus)
			routes.PUT("/sessions/:id/status", r.typebotHandler.UpdateSessionStatus)
			routes.DELETE("/sessions/:id", r.typebotHandler.DeleteSession)

			routes.PUT("/:id", r.typebotHandler.UpdateBot)
			routes.DELETE("/:id", r.typebotHandler.DeleteBot)
		}
	}

}

func NewRouter(
	config *config.Config,
	authMiddleware auth_middleware.Middleware,
	instanceHandler instance_handler.InstanceHandler,
	userHandler user_handler.UserHandler,
	sendHandler send_handler.SendHandler,
	messageHandler message_handler.MessageHandler,
	chatHandler chat_handler.ChatHandler,
	groupHandler group_handler.GroupHandler,
	callHandler call_handler.CallHandler,
	communityHandler community_handler.CommunityHandler,
	labelHandler label_handler.LabelHandler,
	newsletterHandler newsletter_handler.NewsletterHandler,
	pollHandler *poll_handler.PollHandler,
	serverHandler server_handler.ServerHandler,
	typebotHandler typebot_handler.TypebotHandler,
) *Routes {
	return &Routes{
		config:                  config,
		authMiddleware:          authMiddleware,
		jidValidationMiddleware: auth_middleware.NewJIDValidationMiddleware(),
		instanceHandler:         instanceHandler,
		userHandler:             userHandler,
		sendHandler:             sendHandler,
		messageHandler:          messageHandler,
		chatHandler:             chatHandler,
		groupHandler:            groupHandler,
		callHandler:             callHandler,
		communityHandler:        communityHandler,
		labelHandler:            labelHandler,
		newsletterHandler:       newsletterHandler,
		pollHandler:             pollHandler,
		serverHandler:           serverHandler,
		typebotHandler:          typebotHandler,
		sendGuard:               httpguard.NewSendGuard(config.SendRateLimitPerMinute, config.SendMaxConcurrent),
	}
}
