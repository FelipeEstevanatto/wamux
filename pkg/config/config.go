package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gomessguii/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	config_env "github.com/EvolutionAPI/evolution-go/pkg/config/env"
)

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
	ConnectOnStartup     bool
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
	ProxyHost            string
	ProxyPort            string
	ProxyUsername        string
	ProxyPassword        string
	AmqpGlobalEvents     []string
	NatsUrl              string
	NatsGlobalEnabled    bool
	NatsGlobalEvents     []string
}

func (c *Config) CreateAuthDB() (*gorm.DB, error) {
	logger.LogDebug("Connecting to database on: %s", c.PostgresAuthDB)
	db, err := gorm.Open(
		postgres.Open(c.PostgresAuthDB),
		&gorm.Config{},
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (c *Config) CreateUsersDB() (*gorm.DB, error) {
	logger.LogDebug("Connecting to database on: %s", c.postgresUsersDB)

	dbDSN := c.postgresUsersDB

	if c.postgresUsersDB == "" {
		dbDSN = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB)
	}

	db, err := gorm.Open(
		postgres.Open(dbDSN),
		&gorm.Config{},
	)
	if err != nil {
		return nil, err
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
		logger.LogFatal("[CONFIG] variables POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER, POSTGRES_PASSWORD and POSTGRES_DB must be set")
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

	connectOnStartup := os.Getenv(config_env.CONNECT_ON_STARTUP)
	if connectOnStartup == "" {
		connectOnStartup = "false"
	}

	osName := os.Getenv(config_env.OS_NAME)

	amqpUrl := os.Getenv(config_env.AMQP_URL)
	amqpGlobalEnabled := os.Getenv(config_env.AMQP_GLOBAL_ENABLED)

	webhookUrl := os.Getenv(config_env.WEBHOOK_URL)

	apiAudioConverter := os.Getenv(config_env.API_AUDIO_CONVERTER)
	apiAudioConverterKey := os.Getenv(config_env.API_AUDIO_CONVERTER_KEY)

	whatsappVersionMajor := os.Getenv(config_env.WHATSAPP_VERSION_MAJOR)
	whatsappVersionMinor := os.Getenv(config_env.WHATSAPP_VERSION_MINOR)
	whatsappVersionPatch := os.Getenv(config_env.WHATSAPP_VERSION_PATCH)

	proxyHost := os.Getenv(config_env.PROXY_HOST)
	proxyPort := os.Getenv(config_env.PROXY_PORT)
	proxyUsername := os.Getenv(config_env.PROXY_USERNAME)
	proxyPassword := os.Getenv(config_env.PROXY_PASSWORD)

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

	amqpGlobalEvents := strings.Split(os.Getenv(config_env.AMQP_GLOBAL_EVENTS), ",")
	if len(amqpGlobalEvents) == 1 && amqpGlobalEvents[0] == "" {
		amqpGlobalEvents = []string{}
	}

	natsUrl := os.Getenv(config_env.NATS_URL)
	natsGlobalEnabled := os.Getenv(config_env.NATS_GLOBAL_ENABLED)
	natsGlobalEvents := strings.Split(os.Getenv(config_env.NATS_GLOBAL_EVENTS), ",")
	if len(natsGlobalEvents) == 1 && natsGlobalEvents[0] == "" {
		natsGlobalEvents = []string{}
	}

	config := &Config{
		PostgresAuthDB:       postgresAuthDB,
		postgresUsersDB:      postgresUsersDB,
		DatabaseSaveMessages: databaseSaveMessages == "true",
		GlobalApiKey:         globalApiKey,
		WaDebug:              waDebug,
		LogType:              logType,
		WebhookFiles:         webhookFiles == "true",
		ConnectOnStartup:     connectOnStartup == "true",
		OsName:               osName,
		AmqpUrl:              amqpUrl,
		AmqpGlobalEnabled:    amqpGlobalEnabled == "true",
		WebhookUrl:           webhookUrl,
		ClientName:           clientName,
		ApiAudioConverter:    apiAudioConverter,
		ApiAudioConverterKey: apiAudioConverterKey,
		PostgresHost:         postgresHost,
		PostgresPort:         postgresPort,
		PostgresUser:         postgresUser,
		PostgresPassword:     postgresPassword,
		PostgresDB:           postgresDB,
		WhatsappVersionMajor: major,
		WhatsappVersionMinor: minor,
		WhatsappVersionPatch: patch,
		ProxyHost:            proxyHost,
		ProxyPort:            proxyPort,
		ProxyUsername:        proxyUsername,
		ProxyPassword:        proxyPassword,
		AmqpGlobalEvents:     amqpGlobalEvents,
		NatsUrl:              natsUrl,
		NatsGlobalEnabled:    natsGlobalEnabled == "true",
		NatsGlobalEvents:     natsGlobalEvents,
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

func panicIfEmpty(key, value string) {
	if value == "" {
		if os.Getenv("DEBUG_ENABLED") != "1" {
			logger.LogInfo("You are NOT on development mode")
		}
		logger.LogFatal("[CONFIG] variable %s must be set", key)
	}
}
