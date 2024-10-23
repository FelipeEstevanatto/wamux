package config

import (
	"os"

	"github.com/gomessguii/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	PostgresAuthDB       string
	postgresUsersDB      string
	DatabaseSaveMessages bool
	GlobalApiKey         string
	WaDebug              string
	LogType              string
	WebhookFiles         bool
	ConnectOnStartup     bool
	OsName               string
	AmqpUrl              string
	WebhookUrl           string
	ClientName           string
	ApiAudioConverter    string
	ApiAudioConverterKey string
}

func (c Config) CreateAuthDB() (*gorm.DB, error) {
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

func (c Config) CreateUsersDB() (*gorm.DB, error) {
	logger.LogDebug("Connecting to database on: %s", c.postgresUsersDB)
	db, err := gorm.Open(
		postgres.Open(c.postgresUsersDB),
		&gorm.Config{},
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func Load() *Config {
	const (
		POSTGRES_AUTH_DB        = "POSTGRES_AUTH_DB"
		POSTGRES_USERS_DB       = "POSTGRES_USERS_DB"
		DATABASE_SAVE_MESSAGES  = "DATABASE_SAVE_MESSAGES"
		GLOBAL_API_KEY          = "GLOBAL_API_KEY"
		WA_DEBUG                = "DEBUG_ENABLED"
		LOGTYPE                 = "LOG_TYPE"
		WEBHOOKFILES            = "WEBHOOK_FILES"
		CONNECT_ON_STARTUP      = "CONNECT_ON_STARTUP"
		OS_NAME                 = "OS_NAME"
		AMQP_URL                = "AMQP_URL"
		WEBHOOK_URL             = "WEBHOOK_URL"
		CLIENT_NAME             = "CLIENT_NAME"
		API_AUDIO_CONVERTER     = "API_AUDIO_CONVERTER"
		API_AUDIO_CONVERTER_KEY = "API_AUDIO_CONVERTER_KEY"
	)

	postgresAuthDB := os.Getenv(POSTGRES_AUTH_DB)
	panicIfEmpty(POSTGRES_AUTH_DB, postgresAuthDB)

	postgresUsersDB := os.Getenv(POSTGRES_USERS_DB)
	panicIfEmpty(POSTGRES_USERS_DB, postgresUsersDB)

	databaseSaveMessages := os.Getenv(DATABASE_SAVE_MESSAGES)
	panicIfEmpty(DATABASE_SAVE_MESSAGES, databaseSaveMessages)

	globalApiKey := os.Getenv(GLOBAL_API_KEY)
	panicIfEmpty(GLOBAL_API_KEY, globalApiKey)

	clientName := os.Getenv(CLIENT_NAME)

	waDebug := os.Getenv(WA_DEBUG)

	logType := os.Getenv(LOGTYPE)

	webhookFiles := os.Getenv(WEBHOOKFILES)
	if webhookFiles == "" {
		webhookFiles = "true"
	}

	connectOnStartup := os.Getenv(CONNECT_ON_STARTUP)
	if connectOnStartup == "" {
		connectOnStartup = "false"
	}

	osName := os.Getenv(OS_NAME)
	panicIfEmpty(OS_NAME, osName)

	amqpUrl := os.Getenv(AMQP_URL)

	webhookUrl := os.Getenv(WEBHOOK_URL)

	apiAudioConverter := os.Getenv(API_AUDIO_CONVERTER)
	apiAudioConverterKey := os.Getenv(API_AUDIO_CONVERTER_KEY)

	return &Config{
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
		WebhookUrl:           webhookUrl,
		ClientName:           clientName,
		ApiAudioConverter:    apiAudioConverter,
		ApiAudioConverterKey: apiAudioConverterKey,
	}
}

func panicIfEmpty(key, value string) {
	if value == "" {
		if os.Getenv("DEBUG_ENABLED") != "1" {
			logger.LogInfo("You are NOT on development mode")
		}
		logger.LogFatal("[CONFIG] variable %s must be set", key)
	}
}
