package routes

import (
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
	"github.com/gin-gonic/gin"
)

// The no-op handlers below satisfy their interfaces by embedding the interface
// itself (leaving it nil). Only AssignRoutes is exercised, which binds the
// method values without calling them, so a test can build the FULL route table
// — middleware, guards and all — without a database, a WhatsApp client or a
// logger. Any route actually invoked with a no-op handler would panic, so the
// smoke test only hits unauthenticated/health routes (which never reach a
// handler stub) and inspects the registered table.
type (
	noopInstanceHandler struct {
		instance_handler.InstanceHandler
	}
	noopUserHandler      struct{ user_handler.UserHandler }
	noopSendHandler      struct{ send_handler.SendHandler }
	noopMessageHandler   struct{ message_handler.MessageHandler }
	noopChatHandler      struct{ chat_handler.ChatHandler }
	noopGroupHandler     struct{ group_handler.GroupHandler }
	noopCallHandler      struct{ call_handler.CallHandler }
	noopCommunityHandler struct {
		community_handler.CommunityHandler
	}
	noopLabelHandler      struct{ label_handler.LabelHandler }
	noopNewsletterHandler struct {
		newsletter_handler.NewsletterHandler
	}
	noopServerHandler  struct{ server_handler.ServerHandler }
	noopTypebotHandler struct{ typebot_handler.TypebotHandler }
)

// NewTestRouter builds a Gin engine with the complete production route table
// (AssignRoutes) wired to no-op handlers, for routing and middleware tests.
//
// It exists because pkg/routes had zero test coverage: a mis-wired path or a
// route mounted under the wrong middleware group was invisible to the suite.
// Tests in this or any other package can call it and then assert on
// engine.Routes() or drive requests through the real middleware chain.
//
// It is a _test.go file so it is compiled only for tests — it must not ship in
// the production binary. Tests in this package call it and then assert on
// engine.Routes() or drive requests through the real middleware chain.
func NewTestRouter(cfg *config.Config, authMiddleware auth_middleware.Middleware) *gin.Engine {
	gin.SetMode(gin.TestMode)
	eng := gin.New()

	r := &Routes{
		config:                  cfg,
		authMiddleware:          authMiddleware,
		jidValidationMiddleware: auth_middleware.NewJIDValidationMiddleware(),
		instanceHandler:         noopInstanceHandler{},
		userHandler:             noopUserHandler{},
		sendHandler:             noopSendHandler{},
		messageHandler:          noopMessageHandler{},
		chatHandler:             noopChatHandler{},
		groupHandler:            noopGroupHandler{},
		callHandler:             noopCallHandler{},
		communityHandler:        noopCommunityHandler{},
		labelHandler:            noopLabelHandler{},
		newsletterHandler:       noopNewsletterHandler{},
		// PollHandler is a concrete struct, not an interface, so it cannot be
		// embedded; the zero value is enough to bind its method value.
		pollHandler:    &poll_handler.PollHandler{},
		serverHandler:  noopServerHandler{},
		typebotHandler: noopTypebotHandler{},
		sendGuard:      httpguard.NewSendGuard(cfg.SendRateLimitPerMinute, cfg.SendMaxConcurrent),
	}

	r.AssignRoutes(eng)
	return eng
}
