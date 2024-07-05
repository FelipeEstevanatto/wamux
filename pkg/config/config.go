package config

import (
	"os"

	"github.com/gomessguii/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Config struct {
	PostgresAuthDB   string
	postgresUsersDB  string
	GlobalApiKey     string
	WaDebug          string
	LogType          string
	WebhookFiles     bool
	ConnectOnStartup bool
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
		POSTGRES_AUTH_DB   = "POSTGRES_AUTH_DB"
		POSTGRES_USERS_DB  = "POSTGRES_USERS_DB"
		GLOBAL_API_KEY     = "GLOBAL_API_KEY"
		WA_DEBUG           = "DEBUG_ENABLED"
		LOGTYPE            = "LOG_TYPE"
		WEBHOOKFILES       = "WEBHOOK_FILES"
		CONNECT_ON_STARTUP = "CONNECT_ON_STARTUP"
	)

	postgresAuthDB := os.Getenv(POSTGRES_AUTH_DB)
	panicIfEmpty(POSTGRES_AUTH_DB, postgresAuthDB)

	postgresUsersDB := os.Getenv(POSTGRES_USERS_DB)
	panicIfEmpty(POSTGRES_USERS_DB, postgresUsersDB)

	globalApiKey := os.Getenv(GLOBAL_API_KEY)
	panicIfEmpty(GLOBAL_API_KEY, globalApiKey)

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

	return &Config{
		PostgresAuthDB:   postgresAuthDB,
		postgresUsersDB:  postgresUsersDB,
		GlobalApiKey:     globalApiKey,
		WaDebug:          waDebug,
		LogType:          logType,
		WebhookFiles:     webhookFiles == "true",
		ConnectOnStartup: connectOnStartup == "true",
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
