package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"flag"
	"fmt"
	"github.com/evolution-foundation/evolution-go/pkg/safemap"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	applog "github.com/evolution-foundation/evolution-go/pkg/applog"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mau.fi/whatsmeow"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"

	"net/http/pprof"

	call_handler "github.com/evolution-foundation/evolution-go/pkg/call/handler"
	call_service "github.com/evolution-foundation/evolution-go/pkg/call/service"
	chat_handler "github.com/evolution-foundation/evolution-go/pkg/chat/handler"
	chat_service "github.com/evolution-foundation/evolution-go/pkg/chat/service"
	community_handler "github.com/evolution-foundation/evolution-go/pkg/community/handler"
	community_service "github.com/evolution-foundation/evolution-go/pkg/community/service"
	config "github.com/evolution-foundation/evolution-go/pkg/config"
	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	nats_producer "github.com/evolution-foundation/evolution-go/pkg/events/nats"
	rabbitmq_producer "github.com/evolution-foundation/evolution-go/pkg/events/rabbitmq"
	webhook_producer "github.com/evolution-foundation/evolution-go/pkg/events/webhook"
	websocket_producer "github.com/evolution-foundation/evolution-go/pkg/events/websocket"
	group_handler "github.com/evolution-foundation/evolution-go/pkg/group/handler"
	group_service "github.com/evolution-foundation/evolution-go/pkg/group/service"
	"github.com/evolution-foundation/evolution-go/pkg/httpguard"
	instance_handler "github.com/evolution-foundation/evolution-go/pkg/instance/handler"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	label_handler "github.com/evolution-foundation/evolution-go/pkg/label/handler"
	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	label_repository "github.com/evolution-foundation/evolution-go/pkg/label/repository"
	label_service "github.com/evolution-foundation/evolution-go/pkg/label/service"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	localmedia "github.com/evolution-foundation/evolution-go/pkg/media"
	message_cleanup "github.com/evolution-foundation/evolution-go/pkg/message/cleanup"
	message_handler "github.com/evolution-foundation/evolution-go/pkg/message/handler"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	message_service "github.com/evolution-foundation/evolution-go/pkg/message/service"
	auth_middleware "github.com/evolution-foundation/evolution-go/pkg/middleware"
	"github.com/evolution-foundation/evolution-go/pkg/migrations"
	newsletter_handler "github.com/evolution-foundation/evolution-go/pkg/newsletter/handler"
	newsletter_service "github.com/evolution-foundation/evolution-go/pkg/newsletter/service"
	"github.com/evolution-foundation/evolution-go/pkg/ownership"
	passkey_handler "github.com/evolution-foundation/evolution-go/pkg/passkey/handler"
	poll_handler "github.com/evolution-foundation/evolution-go/pkg/poll/handler"
	routes "github.com/evolution-foundation/evolution-go/pkg/routes"
	send_handler "github.com/evolution-foundation/evolution-go/pkg/sendMessage/handler"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	server_handler "github.com/evolution-foundation/evolution-go/pkg/server/handler"
	storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
	minio_storage "github.com/evolution-foundation/evolution-go/pkg/storage/minio"
	tokencrypt "github.com/evolution-foundation/evolution-go/pkg/tokencrypt"
	typebot_handler "github.com/evolution-foundation/evolution-go/pkg/typebot/handler"
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
	typebot_repository "github.com/evolution-foundation/evolution-go/pkg/typebot/repository"
	typebot_service "github.com/evolution-foundation/evolution-go/pkg/typebot/service"
	user_handler "github.com/evolution-foundation/evolution-go/pkg/user/handler"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	amqp "github.com/rabbitmq/amqp091-go"
)

var devMode = flag.Bool("dev", false, "Enable development mode")

var version = "0.0.0"

func init() {
	// ldflags -X main.version= sets this at compile time.
	// If not set (or still default), try reading from VERSION file.
	if version == "0.0.0" {
		if v, err := os.ReadFile("VERSION"); err == nil {
			if trimmed := strings.TrimSpace(string(v)); trimmed != "" {
				version = trimmed
			}
		}
	}
}

func setupRouter(db *gorm.DB, authDB *sql.DB, sqliteDB *sql.DB, config *config.Config, conn *amqp.Connection, exPath string, messageRepository message_repository.MessageRepository) *gin.Engine {
	killChannel := safemap.New[chan bool]()
	clientPointer := safemap.New[*whatsmeow.Client]()

	loggerWrapper := logger_wrapper.NewLoggerManager(config)

	var rabbitmqProducer producer_interfaces.Producer
	if conn != nil {
		applog.Logger.LogInfo("RabbitMQ enabled")
		rabbitmqProducer = rabbitmq_producer.NewRabbitMQProducer(
			conn,
			config.AmqpGlobalEnabled,
			config.AmqpGlobalEvents,
			config.AmqpSpecificEvents,
			config.AmqpUrl,
			loggerWrapper,
		)
	} else {
		// Even if initial connection failed, pass the URL so reconnection can work
		rabbitmqProducer = rabbitmq_producer.NewRabbitMQProducer(
			nil,
			config.AmqpGlobalEnabled,
			config.AmqpGlobalEvents,
			config.AmqpSpecificEvents,
			config.AmqpUrl, // Keep the URL for reconnection attempts
			loggerWrapper,
		)
	}

	var natsProducer producer_interfaces.Producer
	if config.NatsUrl != "" {
		applog.Logger.LogInfo("NATS enabled")
		natsProducer = nats_producer.NewNatsProducer(
			config.NatsUrl,
			config.NatsGlobalEnabled,
			config.NatsGlobalEvents,
			loggerWrapper,
		)
	} else {
		natsProducer = nats_producer.NewNatsProducer(
			"",
			false,
			nil,
			loggerWrapper,
		)
	}

	// Permanently failed webhooks go to a dead-letter queue, but only when
	// RabbitMQ is configured (there is nowhere to publish them otherwise).
	var webhookOpts []webhook_producer.Option
	if config.AmqpUrl != "" {
		webhookOpts = append(webhookOpts, webhook_producer.WithDeadLetterQueue(rabbitmqProducer, config.WebhookErrorQueueName))
	}
	webhookProducer := webhook_producer.NewWebhookProducer(config.WebhookUrl, loggerWrapper, webhookOpts...)
	websocketProducer := websocket_producer.NewWebsocketProducer(loggerWrapper)

	// Cria filas globais se o RabbitMQ global estiver habilitado
	if config.AmqpGlobalEnabled && conn != nil {
		applog.Logger.LogInfo("Creating global RabbitMQ queues...")
		if err := rabbitmqProducer.CreateGlobalQueues(); err != nil {
			applog.Logger.LogError("Failed to create global RabbitMQ queues: %v", err)
		} else {
			applog.Logger.LogInfo("Global RabbitMQ queues created successfully")
		}
	}

	var mediaStorage storage_interfaces.MediaStorage
	var err error
	if config.MinioEnabled {
		mediaStorage, err = minio_storage.NewMinioMediaStorage(
			config.MinioEndpoint,
			config.MinioAccessKey,
			config.MinioSecretKey,
			config.MinioBucket,
			config.MinioRegion,
			config.MinioUseSSL,
		)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Token codec: encrypts instance API tokens at rest and provides the
	// deterministic hash used for auth lookup. Derived from the same encryption
	// key as the other secrets; nil when none is configured (legacy plaintext).
	var instanceTokenCodec *tokencrypt.Codec
	if len(config.DataEncryptionKey) > 0 {
		if c, err := tokencrypt.New(string(config.DataEncryptionKey)); err == nil {
			instanceTokenCodec = c
		} else {
			applog.Logger.LogWarn("[TOKEN] Could not build token codec: %v", err)
		}
	}

	instanceRepository := instance_repository.NewInstanceRepository(db, instanceTokenCodec)
	labelRepository := label_repository.NewLabelRepository(db)
	typebotRepository := typebot_repository.NewTypebotRepository(db)

	// Single-writer guard for horizontal scaling: one node connects an instance
	// at a time. Postgres advisory locks hold it for the life of the instance.
	// On sqlite (single node) this is inert and every claim succeeds.
	ownershipDriver := ""
	if config.PostgresAuthDB != "" {
		ownershipDriver = "postgres"
	}
	ownershipGuard := ownership.NewGuard(authDB, ownershipDriver)
	// The lease store records who owns each instance (routing/failover). It needs
	// a database; the Postgres auth handle is used for both the lock and the row.
	ownershipStore := ownership.NewStore(authDB, config.NodeID, time.Duration(config.OwnershipLeaseTTLSeconds)*time.Second)

	// Ownership heartbeat: renew this node's leases so they do not expire while
	// it is healthy. If the process dies, renewal stops and another node can
	// adopt the instances (failover). No-op without a database.
	ownership.StartHeartbeat(ownershipStore, time.Duration(config.OwnershipLeaseTTLSeconds)*time.Second/3)

	if ownershipGuard.Supported() {
		applog.Logger.LogInfo("[OWNERSHIP] node=%s single-writer enabled (lease TTL %ds)", config.NodeID, config.OwnershipLeaseTTLSeconds)
	} else {
		applog.Logger.LogInfo("[OWNERSHIP] node=%s single-writer disabled (no Postgres); assume a single node", config.NodeID)
	}

	whatsmeowService := whatsmeow_service.NewWhatsmeowService(
		instanceRepository,
		authDB,
		messageRepository,
		labelRepository,
		config,
		killChannel,
		clientPointer,
		rabbitmqProducer,
		webhookProducer,
		websocketProducer,
		sqliteDB,
		exPath,
		mediaStorage,
		natsProducer,
		loggerWrapper,
		ownershipGuard,
		ownershipStore,
	)
	instanceService := instance_service.NewInstanceService(
		instanceRepository,
		killChannel,
		clientPointer,
		whatsmeowService,
		config,
		loggerWrapper,
	)

	// One-time upgrade: encrypt any instance token still stored in plaintext.
	instanceService.EncryptExistingTokens()

	// Observability registry. Built early so the send service can report into it;
	// the server handler serves it at /metrics and /server/health. The whatsmeow
	// service is the metrics provider (connections + pools) and the webhook
	// producer reports its in-flight depth when it supports it.
	var webhookDepth server_handler.MetricsSource
	if dr, ok := webhookProducer.(producer_interfaces.DepthReporter); ok {
		webhookDepth = dr
	}
	observability := server_handler.NewObservability(whatsmeowService, webhookDepth)

	sendMessageService := send_service.NewSendService(clientPointer, whatsmeowService, config, loggerWrapper, messageRepository, observability)
	userService := user_service.NewUserService(clientPointer, whatsmeowService, loggerWrapper)
	messageService := message_service.NewMessageService(clientPointer, messageRepository, whatsmeowService, loggerWrapper)
	chatService := chat_service.NewChatService(clientPointer, whatsmeowService, loggerWrapper)
	groupService := group_service.NewGroupService(clientPointer, whatsmeowService, loggerWrapper)
	callService := call_service.NewCallService(clientPointer, whatsmeowService, loggerWrapper)
	communityService := community_service.NewCommunityService(clientPointer, whatsmeowService, loggerWrapper)
	labelService := label_service.NewLabelService(clientPointer, whatsmeowService, labelRepository, loggerWrapper)
	newsletterService := newsletter_service.NewNewsletterService(clientPointer, whatsmeowService, loggerWrapper)

	// Typebot replies through the instance itself, so it consumes
	// sendMessageService and is registered on whatsmeowService to be called when
	// a message arrives.
	typebotService := typebot_service.NewTypebotService(
		typebotRepository,
		instanceRepository,
		sendMessageService,
		whatsmeowService, // emitter of auto-pause alerts
		config,
		loggerWrapper,
	)
	whatsmeowService.SetTypebotService(typebotService)

	// NOVO: PollHandler usando PollService já inicializado no whatsmeowService (evita dupla inicialização)
	pollHandler := poll_handler.NewPollHandler(whatsmeowService.GetPollService(), loggerWrapper)

	r := gin.Default()

	// Profiling endpoints (CPU/heap/goroutine) for benchmarking. Off by default;
	// enable with PPROF_ENABLED=true. Uses the standard library handlers, so it
	// adds no dependency. Gated behind the admin key so enabling it for a
	// benchmark does not expose internals to anyone who can reach the port.
	if config.PprofEnabled {
		for path, handler := range pprofHandlers() {
			r.GET(path, auth_middleware.RequireAdminKey(config.GlobalApiKey), gin.WrapH(handler))
		}
		applog.Logger.LogWarn("[PPROF] /debug/pprof is enabled (admin key required) — do not leave this on in production")
	}

	// Abuse protection first, so it covers every route including the public
	// passkey/license ones registered below.
	//
	// CORS: an allowlist (CORS_ALLOWED_ORIGINS). The previous handler echoed `*`
	// together with credentials, which is spec-invalid and unsafe; empty config
	// now means same-origin only.
	r.Use(httpguard.CORSMiddleware(config.CorsAllowedOrigins))

	// Rate limiting: per credential (instance token / admin key) or per client
	// IP when unauthenticated. Stops apikey brute force and POST /send/* floods.
	// Health/scrape routes are exempt and the admin key gets its own, more
	// generous bucket, so a load test or dashboard burst never locks the operator
	// out (both refinements came from a real load test).
	r.Use(httpguard.MiddlewareWithAdmin(
		httpguard.NewLimiter(config.RateLimitPerMinute, time.Minute),
		nil,
		config.GlobalApiKey,
	))

	// Passkey ceremony routes — PUBLIC (called by the browser extension from the
	// web.whatsapp.com origin, gated only by an opaque ephemeral token).
	passkey_handler.RegisterRoutes(r, whatsmeowService)

	// Local-only license compatibility endpoints.
	//
	// The fork removed the vendor license server, its gate middleware and the
	// heartbeat entirely (see FORK_NOTES.md). The prebuilt Manager UI, however,
	// probes /license/status and hides instance management unless it reads
	// "active". These handlers answer locally so the UI is usable offline —
	// nothing here contacts Evolution Foundation or any other server.
	licenseStatus := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "active",
			"instance_id": "local",
			"message":     "License activation removed in this build",
		})
	}
	r.GET("/license/status", licenseStatus)
	r.GET("/license/register", licenseStatus)
	r.GET("/license/activate", licenseStatus)

	// The dashboard's storage panel reports the size of the app's data directory
	// (the parent of LOG_DIRECTORY, i.e. the mounted volume) and of the disk that
	// holds it. Fall back to the executable's directory when the logs path is
	// relative, so the numbers stay meaningful outside Docker.
	dataDir := exPath
	if config.LogDirectory != "" {
		if parent := filepath.Dir(config.LogDirectory); parent != "" && parent != "." {
			dataDir = parent
		}
	}
	mediaBackend := ""
	if config.MinioEnabled {
		mediaBackend = "minio:" + config.MinioBucket
	}

	// Local attachment store (manager previews) under the data volume. Turned
	// off with MEDIA_LOCAL_STORE=false, which restores the previous behaviour.
	if config.MediaLocalStore {
		if err := localmedia.Configure(filepath.Join(dataDir, "media")); err != nil {
			log.Fatal(err)
		}
	}

	// A boot/config problem worth reporting on GET /. Empty means all good; a
	// missing API key or database is fatal earlier in config.Load, so this only
	// covers non-fatal misconfiguration the service still starts with.
	configError := ""

	routes.NewRouter(
		config,
		auth_middleware.NewMiddleware(config, instanceService),
		instance_handler.NewInstanceHandler(instanceService, config),
		user_handler.NewUserHandler(userService),
		send_handler.NewSendHandler(sendMessageService),
		message_handler.NewMessageHandler(messageService),
		chat_handler.NewChatHandler(chatService),
		group_handler.NewGroupHandler(groupService),
		call_handler.NewCallHandler(callService),
		community_handler.NewCommunityHandler(communityService),
		label_handler.NewLabelHandler(labelService),
		newsletter_handler.NewNewsletterHandler(newsletterService),
		pollHandler,
		server_handler.NewServerHandler(messageRepository, version, whatsmeowService, dataDir, mediaBackend, config.DatabaseSaveMessages, config.MediaLocalStore, config.WebhookFiles, config.ClientName, configError, observability),
		typebot_handler.NewTypebotHandler(typebotRepository, loggerWrapper),
	).AssignRoutes(r)

	if config.ConnectOnStartup {
		go whatsmeowService.ConnectOnStartup(config.ClientName)
	}

	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		instanceId := c.Query("instanceId")

		// Constant-time compare so the global key cannot be recovered byte by
		// byte through response-timing differences (matches AuthAdmin).
		if subtle.ConstantTimeCompare([]byte(token), []byte(config.GlobalApiKey)) != 1 {
			applog.Logger.LogError("WebSocket connection rejected: invalid token")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token inválido"})
			return
		}

		websocket_producer.ServeWs(c.Writer, c.Request, instanceId, websocketProducer)
	})

	return r
}

// migrate brings the schema up to date in two phases:
//
//  1. AutoMigrate bootstraps the desired shape. It is additive-only, which is
//     exactly what a brand-new database needs, and it keeps the initial schema
//     defined in one place (the models).
//  2. The versioned migrations then apply the changes AutoMigrate cannot express
//     (composite indexes, corrections) and record what ran in schema_migrations.
//
// For a fresh database phase 1 creates the tables and phase 2 adds the indexes;
// for an existing one both are no-ops after the first run.
// pprofHandlers maps the standard library profiling endpoints to their paths.
// The default net/http/pprof registrations live on http.DefaultServeMux; this
// exposes the same handlers explicitly so they can be mounted on the Gin engine
// only when PPROF_ENABLED=true.
func pprofHandlers() map[string]http.Handler {
	handlers := map[string]http.Handler{
		"/debug/pprof/":        http.DefaultServeMux,
		"/debug/pprof/cmdline": http.HandlerFunc(pprof.Cmdline),
		"/debug/pprof/profile": http.HandlerFunc(pprof.Profile),
		"/debug/pprof/symbol":  http.HandlerFunc(pprof.Symbol),
		"/debug/pprof/trace":   http.HandlerFunc(pprof.Trace),
	}
	// Named profiles served by pprof.Index (e.g. /debug/pprof/heap). Gin does
	// not fall through to DefaultServeMux, so each needs its own route.
	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		handlers["/debug/pprof/"+name] = http.DefaultServeMux
	}
	return handlers
}

func migrate(db *gorm.DB) {
	err := db.AutoMigrate(
		&instance_model.Instance{},
		&message_model.Message{},
		&label_model.Label{},
		&typebot_model.Typebot{},
		&typebot_model.TypebotSession{},
	)

	if err != nil {
		log.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("[MIGRATIONS] could not reach the raw database handle: %v", err)
	}

	// The GORM driver here is always postgres (see config.CreateUsersDB), but
	// the runner supports sqlite too for the store-less setups.
	if _, err := migrations.Apply(context.Background(), sqlDB, "postgres"); err != nil {
		log.Fatalf("[MIGRATIONS] %v", err)
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

func initPostgresAuthDB(config *config.Config) (*sql.DB, error) {
	if config.PostgresAuthDB == "" {
		return nil, nil
	}

	if err := config.EnsureDBExists(config.PostgresAuthDB); err != nil {
		applog.Logger.LogWarn("Auto-setup auth DB failed (will try connecting anyway): %v", err)
	}

	db, err := sql.Open("postgres", config.PostgresAuthDB)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao banco AUTH PostgreSQL: %v", err)
	}

	// One source of truth for pool sizing: DB_MAX_OPEN_CONNS / DB_MAX_IDLE_CONNS,
	// shared with the GORM pools and the whatsmeow key store.
	poolOpen, poolIdle := config.DatabaseMaxOpenConns, config.DatabaseMaxIdleConns
	if poolOpen <= 0 {
		poolOpen = 25
	}
	if poolIdle <= 0 {
		poolIdle = 5
	}
	db.SetMaxOpenConns(poolOpen)
	db.SetMaxIdleConns(poolIdle)
	db.SetConnMaxLifetime(5 * time.Minute) // Reconectar após 5 minutos para evitar timeouts
	db.SetConnMaxIdleTime(1 * time.Minute) // Fechar conexões ociosas após 1 minuto

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("erro ao pingar banco AUTH PostgreSQL: %v", err)
	}

	applog.Logger.LogInfo("Conectado ao banco AUTH PostgreSQL com pool configurado")
	return db, nil
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

	cfg := config.Load()

	applog.Logger.LogInfo("Starting Evolution GO version %s", version)

	db, err := cfg.CreateUsersDB()
	if err != nil {
		log.Fatal(err)
	}

	// Inicializar PostgreSQL AUTH
	authDB, err := initPostgresAuthDB(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if authDB != nil {
		defer authDB.Close()

		// The ownership/lease table lives in the AUTH database next to the
		// whatsmeow key store: it is about which node holds a connection, not
		// about message history. Run only the ownership migration here.
		if _, err := migrations.ApplyAuth(context.Background(), authDB, "postgres"); err != nil {
			log.Fatalf("[MIGRATIONS] auth database: %v", err)
		}
	}

	// Manter inicialização do SQLite
	sqliteDB, exPath, err := initAuthDB(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if sqliteDB != nil {
		defer sqliteDB.Close()
	}

	migrate(db)

	var conn *amqp.Connection

	if cfg.AmqpUrl != "" {
		applog.Logger.LogInfo("Attempting to connect to RabbitMQ...")

		// Create connection with heartbeat to prevent timeouts
		amqpConfig := amqp.Config{
			Heartbeat: 30 * time.Second, // Send heartbeat every 30 seconds
			Locale:    "en_US",
		}

		conn, err = amqp.DialConfig(cfg.AmqpUrl, amqpConfig)
		if err != nil {
			applog.Logger.LogError("Failed to connect to RabbitMQ, err: %v", err)
			applog.Logger.LogInfo("RabbitMQ producer will be created with reconnection capability")
		} else {
			applog.Logger.LogInfo("Successfully connected to RabbitMQ with heartbeat enabled")
			defer func(conn *amqp.Connection) {
				err := conn.Close()
				if err != nil {
					applog.Logger.LogError("Failed to close RabbitMQ connection, err: %v", err)
				}
			}(conn)
		}
	} else {
		applog.Logger.LogInfo("RabbitMQ URL not configured, skipping RabbitMQ connection")
	}

	// The repository is shared by the HTTP handlers and the retention job.
	//
	// The dashboard aggregates come from the message_counters rollup when it
	// installed successfully, and from a (cached) scan of `messages` otherwise.
	rollupReady := true
	if err := message_repository.EnsureMessageCounters(db); err != nil {
		applog.Logger.LogError("[stats] message counter rollup unavailable, the dashboard will aggregate live: %v", err)
		rollupReady = false
	}
	messageRepository := message_repository.NewMessageRepository(db,
		message_repository.WithRollup(rollupReady),
		message_repository.WithAggregateCacheTTL(cfg.DashboardCacheTTL))

	// Background workers. Cancelled on shutdown so they do not outlive the
	// server; the message cleanup prunes rows past MESSAGE_RETENTION_DAYS.
	workersCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	message_cleanup.NewCleaner(messageRepository, cfg.MessageRetentionDays).Start(workersCtx)

	r := setupRouter(db, authDB, sqliteDB, cfg, conn, exPath, messageRepository)

	srv := &http.Server{
		Addr:    ":" + os.Getenv("SERVER_PORT"),
		Handler: r,
		// Bounds so a slow/idle client cannot pin a connection (Slowloris) or
		// hold workers open indefinitely. WriteTimeout is generous because some
		// endpoints (media download/upload, group info) can legitimately take a
		// while; ReadHeaderTimeout is the important anti-Slowloris one.
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		applog.Logger.LogInfo("Iniciando servidor na porta %s", os.Getenv("SERVER_PORT"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	applog.Logger.LogInfo("[SHUTDOWN] Signal received, shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		applog.Logger.LogError("[SHUTDOWN] Server forced to shutdown: %v", err)
	}

	applog.Logger.LogInfo("[SHUTDOWN] Server exited")
}
