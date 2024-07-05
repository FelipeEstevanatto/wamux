package main

import (
	"flag"
	"log"
	"os"

	chat_handler "github.com/Zapbox-API/evolution-go/pkg/chat/handler"
	community_handler "github.com/Zapbox-API/evolution-go/pkg/community/handler"
	"github.com/Zapbox-API/evolution-go/pkg/config"
	group_handler "github.com/Zapbox-API/evolution-go/pkg/group/handler"
	instance_handler "github.com/Zapbox-API/evolution-go/pkg/instance/handler"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	instance_repository "github.com/Zapbox-API/evolution-go/pkg/instance/repository"
	instance_service "github.com/Zapbox-API/evolution-go/pkg/instance/service"
	label_handler "github.com/Zapbox-API/evolution-go/pkg/label/handler"
	message_handler "github.com/Zapbox-API/evolution-go/pkg/message/handler"
	message_model "github.com/Zapbox-API/evolution-go/pkg/message/model"
	message_repository "github.com/Zapbox-API/evolution-go/pkg/message/repository"
	auth_middleware "github.com/Zapbox-API/evolution-go/pkg/middleware"
	newsletter_handler "github.com/Zapbox-API/evolution-go/pkg/newsletter/handler"
	"github.com/Zapbox-API/evolution-go/pkg/routes"
	send_handler "github.com/Zapbox-API/evolution-go/pkg/sendMessage/handler"
	server_handler "github.com/Zapbox-API/evolution-go/pkg/server/handler"
	user_handler "github.com/Zapbox-API/evolution-go/pkg/user/handler"
	websocket_handler "github.com/Zapbox-API/evolution-go/pkg/websocket/handler"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

var devMode = flag.Bool("dev", false, "Enable development mode")

func setupRouter(db *gorm.DB, config *config.Config) *gin.Engine {
	r := gin.Default()

	killChannel := make(map[int](chan bool))
	clientPointer := make(map[int]whatsmeow_service.ClientInfo)
	linkingCodeEventChannel := make(chan whatsmeow_service.LinkingCodeEvent)
	instanceRepository := instance_repository.NewInstanceRepository(db)
	whatsmeowService := whatsmeow_service.NewWhatsmeowService(instanceRepository,
		message_repository.NewMessageRepository(db),
		config,
		killChannel,
		clientPointer,
		linkingCodeEventChannel,
	)
	instanceService := instance_service.NewInstanceService(
		instanceRepository,
		killChannel,
		clientPointer,
		linkingCodeEventChannel,
		whatsmeowService,
		config,
	)

	routes.NewRouter(
		auth_middleware.NewMiddleware(config, instanceService),
		instance_handler.NewInstanceHandler(instanceService, config),
		user_handler.NewUserHandler(),
		send_handler.NewSendHandler(),
		message_handler.NewMessageHandler(),
		chat_handler.NewChatHandler(),
		group_handler.NewGroupHandler(),
		community_handler.NewCommunityHandler(),
		label_handler.NewLabelHandler(),
		newsletter_handler.NewNewsletterHandler(),
		websocket_handler.NewWebsocketHandler(),
		server_handler.NewServerHandler(),
	).AssignRoutes(r)

	if config.ConnectOnStartup {
		whatsmeowService.ConnectOnStartup()
	}

	return r
}

func migrate(db *gorm.DB) {
	err := db.AutoMigrate(&instance_model.Instance{}, &message_model.Message{})

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	flag.Parse()
	if *devMode {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatal(err)
		}
	}

	config := config.Load()

	db, err := config.CreateUsersDB()

	if err != nil {
		log.Fatal(err)
	}

	migrate(db)

	r := setupRouter(db, config)

	r.Run(":" + os.Getenv("SERVER_PORT"))
}
