package routes

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/felipeestevanatto/wamux/docs"
	call_handler "github.com/felipeestevanatto/wamux/pkg/call/handler"
	chat_handler "github.com/felipeestevanatto/wamux/pkg/chat/handler"
	cloud_handler "github.com/felipeestevanatto/wamux/pkg/cloud/handler"
	community_handler "github.com/felipeestevanatto/wamux/pkg/community/handler"
	config "github.com/felipeestevanatto/wamux/pkg/config"
	group_handler "github.com/felipeestevanatto/wamux/pkg/group/handler"
	"github.com/felipeestevanatto/wamux/pkg/httpguard"
	"github.com/felipeestevanatto/wamux/pkg/idempotency"
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

// servePage serves an HTML page that must never go stale after a deploy. Without
// a Cache-Control header browsers guess a lifetime from Last-Modified, so a page
// untouched for weeks stays "fresh" for days. no-cache still allows a cheap 304.
func servePage(file string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.File(file)
	}
}

func (r *Routes) uploadGuard() gin.HandlerFunc {
	limit := int64(config.DefaultMaxUploadBytes)
	if r.config != nil && r.config.MaxUploadBytes > 0 {
		limit = r.config.MaxUploadBytes
	}
	return uploadGuard(limit)
}

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
	idempotency             *idempotency.Handler
	cloudHandler            *cloud_handler.Handler
}

// SetIdempotency installs the Idempotency-Key middleware and the operation
// status route. It is called before AssignRoutes; nil keeps the old behaviour.
func (r *Routes) SetIdempotency(h *idempotency.Handler) {
	r.idempotency = h
}

// withIdempotency runs the Idempotency-Key middleware in front of a JSON send.
// Without a configured handler the route behaves as before.
func (r *Routes) withIdempotency(handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if r.idempotency == nil {
		return handlers
	}
	out := make([]gin.HandlerFunc, 0, len(handlers)+1)
	out = append(out, r.idempotency.Middleware())
	return append(out, handlers...)
}

// loopbackOnly admits a scrape only from a loopback TCP peer. It deliberately
// uses RemoteAddr rather than ClientIP: a forwarded header must not turn a
// public request into a local one.
func loopbackOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		if err != nil {
			host = c.Request.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "metrics are restricted to loopback"})
			return
		}
		c.Next()
	}
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
	eng.GET("/manager/*any", servePage("manager/dist/index.html"))
	eng.GET("/manager", servePage("manager/dist/index.html"))

	// Public: the signed link is the credential, because a browser download cannot
	// send an API key.
	eng.GET("/download/:messageId", r.messageHandler.DownloadStoredMedia)

	// Meta's Cloud API webhook is public by design. POST is authenticated by
	// X-Hub-Signature-256, not by a WaMux API key.
	if r.cloudHandler != nil {
		eng.GET("/webhook/cloud", r.cloudHandler.WebhookVerify)
		eng.POST("/webhook/cloud", r.cloudHandler.WebhookReceive)
		eng.GET("/webhook/cloud/:appId", r.cloudHandler.WebhookVerify)
		eng.POST("/webhook/cloud/:appId", r.cloudHandler.WebhookReceive)
		eng.GET("/cloud/embedded/callback", r.cloudHandler.EmbeddedCallback)
		r.cloudHandler.RegisterPublicFeatureRoutes(eng)
	}

	if r.cloudHandler != nil {
		adminCloud := eng.Group("/cloud/admin")
		adminCloud.Use(r.authMiddleware.AuthAdmin)
		adminCloud.GET("/instances/:instanceId/diagnostics", r.cloudHandler.AdminDiagnostics)
	}

	eng.GET("/", r.serverHandler.Root)
	eng.GET("/server/ok", r.serverHandler.ServerOk)
	// Observability. /server/health is a compact JSON readiness probe; /metrics
	// is the Prometheus text format. Both are public (no apikey) like the other
	// health endpoints, and expose no message contents — only counts.
	eng.GET("/server/health", r.serverHandler.HealthHandler)
	metricsAuth := "admin"
	if r.config != nil {
		metricsAuth = strings.ToLower(strings.TrimSpace(r.config.MetricsAuth))
	}
	switch metricsAuth {
	case "none":
		eng.GET("/metrics", r.serverHandler.MetricsHandler)
	case "loopback":
		eng.GET("/metrics", loopbackOnly(), r.serverHandler.MetricsHandler)
	default:
		eng.GET("/metrics", r.authMiddleware.AuthAdmin, r.serverHandler.MetricsHandler)
	}

	// The dashboard lives in the manager now; old links and bookmarks still land.
	eng.GET("/dashboard", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/manager")
	})
	// The system/message metrics the dashboard reads. Auth: Global API Key.
	eng.GET("/server/stats", r.authMiddleware.AuthAdmin, r.serverHandler.Stats)
	eng.GET("/server/audit", r.authMiddleware.AuthAdmin, r.serverHandler.Audit)

	routes := eng.Group("/instance")
	{
		routes.Use(r.authMiddleware.AuthAdmin)
		{
			routes.POST("/create", r.instanceHandler.Create)
			routes.GET("/all", r.instanceHandler.All)
			routes.GET("/inventory", r.instanceHandler.Inventory)
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
			if r.cloudHandler != nil {
				// Plan-compatible aliases; the token identifies the instance.
				routes.GET("/cloud/credentials", r.cloudHandler.GetCredentials)
				routes.PUT("/cloud/credentials", r.cloudHandler.SetCredentials)
				routes.POST("/cloud/verify", r.cloudHandler.Verify)
				routes.POST("/cloud/register", r.cloudHandler.Register)
				routes.POST("/cloud/deregister", r.cloudHandler.Deregister)
				routes.POST("/cloud/embedded/start", r.cloudHandler.StartEmbeddedSignup)
				routes.GET("/cloud/embedded/assets", r.cloudHandler.EmbeddedAssets)
				routes.POST("/cloud/embedded/select", r.cloudHandler.SelectEmbeddedAsset)
				routes.POST("/cloud/read", r.cloudHandler.MarkRead)
				routes.GET("/cloud/diagnostics", r.cloudHandler.Diagnostics)
				routes.GET("/cloud/contacts/:contactId", r.cloudHandler.GetContact)
				routes.POST("/cloud/contacts/:contactId/reconcile", r.cloudHandler.ReconcileContact)
				routes.GET("/cloud/identity-events", r.cloudHandler.ListIdentityEvents)
				routes.GET("/cloud/account-events", r.cloudHandler.ListAccountEvents)
				routes.GET("/cloud/account-events/:id", r.cloudHandler.GetAccountEvent)
				routes.GET("/cloud/business/phone-numbers", r.cloudHandler.ListBusinessPhoneNumbers)
				routes.GET("/cloud/business/phone-numbers/:id", r.cloudHandler.GetBusinessPhone)
				routes.POST("/cloud/business/phone-numbers/:id/register", r.cloudHandler.RegisterBusinessPhone)
				routes.POST("/cloud/business/phone-numbers/:id/deregister", r.cloudHandler.DeregisterBusinessPhone)
				routes.POST("/cloud/business/phone-numbers/:id/request-code", r.cloudHandler.RequestBusinessCode)
				routes.POST("/cloud/business/phone-numbers/:id/verify-code", r.cloudHandler.VerifyBusinessCode)
				routes.GET("/cloud/business/waba", r.cloudHandler.GetBusinessWABA)
				routes.GET("/cloud/business/wabas", r.cloudHandler.ListBusinessWABAs)
				routes.GET("/cloud/business/analytics", r.cloudHandler.GetBusinessAnalytics)
				routes.GET("/cloud/business/catalogs", r.cloudHandler.ListBusinessCatalogs)
				routes.POST("/cloud/business/catalogs", r.cloudHandler.CreateBusinessCatalog)
				routes.GET("/cloud/business/catalogs/:id/products", r.cloudHandler.ListBusinessProducts)
				routes.POST("/cloud/business/catalogs/:id/products", r.cloudHandler.CreateBusinessProduct)

				routes.GET("/cloud/templates", r.cloudHandler.ListTemplates)
				routes.GET("/cloud/templates/:name/:language", r.cloudHandler.GetTemplate)
				routes.POST("/cloud/templates/sync", r.cloudHandler.SyncTemplates)
				routes.POST("/cloud/templates", r.cloudHandler.CreateTemplate)
				routes.PUT("/cloud/templates/:name/:language", r.cloudHandler.UpdateTemplate)
				routes.DELETE("/cloud/templates/:name/:language", r.cloudHandler.DeleteTemplate)
				r.cloudHandler.RegisterFeatureRoutes(routes.Group("/cloud"))
			}
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

	if r.cloudHandler != nil {
		apps := eng.Group("/cloud")
		{
			apps.Use(r.authMiddleware.AuthAdmin)
			apps.GET("/apps", r.cloudHandler.ListApps)
			apps.POST("/apps", r.cloudHandler.SaveApp)
			apps.PUT("/apps/:appId", r.cloudHandler.SaveApp)
			apps.DELETE("/apps/:appId", r.cloudHandler.DeleteApp)
		}
		cloudRoutes := eng.Group("/cloud")
		{
			cloudRoutes.Use(r.authMiddleware.Auth)
			cloudRoutes.GET("/credentials", r.cloudHandler.GetCredentials)
			cloudRoutes.PUT("/credentials", r.cloudHandler.SetCredentials)
			cloudRoutes.POST("/verify", r.cloudHandler.Verify)
			cloudRoutes.POST("/register", r.cloudHandler.Register)
			cloudRoutes.POST("/deregister", r.cloudHandler.Deregister)
			cloudRoutes.POST("/embedded/start", r.cloudHandler.StartEmbeddedSignup)
			cloudRoutes.GET("/embedded/assets", r.cloudHandler.EmbeddedAssets)
			cloudRoutes.POST("/embedded/select", r.cloudHandler.SelectEmbeddedAsset)
			cloudRoutes.POST("/read", r.cloudHandler.MarkRead)
			cloudRoutes.GET("/diagnostics", r.cloudHandler.Diagnostics)
			cloudRoutes.GET("/contacts/:contactId", r.cloudHandler.GetContact)
			cloudRoutes.POST("/contacts/:contactId/reconcile", r.cloudHandler.ReconcileContact)
			cloudRoutes.GET("/identity-events", r.cloudHandler.ListIdentityEvents)
			cloudRoutes.GET("/account-events", r.cloudHandler.ListAccountEvents)
			cloudRoutes.GET("/account-events/:id", r.cloudHandler.GetAccountEvent)
			cloudRoutes.GET("/business/phone-numbers", r.cloudHandler.ListBusinessPhoneNumbers)
			cloudRoutes.GET("/business/phone-numbers/:id", r.cloudHandler.GetBusinessPhone)
			cloudRoutes.POST("/business/phone-numbers/:id/register", r.cloudHandler.RegisterBusinessPhone)
			cloudRoutes.POST("/business/phone-numbers/:id/deregister", r.cloudHandler.DeregisterBusinessPhone)
			cloudRoutes.POST("/business/phone-numbers/:id/request-code", r.cloudHandler.RequestBusinessCode)
			cloudRoutes.POST("/business/phone-numbers/:id/verify-code", r.cloudHandler.VerifyBusinessCode)
			cloudRoutes.GET("/business/waba", r.cloudHandler.GetBusinessWABA)
			cloudRoutes.GET("/business/wabas", r.cloudHandler.ListBusinessWABAs)
			cloudRoutes.GET("/business/analytics", r.cloudHandler.GetBusinessAnalytics)
			cloudRoutes.GET("/business/catalogs", r.cloudHandler.ListBusinessCatalogs)
			cloudRoutes.POST("/business/catalogs", r.cloudHandler.CreateBusinessCatalog)
			cloudRoutes.GET("/business/catalogs/:id/products", r.cloudHandler.ListBusinessProducts)
			cloudRoutes.POST("/business/catalogs/:id/products", r.cloudHandler.CreateBusinessProduct)

			cloudRoutes.GET("/templates", r.cloudHandler.ListTemplates)
			cloudRoutes.GET("/templates/:name/:language", r.cloudHandler.GetTemplate)
			cloudRoutes.POST("/templates/sync", r.cloudHandler.SyncTemplates)
			cloudRoutes.POST("/templates", r.cloudHandler.CreateTemplate)
			cloudRoutes.PUT("/templates/:name/:language", r.cloudHandler.UpdateTemplate)
			cloudRoutes.DELETE("/templates/:name/:language", r.cloudHandler.DeleteTemplate)
			r.cloudHandler.RegisterFeatureRoutes(cloudRoutes)
		}
		templates := eng.Group("/template")
		{
			templates.Use(r.authMiddleware.Auth)
			templates.GET("", r.cloudHandler.ListTemplates)
			templates.POST("", r.cloudHandler.CreateTemplate)
			templates.POST("/sync", r.cloudHandler.SyncTemplates)
			templates.PUT("/:name/:language", r.cloudHandler.UpdateTemplate)
			templates.DELETE("/:name/:language", r.cloudHandler.DeleteTemplate)
		}
	}

	routes = eng.Group("/send")
	{
		routes.Use(r.authMiddleware.Auth)
		// Observing an idempotent send is not a send: it must not consume the
		// per-instance send budget below.
		if r.idempotency != nil {
			routes.GET("/operations", r.idempotency.Operations)
			routes.GET("/operations/:operationId", r.idempotency.Operation)
		}
		// Per-instance send limits (MAX_INSTANCES-style tenant protection):
		// rate limit + concurrency cap, keyed by the authenticated instance.
		if r.sendGuard != nil {
			routes.Use(r.sendGuard.Middleware())
		}
		{
			// Every JSON send replays a repeated Idempotency-Key. An album is the
			// exception: it is several WhatsApp messages, and the handler rejects
			// the header. Multipart uploads are rejected by the middleware itself.
			routes.POST("/text", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendText)...)
			routes.POST("/link", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendLink)...)
			routes.POST("/template", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberField(), r.sendHandler.SendTemplate)...)
			routes.POST("/media", r.withIdempotency(r.uploadGuard(), r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendMedia)...)
			routes.POST("/album", r.uploadGuard(), r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendAlbum)
			routes.POST("/poll", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendPoll)...)
			routes.POST("/sticker", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendSticker)...)
			routes.POST("/location", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendLocation)...)
			routes.POST("/contact", r.withIdempotency(r.jidValidationMiddleware.ValidateContactFields(), r.sendHandler.SendContact)...) // TODO: send multiple contacts
			routes.POST("/button", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendButton)...)
			routes.POST("/list", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendList)...)
			routes.POST("/carousel", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendCarousel)...)
			routes.POST("/event", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendEvent)...)
			routes.POST("/product", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendProduct)...)
			routes.POST("/product-list", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendProductList)...)
			routes.POST("/catalog", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendCatalog)...)
			routes.POST("/contact-request", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendContactRequest)...)
			routes.POST("/request-contact-info", r.withIdempotency(r.jidValidationMiddleware.ValidateNumberFieldWithFormatJid(), r.sendHandler.SendContactRequest)...)
			routes.POST("/status/text", r.withIdempotency(r.sendHandler.SendStatusText)...)
			routes.POST("/status/media", r.withIdempotency(r.uploadGuard(), r.sendHandler.SendStatusMedia)...)
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
			routes.POST("/markunread", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatMarkUnread)
			routes.POST("/mute", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatMute)
			routes.POST("/unmute", r.jidValidationMiddleware.ValidateNumberField(), r.chatHandler.ChatUnmute)
			routes.POST("/ephemeral", r.chatHandler.SetEphemeralExpiration)
			routes.POST("/history-sync", r.chatHandler.HistorySyncRequest)
			routes.POST("/lidMapping", r.chatHandler.LidMapping)
			// Read stored history back (requires DATABASE_SAVE_MESSAGES).
			routes.GET("/history", r.messageHandler.GetHistory)
			routes.GET("/history/search", r.messageHandler.SearchHistory)
			routes.GET("/history/capabilities", r.messageHandler.HistoryCapabilities)
			routes.GET("/chats", r.messageHandler.ListChats)
			routes.POST("/read", r.messageHandler.MarkChatRead)
			routes.GET("/contacts", r.messageHandler.Contacts)
			routes.GET("/senders", r.messageHandler.SenderNames)
			routes.GET("/media/:messageId", r.messageHandler.ServeMedia)
			routes.POST("/media/:messageId/link", r.messageHandler.MediaDownloadLink)
			routes.GET("/media/:messageId/preview", r.messageHandler.MediaPreview)
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

	// Unknown routes return a NestJS-style JSON 404 instead of Gin's default
	// "404 page not found" text, which is wrong for both humans and API clients.
	eng.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "Not Found",
			"response": gin.H{
				"message": []string{"Cannot " + c.Request.Method + " " + c.Request.URL.Path},
			},
		})
	})

	// Method mismatch on a known path: also 404, NestJS-style.
	eng.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "Not Found",
			"response": gin.H{
				"message": []string{"Cannot " + c.Request.Method + " " + c.Request.URL.Path},
			},
		})
	})

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
	cloudHandler *cloud_handler.Handler,
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
		cloudHandler:            cloudHandler,
		sendGuard:               httpguard.NewSendGuard(config.SendRateLimitPerMinute, config.SendMaxConcurrent),
	}
}
