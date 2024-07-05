package main

import (
	"flag"
	"log"
	"os"

	"github.com/Zapbox-API/evolution-go/pkg/config"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	instance_repository "github.com/Zapbox-API/evolution-go/pkg/instances/repository"
	instance_service "github.com/Zapbox-API/evolution-go/pkg/instances/service"
	message_model "github.com/Zapbox-API/evolution-go/pkg/messages/model"
	message_repository "github.com/Zapbox-API/evolution-go/pkg/messages/repository"
	"github.com/Zapbox-API/evolution-go/pkg/middlewares"
	"github.com/Zapbox-API/evolution-go/pkg/routes"
	session_handler "github.com/Zapbox-API/evolution-go/pkg/sessions/handler"
	session_service "github.com/Zapbox-API/evolution-go/pkg/sessions/service"
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

	routes.NewRouter(
		session_handler.NewSessionHandler(
			session_service.NewSessionService(
				instanceRepository,
				killChannel,
				clientPointer,
				linkingCodeEventChannel,
				whatsmeowService,
			), config),
		middlewares.NewMiddleware(config, instance_service.NewInstanceService(instanceRepository)),
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
