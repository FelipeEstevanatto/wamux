package config

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	applog "github.com/evolution-foundation/evolution-go/pkg/applog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	config_env "github.com/evolution-foundation/evolution-go/pkg/config/env"
	"github.com/evolution-foundation/evolution-go/pkg/webhooksign"
)

// webhookHmacDerivationLabel domain-separates the webhook-key encryption key
// derived from GLOBAL_API_KEY from any other use of that secret.
const webhookHmacDerivationLabel = "evolution-go:webhook-hmac-encryption:v1:"

type Config struct {
	PostgresAuthDB       string
	postgresUsersDB      string
	PostgresHost         string
	PostgresPort         string
	PostgresUser         string
	PostgresPassword     string
	PostgresDB           string
	DatabaseSaveMessages bool
	GlobalApiKey         string
	WaDebug              string
	LogType              string
	WebhookFiles         bool
	// MediaLocalStore keeps attachment copies under <dataDir>/media for manager
	// previews. Defaults to true; set MEDIA_LOCAL_STORE=false to disable.
	MediaLocalStore      bool
	ConnectOnStartup     bool
	RerequestFromPhone   bool
	OsName               string
	AmqpUrl              string
	AmqpGlobalEnabled    bool
	WebhookUrl           string
	ClientName           string
	ApiAudioConverter    string
	ApiAudioConverterKey string
	MinioEndpoint        string
	MinioAccessKey       string
	MinioSecretKey       string
	MinioBucket          string
	MinioUseSSL          bool
	MinioEnabled         bool
	MinioRegion          string
	WhatsappVersionMajor int
	WhatsappVersionMinor int
	WhatsappVersionPatch int
	ProxyProtocol        string
	ProxyHost            string
	ProxyPort            string
	ProxyUsername        string
	ProxyPassword        string
	AmqpGlobalEvents     []string
	AmqpSpecificEvents   []string
	NatsUrl              string
	NatsGlobalEnabled    bool
	NatsGlobalEvents     []string
	EventIgnoreGroup     bool
	EventIgnoreStatus    bool
	QrcodeMaxCount       int
	CheckUserExists      bool

	// WebhookHmacGlobalKey signs webhook deliveries for instances that have no
	// per-instance key. Empty disables the global fallback.
	WebhookHmacGlobalKey string
	// WebhookHmacEncryptionKey is the 32-byte AES-256-GCM key used to encrypt
	// per-instance HMAC keys at rest. It is always populated: a dedicated
	// secret is used when configured, otherwise one is derived from
	// GLOBAL_API_KEY so the feature works out of the box.
	WebhookHmacEncryptionKey []byte
	// DataEncryptionKey is the 32-byte AES-256-GCM key used to encrypt other
	// per-instance secrets at rest (currently the S3 secret key). Derived from
	// the same configured secret as WebhookHmacEncryptionKey.
	DataEncryptionKey []byte
	// WebhookErrorQueueName is the RabbitMQ queue for webhooks that failed
	// permanently. Empty disables the dead-letter path.
	WebhookErrorQueueName string

	// Typebot flood/loop protections. See pkg/typebot/service/protection.go.
	TypebotContactRateLimit  int
	TypebotContactRateWindow int
	TypebotSendRateLimit     int
	TypebotSendRateBurst     int

	// SwaggerEnabled controls whether the public /swagger documentation routes
	// are registered. Defaults to true.
	SwaggerEnabled bool

	// Logger configurations
	LogMaxSize    int
	LogMaxBackups int
	LogMaxAge     int
	LogDirectory  string
	LogCompress   bool

	// DashboardCacheTTL bounds how long the dashboard's message aggregations
	// (/server/stats and /instance/overview counts) may be served from memory.
	// Those queries are whole-table scans, so caching keeps the DB cost
	// independent of how many dashboards are open. Zero disables the cache.
	DashboardCacheTTL time.Duration

	// MessageRetentionDays is how long persisted messages are kept. A background
	// job deletes anything older, in batches; zero keeps them forever.
	MessageRetentionDays int

	// DatabaseMaxOpenConns / DatabaseMaxIdleConns size the Postgres connection
	// pools (users DB, auth DB and the whatsmeow store). The default of 25/5 is
	// shared across every instance and the dashboard; a busy server with many
	// instances should raise MaxOpenConns (and may need Postgres
	// max_connections / a pooler to match).
	DatabaseMaxOpenConns int
	DatabaseMaxIdleConns int

	// HTTP-layer abuse protection. RateLimitPerMinute bounds requests per
	// credential (instance token / admin key) or per IP when unauthenticated;
	// 0 disables it. CorsAllowedOrigins is an allowlist ("*" opts into
	// reflecting any origin); empty means same-origin only — the old `*` +
	// credentials combination was both unsafe and spec-invalid.
	RateLimitPerMinute int
	CorsAllowedOrigins []string
}

// EnsureDBExists connects to postgres (without the target database) and creates it if it doesn't exist.
func (c *Config) EnsureDBExists(dsn string) error {
	return ensureDBExists(dsn)
}

// ensureDBExists connects to postgres (without the target database) and creates it if it doesn't exist.
func ensureDBExists(dsn string) error {
	dbName, adminDSN, err := extractDBNameAndAdminDSN(dsn)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres for auto-setup: %v", err)
	}
	defer db.Close()

	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check database existence: %v", err)
	}

	if !exists {
		applog.Logger.LogInfo("[CONFIG] Database %q not found, creating it automatically...", dbName)
		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %q", dbName))
		if err != nil {
			return fmt.Errorf("failed to create database %q: %v", dbName, err)
		}
		applog.Logger.LogInfo("[CONFIG] Database %q created successfully", dbName)
	}

	return nil
}

// extractDBNameAndAdminDSN parses a DSN (URL or key=value) and returns the database name
// and a DSN pointing to the "postgres" maintenance database.
func extractDBNameAndAdminDSN(dsn string) (string, string, error) {
	// Try URL format: postgres://user:pass@host:port/dbname?...
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse DSN URL: %v", err)
		}
		dbName := strings.TrimPrefix(u.Path, "/")
		u.Path = "/postgres"
		return dbName, u.String(), nil
	}

	// Key=value format: host=... user=... password=... dbname=... sslmode=...
	parts := strings.Fields(dsn)
	kvMap := make(map[string]string, len(parts))
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			kvMap[kv[0]] = kv[1]
		}
	}
	dbName, ok := kvMap["dbname"]
	if !ok || dbName == "" {
		return "", "", fmt.Errorf("could not extract dbname from DSN")
	}
	kvMap["dbname"] = "postgres"
	adminParts := make([]string, 0, len(kvMap))
	for k, v := range kvMap {
		adminParts = append(adminParts, k+"="+v)
	}
	return dbName, strings.Join(adminParts, " "), nil
}

func (c *Config) CreateUsersDB() (*gorm.DB, error) {
	applog.Logger.LogDebug("Connecting to database on: %s", c.postgresUsersDB)

	dbDSN := c.postgresUsersDB

	if c.postgresUsersDB == "" {
		dbDSN = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB)
	}

	if err := ensureDBExists(dbDSN); err != nil {
		applog.Logger.LogWarn("[CONFIG] Auto-setup failed (will try connecting anyway): %v", err)
	}

	db, err := gorm.Open(
		postgres.Open(dbDSN),
		&gorm.Config{},
	)
	if err != nil {
		return nil, err
	}

	// Configurar pool de conexões no GORM
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter sql.DB do GORM: %v", err)
	}

	// Configurar pool de conexões para evitar conexões ociosas não fechadas
	sqlDB.SetMaxOpenConns(c.DatabaseMaxOpenConns)
	sqlDB.SetMaxIdleConns(c.DatabaseMaxIdleConns)
	sqlDB.SetConnMaxLifetime(5 * time.Minute) // Reconectar após 5 minutos para evitar timeouts
	sqlDB.SetConnMaxIdleTime(1 * time.Minute) // Fechar conexões ociosas após 1 minuto

	return db, nil
}

func (c *Config) CreateAuthDB() (*sql.DB, error) {
	dbDSN := c.postgresUsersDB

	if c.postgresUsersDB == "" {
		dbDSN = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB)
	}

	if err := ensureDBExists(dbDSN); err != nil {
		applog.Logger.LogWarn("[CONFIG] Auto-setup failed (will try connecting anyway): %v", err)
	}

	db, err := sql.Open("postgres", dbDSN)
	if err != nil {
		return nil, err
	}

	// Configurar pool de conexões para evitar conexões ociosas não fechadas
	db.SetMaxOpenConns(c.DatabaseMaxOpenConns)
	db.SetMaxIdleConns(c.DatabaseMaxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute) // Reconectar após 5 minutos para evitar timeouts
	db.SetConnMaxIdleTime(1 * time.Minute) // Fechar conexões ociosas após 1 minuto

	// Testar a conexão
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("erro ao testar conexão PostgreSQL AUTH: %v", err)
	}

	return db, nil
}

func Load() *Config {
	postgresAuthDB := os.Getenv(config_env.POSTGRES_AUTH_DB)

	postgresUsersDB := os.Getenv(config_env.POSTGRES_USERS_DB)

	postgresHost := os.Getenv(config_env.POSTGRES_HOST)
	postgresPort := os.Getenv(config_env.POSTGRES_PORT)
	postgresUser := os.Getenv(config_env.POSTGRES_USER)
	postgresPassword := os.Getenv(config_env.POSTGRES_PASSWORD)
	postgresDB := os.Getenv(config_env.POSTGRES_DB)

	if postgresUsersDB == "" && (postgresHost == "" || postgresPort == "" || postgresUser == "" || postgresPassword == "" || postgresDB == "") {
		applog.Logger.LogFatal("[CONFIG] required database configuration variables are missing. Please check your environment configuration.")
	}

	databaseSaveMessages := os.Getenv(config_env.DATABASE_SAVE_MESSAGES)
	panicIfEmpty(config_env.DATABASE_SAVE_MESSAGES, databaseSaveMessages)

	globalApiKey := os.Getenv(config_env.GLOBAL_API_KEY)
	panicIfEmpty(config_env.GLOBAL_API_KEY, globalApiKey)

	clientName := os.Getenv(config_env.CLIENT_NAME)

	waDebug := os.Getenv(config_env.WA_DEBUG)

	logType := os.Getenv(config_env.LOGTYPE)

	webhookFiles := os.Getenv(config_env.WEBHOOKFILES)
	if webhookFiles == "" {
		webhookFiles = "true"
	}

	// Keep local attachment copies for the manager's previews. On by default;
	// set MEDIA_LOCAL_STORE=false to restore the no-local-copy behaviour.
	mediaLocalStore := os.Getenv(config_env.MEDIA_LOCAL_STORE)
	if mediaLocalStore == "" {
		mediaLocalStore = "true"
	}

	connectOnStartup := os.Getenv(config_env.CONNECT_ON_STARTUP)
	if connectOnStartup == "" {
		connectOnStartup = "false"
	}

	osName := os.Getenv(config_env.OS_NAME)

	amqpUrl := os.Getenv(config_env.AMQP_URL)

	// Validate AMQP URL format
	if err := validateAMQPURL(amqpUrl); err != nil {
		applog.Logger.LogFatal("[CONFIG] AMQP URL validation failed: %v", err)
	}

	amqpGlobalEnabled := os.Getenv(config_env.AMQP_GLOBAL_ENABLED)

	webhookUrl := os.Getenv(config_env.WEBHOOK_URL)

	apiAudioConverter := os.Getenv(config_env.API_AUDIO_CONVERTER)
	apiAudioConverterKey := os.Getenv(config_env.API_AUDIO_CONVERTER_KEY)

	whatsappVersionMajor := os.Getenv(config_env.WHATSAPP_VERSION_MAJOR)
	whatsappVersionMinor := os.Getenv(config_env.WHATSAPP_VERSION_MINOR)
	whatsappVersionPatch := os.Getenv(config_env.WHATSAPP_VERSION_PATCH)

	proxyProtocol := os.Getenv(config_env.PROXY_PROTOCOL)
	proxyHost := os.Getenv(config_env.PROXY_HOST)
	proxyPort := os.Getenv(config_env.PROXY_PORT)
	proxyUsername := os.Getenv(config_env.PROXY_USERNAME)
	proxyPassword := os.Getenv(config_env.PROXY_PASSWORD)

	eventIgnoreGroup := os.Getenv(config_env.EVENT_IGNORE_GROUP)
	eventIgnoreStatus := os.Getenv(config_env.EVENT_IGNORE_STATUS)
	qrcodeMaxCount := os.Getenv(config_env.QRCODE_MAX_COUNT)
	checkUserExists := os.Getenv(config_env.CHECK_USER_EXISTS)

	if checkUserExists == "" {
		checkUserExists = "true"
	}

	// Webhook HMAC signing. A global key is optional (it signs deliveries for
	// instances without their own key); the encryption key always exists so
	// per-instance keys can be stored without extra configuration.
	webhookHmacGlobalKey := strings.TrimSpace(os.Getenv(config_env.WEBHOOK_HMAC_KEY))
	if webhookHmacGlobalKey != "" && len(webhookHmacGlobalKey) < webhooksign.MinKeyLength {
		applog.Logger.LogWarn("[CONFIG] %s is shorter than %d characters; use a longer, random key", config_env.WEBHOOK_HMAC_KEY, webhooksign.MinKeyLength)
	}

	hmacEncSecret := os.Getenv(config_env.WEBHOOK_HMAC_ENCRYPTION_KEY)
	hmacEncSource := config_env.WEBHOOK_HMAC_ENCRYPTION_KEY
	if hmacEncSecret == "" {
		hmacEncSecret = os.Getenv(config_env.GLOBAL_ENCRYPTION_KEY)
		hmacEncSource = config_env.GLOBAL_ENCRYPTION_KEY
	}
	if hmacEncSecret == "" {
		// Derive a stable key from the always-present global API key so
		// per-instance keys survive restarts without any extra configuration.
		// Setting WEBHOOK_HMAC_ENCRYPTION_KEY decouples the two secrets.
		hmacEncSecret = webhookHmacDerivationLabel + globalApiKey
		hmacEncSource = config_env.GLOBAL_API_KEY + " (derived; set " + config_env.WEBHOOK_HMAC_ENCRYPTION_KEY + " to decouple)"
	}
	webhookHmacEncryptionKey, err := webhooksign.DeriveEncryptionKey(hmacEncSecret)
	if err != nil {
		applog.Logger.LogFatal("[CONFIG] failed to derive the webhook HMAC encryption key: %v", err)
	}
	applog.Logger.LogInfo("[CONFIG] webhook HMAC key encryption source: %s", hmacEncSource)

	// Dead-letter queue for permanently failed webhooks (RabbitMQ only).
	webhookErrorQueueName := strings.TrimSpace(os.Getenv(config_env.WEBHOOK_ERROR_QUEUE_NAME))
	if webhookErrorQueueName == "" {
		webhookErrorQueueName = "webhook_errors"
	}

	// Swagger is served by default; set SWAGGER_ENABLED=false to disable it. The
	// docs are public (no apikey), so this is the switch for locking down /swagger
	// on an internet-facing deployment.
	swaggerEnabled := os.Getenv(config_env.SWAGGER_ENABLED) != "false"

	rerequestFromPhone := os.Getenv(config_env.REREQUEST_FROM_PHONE)

	// Convertendo para int com valores padrão caso estejam vazios
	major := 0
	if whatsappVersionMajor != "" {
		major, _ = strconv.Atoi(whatsappVersionMajor)
	}
	minor := 0
	if whatsappVersionMinor != "" {
		minor, _ = strconv.Atoi(whatsappVersionMinor)
	}
	patch := 0
	if whatsappVersionPatch != "" {
		patch, _ = strconv.Atoi(whatsappVersionPatch)
	}

	qrMaxCount := 5 // Valor padrão
	if qrcodeMaxCount != "" {
		// A malformed value must not silently disable the QR limit (0 means
		// "unlimited" downstream), so keep the default on parse failure.
		parsed, err := strconv.Atoi(qrcodeMaxCount)
		if err != nil {
			fmt.Printf("invalid QRCODE_MAX_COUNT %q, using default %d: %v\n", qrcodeMaxCount, qrMaxCount, err)
		} else if parsed < 0 {
			fmt.Printf("invalid QRCODE_MAX_COUNT %d (negative), using default %d\n", parsed, qrMaxCount)
		} else {
			qrMaxCount = parsed
		}
	}

	amqpGlobalEvents := strings.Split(os.Getenv(config_env.AMQP_GLOBAL_EVENTS), ",")
	if len(amqpGlobalEvents) == 1 && amqpGlobalEvents[0] == "" {
		amqpGlobalEvents = []string{}
	}

	amqpSpecificEvents := strings.Split(os.Getenv(config_env.AMQP_SPECIFIC_EVENTS), ",")
	if len(amqpSpecificEvents) == 1 && amqpSpecificEvents[0] == "" {
		amqpSpecificEvents = []string{}
	}

	natsUrl := os.Getenv(config_env.NATS_URL)
	natsGlobalEnabled := os.Getenv(config_env.NATS_GLOBAL_ENABLED)
	natsGlobalEvents := strings.Split(os.Getenv(config_env.NATS_GLOBAL_EVENTS), ",")
	if len(natsGlobalEvents) == 1 && natsGlobalEvents[0] == "" {
		natsGlobalEvents = []string{}
	}

	// Logger configurations
	logMaxSize, _ := strconv.Atoi(os.Getenv(config_env.LOG_MAX_SIZE))
	if logMaxSize == 0 {
		logMaxSize = 100 // Default 100MB
	}

	logMaxBackups, _ := strconv.Atoi(os.Getenv(config_env.LOG_MAX_BACKUPS))
	if logMaxBackups == 0 {
		logMaxBackups = 5 // Default 5 backups
	}

	logMaxAge, _ := strconv.Atoi(os.Getenv(config_env.LOG_MAX_AGE))
	if logMaxAge == 0 {
		logMaxAge = 30 // Default 30 days
	}

	logDirectory := os.Getenv(config_env.LOG_DIRECTORY)
	if logDirectory == "" {
		logDirectory = "./logs" // Default logs directory
	}

	logCompress := os.Getenv(config_env.LOG_COMPRESS) == "true"
	if os.Getenv(config_env.LOG_COMPRESS) == "" {
		logCompress = true // Default compression enabled
	}

	// The dashboard aggregations are whole-table scans, so they are cached for a
	// short while by default. Set DASHBOARD_CACHE_TTL_SECONDS=0 to disable.
	dashboardCacheTTL := 30 * time.Second
	if raw := os.Getenv(config_env.DASHBOARD_CACHE_TTL_SECONDS); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 0 {
			applog.Logger.LogWarn("[CONFIG] invalid %s=%q, using the default of 30s", config_env.DASHBOARD_CACHE_TTL_SECONDS, raw)
		} else {
			dashboardCacheTTL = time.Duration(seconds) * time.Second
		}
	}

	// Persisted messages are pruned after this many days; 0 keeps them forever.
	messageRetentionDays := 365
	if raw := os.Getenv(config_env.MESSAGE_RETENTION_DAYS); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 0 {
			applog.Logger.LogWarn("[CONFIG] invalid %s=%q, using the default of 365 days", config_env.MESSAGE_RETENTION_DAYS, raw)
		} else {
			messageRetentionDays = days
		}
	}

	// Postgres pool sizing. 25/5 is a conservative default shared by every
	// instance and the dashboard; raise MaxOpenConns on a busier server.
	dbMaxOpen, dbMaxIdle := parseDBPoolConfig(
		os.Getenv(config_env.DB_MAX_OPEN_CONNS),
		os.Getenv(config_env.DB_MAX_IDLE_CONNS),
	)

	// HTTP abuse protection. Default 600/min per credential is generous enough
	// for a busy integration yet still bounds brute force and floods; a
	// self-hosted install can raise or zero it. CORS defaults to same-origin.
	rateLimitPerMinute, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(config_env.RATE_LIMIT_PER_MINUTE)))
	if rateLimitPerMinute < 0 {
		rateLimitPerMinute = 0
	}
	if _, set := os.LookupEnv(config_env.RATE_LIMIT_PER_MINUTE); !set {
		rateLimitPerMinute = 600
	}
	corsAllowedOrigins := parseCSV(os.Getenv(config_env.CORS_ALLOWED_ORIGINS))

	// Typebot protections. The per-contact limit is on by default (it only
	// affects senders bursting many messages); the per-instance send ceiling is
	// off by default (a badly tuned value would delay legitimate replies).
	typebotContactRateLimit := envInt(config_env.TYPEBOT_CONTACT_RATE_LIMIT, 10)
	typebotContactRateWindow := envInt(config_env.TYPEBOT_CONTACT_RATE_WINDOW, 60)
	typebotSendRateLimit := envInt(config_env.TYPEBOT_SEND_RATE_LIMIT, 0)
	typebotSendRateBurst := envInt(config_env.TYPEBOT_SEND_RATE_BURST, 20)

	config := &Config{
		PostgresAuthDB:           postgresAuthDB,
		postgresUsersDB:          postgresUsersDB,
		DatabaseSaveMessages:     databaseSaveMessages == "true",
		GlobalApiKey:             globalApiKey,
		WaDebug:                  waDebug,
		LogType:                  logType,
		WebhookFiles:             webhookFiles == "true",
		MediaLocalStore:          mediaLocalStore == "true",
		ConnectOnStartup:         connectOnStartup == "true",
		OsName:                   osName,
		AmqpUrl:                  amqpUrl,
		AmqpGlobalEnabled:        amqpGlobalEnabled == "true",
		WebhookUrl:               webhookUrl,
		ClientName:               clientName,
		ApiAudioConverter:        apiAudioConverter,
		ApiAudioConverterKey:     apiAudioConverterKey,
		PostgresHost:             postgresHost,
		PostgresPort:             postgresPort,
		PostgresUser:             postgresUser,
		PostgresPassword:         postgresPassword,
		PostgresDB:               postgresDB,
		WhatsappVersionMajor:     major,
		WhatsappVersionMinor:     minor,
		WhatsappVersionPatch:     patch,
		ProxyProtocol:            proxyProtocol,
		ProxyHost:                proxyHost,
		ProxyPort:                proxyPort,
		ProxyUsername:            proxyUsername,
		ProxyPassword:            proxyPassword,
		EventIgnoreGroup:         eventIgnoreGroup == "true",
		EventIgnoreStatus:        eventIgnoreStatus == "true",
		QrcodeMaxCount:           qrMaxCount,
		CheckUserExists:          checkUserExists != "false", // Default true, set to false to disable
		SwaggerEnabled:           swaggerEnabled,
		TypebotContactRateLimit:  typebotContactRateLimit,
		TypebotContactRateWindow: typebotContactRateWindow,
		TypebotSendRateLimit:     typebotSendRateLimit,
		TypebotSendRateBurst:     typebotSendRateBurst,
		RerequestFromPhone:       rerequestFromPhone == "true",
		AmqpGlobalEvents:         amqpGlobalEvents,
		AmqpSpecificEvents:       amqpSpecificEvents,
		NatsUrl:                  natsUrl,
		NatsGlobalEnabled:        natsGlobalEnabled == "true",
		NatsGlobalEvents:         natsGlobalEvents,
		WebhookHmacGlobalKey:     webhookHmacGlobalKey,
		WebhookHmacEncryptionKey: webhookHmacEncryptionKey,
		DataEncryptionKey:        webhookHmacEncryptionKey,
		WebhookErrorQueueName:    webhookErrorQueueName,
		LogMaxSize:               logMaxSize,
		LogMaxBackups:            logMaxBackups,
		LogMaxAge:                logMaxAge,
		LogDirectory:             logDirectory,
		LogCompress:              logCompress,
		DashboardCacheTTL:        dashboardCacheTTL,
		MessageRetentionDays:     messageRetentionDays,
		DatabaseMaxOpenConns:     dbMaxOpen,
		DatabaseMaxIdleConns:     dbMaxIdle,
		RateLimitPerMinute:       rateLimitPerMinute,
		CorsAllowedOrigins:       corsAllowedOrigins,
	}

	minioEnabled := os.Getenv(config_env.MINIO_ENABLED) == "true"
	if minioEnabled {
		config.MinioEnabled = true
		loadMinioConfig(config)
	}

	return config
}

func loadMinioConfig(config *Config) {
	minioEndpoint := os.Getenv(config_env.MINIO_ENDPOINT)
	panicIfEmpty(config_env.MINIO_ENDPOINT, minioEndpoint)

	minioAccessKey := os.Getenv(config_env.MINIO_ACCESS_KEY)
	panicIfEmpty(config_env.MINIO_ACCESS_KEY, minioAccessKey)

	minioSecretKey := os.Getenv(config_env.MINIO_SECRET_KEY)
	panicIfEmpty(config_env.MINIO_SECRET_KEY, minioSecretKey)

	minioBucket := os.Getenv(config_env.MINIO_BUCKET)
	panicIfEmpty(config_env.MINIO_BUCKET, minioBucket)

	minioUseSSL := os.Getenv(config_env.MINIO_USE_SSL) == "true"

	minioRegion := os.Getenv(config_env.MINIO_REGION)

	config.MinioEndpoint = minioEndpoint
	config.MinioAccessKey = minioAccessKey
	config.MinioSecretKey = minioSecretKey
	config.MinioBucket = minioBucket
	config.MinioUseSSL = minioUseSSL
	config.MinioRegion = minioRegion
}

// parseDBPoolConfig turns the DB_MAX_OPEN_CONNS/DB_MAX_IDLE_CONNS strings into
// pool sizes. It defaults to 25/5, clamps invalid values back to the defaults,
// and never lets idle exceed open.
func parseDBPoolConfig(openRaw, idleRaw string) (int, int) {
	const defOpen, defIdle = 25, 5

	open := defOpen
	if openRaw != "" {
		n, err := strconv.Atoi(openRaw)
		if err != nil || n < 1 {
			applog.Logger.LogWarn("[CONFIG] invalid DB_MAX_OPEN_CONNS=%q, using the default of %d", openRaw, defOpen)
		} else {
			open = n
		}
	}

	idle := defIdle
	if idleRaw != "" {
		n, err := strconv.Atoi(idleRaw)
		if err != nil || n < 0 {
			applog.Logger.LogWarn("[CONFIG] invalid DB_MAX_IDLE_CONNS=%q, using the default of %d", idleRaw, defIdle)
		} else {
			idle = n
		}
	}

	if idle > open {
		idle = open
	}
	return open, idle
}

// parseCSV splits a comma-separated env value, trimming spaces and dropping
// empties, so an unset or blank variable yields a nil (empty) slice.
func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func panicIfEmpty(key, value string) {
	if value == "" {
		if os.Getenv("DEBUG_ENABLED") != "1" {
			applog.Logger.LogInfo("You are NOT on development mode")
		}
		applog.Logger.LogFatal("[CONFIG] required configuration variable is missing. Please check your environment configuration.")
	}
}

// validateAMQPURL validates if the AMQP URL has the correct scheme and format
func validateAMQPURL(amqpURL string) error {
	if amqpURL == "" {
		return nil // Empty URL is allowed (RabbitMQ disabled)
	}

	// Parse the URL
	parsedURL, err := url.Parse(amqpURL)
	if err != nil {
		return fmt.Errorf("invalid AMQP URL format: %v", err)
	}

	// Check if scheme is valid
	if parsedURL.Scheme != "amqp" && parsedURL.Scheme != "amqps" {
		return fmt.Errorf("AMQP scheme must be either 'amqp://' or 'amqps://', got: '%s://'", parsedURL.Scheme)
	}

	// Check if host is present
	if parsedURL.Host == "" {
		return fmt.Errorf("AMQP URL must include a host")
	}

	applog.Logger.LogInfo("[CONFIG] AMQP URL validation successful: %s://%s", parsedURL.Scheme, parsedURL.Host)
	return nil
}

// envInt reads an integer from the environment, falling back to the default
// when the variable is absent or not a number. An invalid value does not stop
// the service: the default is safe, and a misconfigured protection should not
// prevent boot.
func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		applog.Logger.LogWarn("[CONFIG] %s inválido (%q), usando %d", key, raw, fallback)
		return fallback
	}
	return value
}
