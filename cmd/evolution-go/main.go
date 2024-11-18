package main

import (
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"
	"github.com/joho/godotenv"
	"go.mau.fi/whatsmeow"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"

	chat_handler "github.com/EvolutionAPI/evolution-go/pkg/chat/handler"
	chat_service "github.com/EvolutionAPI/evolution-go/pkg/chat/service"
	community_handler "github.com/EvolutionAPI/evolution-go/pkg/community/handler"
	community_service "github.com/EvolutionAPI/evolution-go/pkg/community/service"
	config "github.com/EvolutionAPI/evolution-go/pkg/config"
	rabbitmq_producer "github.com/EvolutionAPI/evolution-go/pkg/events/rabbitmq"
	webhook_producer "github.com/EvolutionAPI/evolution-go/pkg/events/webhook"
	group_handler "github.com/EvolutionAPI/evolution-go/pkg/group/handler"
	group_service "github.com/EvolutionAPI/evolution-go/pkg/group/service"
	instance_handler "github.com/EvolutionAPI/evolution-go/pkg/instance/handler"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	instance_repository "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	instance_service "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	label_handler "github.com/EvolutionAPI/evolution-go/pkg/label/handler"
	label_service "github.com/EvolutionAPI/evolution-go/pkg/label/service"
	message_handler "github.com/EvolutionAPI/evolution-go/pkg/message/handler"
	message_model "github.com/EvolutionAPI/evolution-go/pkg/message/model"
	message_repository "github.com/EvolutionAPI/evolution-go/pkg/message/repository"
	message_service "github.com/EvolutionAPI/evolution-go/pkg/message/service"
	auth_middleware "github.com/EvolutionAPI/evolution-go/pkg/middleware"
	newsletter_handler "github.com/EvolutionAPI/evolution-go/pkg/newsletter/handler"
	newsletter_service "github.com/EvolutionAPI/evolution-go/pkg/newsletter/service"
	routes "github.com/EvolutionAPI/evolution-go/pkg/routes"
	send_handler "github.com/EvolutionAPI/evolution-go/pkg/sendMessage/handler"
	send_service "github.com/EvolutionAPI/evolution-go/pkg/sendMessage/service"
	server_handler "github.com/EvolutionAPI/evolution-go/pkg/server/handler"
	minio_storage "github.com/EvolutionAPI/evolution-go/pkg/storage/minio"
	"github.com/EvolutionAPI/evolution-go/pkg/telemetry"
	user_handler "github.com/EvolutionAPI/evolution-go/pkg/user/handler"
	user_service "github.com/EvolutionAPI/evolution-go/pkg/user/service"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	amqp "github.com/rabbitmq/amqp091-go"
)

var devMode = flag.Bool("dev", false, "Enable development mode")

func setupRouter(db *gorm.DB, sqliteDB *sql.DB, config *config.Config, conn *amqp.Connection, exPath string) *gin.Engine {
	killChannel := make(map[string](chan bool))
	clientPointer := make(map[string]*whatsmeow.Client)
	linkingCodeEventChannel := make(chan whatsmeow_service.LinkingCodeEvent)

	rabbitmqProducer := rabbitmq_producer.NewRabbitMQProducer(conn)
	webhookProducer := webhook_producer.NewWebhookProducer(config.WebhookUrl)

	mediaStorage, err := minio_storage.NewMinioMediaStorage(
		config.MinioEndpoint,
		config.MinioAccessKey,
		config.MinioSecretKey,
		config.MinioBucket,
		config.MinioUseSSL,
	)
	if err != nil {
		log.Fatal(err)
	}

	instanceRepository := instance_repository.NewInstanceRepository(db)
	messageRepository := message_repository.NewMessageRepository(db)
	whatsmeowService := whatsmeow_service.NewWhatsmeowService(
		instanceRepository,
		message_repository.NewMessageRepository(db),
		config,
		killChannel,
		clientPointer,
		linkingCodeEventChannel,
		rabbitmqProducer,
		webhookProducer,
		sqliteDB,
		exPath,
		mediaStorage,
	)
	instanceService := instance_service.NewInstanceService(
		instanceRepository,
		killChannel,
		clientPointer,
		linkingCodeEventChannel,
		whatsmeowService,
		config,
	)
	sendMessageService := send_service.NewSendService(clientPointer, whatsmeowService, config)
	userService := user_service.NewUserService(clientPointer, whatsmeowService)
	messageService := message_service.NewMessageService(clientPointer, messageRepository)
	chatService := chat_service.NewChatService(clientPointer)
	groupService := group_service.NewGroupService(clientPointer)
	communityService := community_service.NewCommunityService(clientPointer)
	labelService := label_service.NewLabelService(clientPointer)
	newsletterService := newsletter_service.NewNewsletterService(clientPointer)

	telemetry := telemetry.NewTelemetryService()

	r := gin.Default()
	r.Use(telemetry.TelemetryMiddleware())
	routes.NewRouter(
		auth_middleware.NewMiddleware(config, instanceService),
		instance_handler.NewInstanceHandler(instanceService, config),
		user_handler.NewUserHandler(userService),
		send_handler.NewSendHandler(sendMessageService),
		message_handler.NewMessageHandler(messageService),
		chat_handler.NewChatHandler(chatService),
		group_handler.NewGroupHandler(groupService),
		community_handler.NewCommunityHandler(communityService),
		label_handler.NewLabelHandler(labelService),
		newsletter_handler.NewNewsletterHandler(newsletterService),
		server_handler.NewServerHandler(),
	).AssignRoutes(r)

	if config.ConnectOnStartup {
		go whatsmeowService.ConnectOnStartup(config.ClientName)
	}

	return r
}

func migrate(db *gorm.DB) {
	err := db.AutoMigrate(&instance_model.Instance{}, &message_model.Message{})

	if err != nil {
		log.Fatal(err)
	}
}

func initAuthDB(config *config.Config) (*sql.DB, string, error) {
	if config.PostgresAuthDB != "" {
		return nil, "", nil
	}

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	dbDirectory := exPath + "/dbdata"
	_, err = os.Stat(dbDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(dbDirectory, 0751)
		if errDir != nil {
			panic("Could not create dbdata directory")
		}
	}

	db, err := sql.Open("sqlite", exPath+"/dbdata/users.db?_pragma=foreign_keys(1)&_busy_timeout=3000")
	if err != nil {
		return nil, "", err
	}

	return db, exPath, nil
}

func checkLicense(licenseToken string) error {
	// licenseAPIURL := "https://check.evolution-api.com/check"

	// payload := map[string]string{
	// 	"token": licenseToken,
	// }
	// jsonPayload, err := json.Marshal(payload)
	// if err != nil {
	// 	return err
	// }

	// resp, err := http.Post(licenseAPIURL, "application/json", bytes.NewBuffer(jsonPayload))
	// if err != nil {
	// 	return err
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != http.StatusOK {
	// 	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	// 	return fmt.Errorf("licença inválida: %s", string(bodyBytes))
	// }

	return nil
}

// @title Evolution GO
// @version 1.0
// @description Evolution GO - whatsmeow
func main() {
	flag.Parse()
	if *devMode {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatal(err)
		}
	}

	config := config.Load()

	licenseToken := config.GlobalApiKey
	if licenseToken == "" {
		log.Fatal("GlobalApiKey não configurado")
	}

	err := checkLicense(licenseToken)
	if err != nil {
		log.Fatalf("Falha na verificação de licença")
	}

	db, err := config.CreateUsersDB()

	if err != nil {
		log.Fatal(err)
	}

	migrate(db)

	var conn *amqp.Connection

	if config.AmqpUrl != "" {
		conn, err := amqp.Dial(config.AmqpUrl)
		if err != nil {
			logger.LogFatal("Failed to connect to RabbitMQ, err: %v", err)
		}
		defer func(conn *amqp.Connection) {
			err := conn.Close()
			if err != nil {
				logger.LogFatal("Failed to close RabbitMQ connection, err: %v", err)
			}
		}(conn)
	}

	sqliteDB, exPath, err := initAuthDB(config)
	if err != nil {
		log.Fatal(err)
	}
	defer sqliteDB.Close()

	r := setupRouter(db, sqliteDB, config, conn, exPath)

	r.Run(":" + os.Getenv("SERVER_PORT"))
}
