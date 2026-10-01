package config_env

const (
	POSTGRES_AUTH_DB       = "POSTGRES_AUTH_DB"
	POSTGRES_USERS_DB      = "POSTGRES_USERS_DB"
	POSTGRES_HOST          = "POSTGRES_HOST"
	POSTGRES_PORT          = "POSTGRES_PORT"
	POSTGRES_USER          = "POSTGRES_USER"
	POSTGRES_PASSWORD      = "POSTGRES_PASSWORD"
	POSTGRES_DB            = "POSTGRES_DB"
	DATABASE_SAVE_MESSAGES = "DATABASE_SAVE_MESSAGES"
	GLOBAL_API_KEY         = "GLOBAL_API_KEY"
	WA_DEBUG               = "DEBUG_ENABLED"
	LOGTYPE                = "LOG_TYPE"
	WEBHOOKFILES           = "WEBHOOK_FILES"
	// MEDIA_LOCAL_STORE keeps a copy of message attachments on the data volume
	// so the manager can preview them without MinIO/S3. Disable to restore the
	// previous behaviour (no local copies written).
	MEDIA_LOCAL_STORE    = "MEDIA_LOCAL_STORE"
	CONNECT_ON_STARTUP   = "CONNECT_ON_STARTUP"
	REREQUEST_FROM_PHONE = "REREQUEST_FROM_PHONE"
	OS_NAME              = "OS_NAME"
	AMQP_URL             = "AMQP_URL"
	AMQP_GLOBAL_ENABLED  = "AMQP_GLOBAL_ENABLED"
	AMQP_GLOBAL_EVENTS   = "AMQP_GLOBAL_EVENTS"
	AMQP_SPECIFIC_EVENTS = "AMQP_SPECIFIC_EVENTS"
	WEBHOOK_URL          = "WEBHOOK_URL"
	// WEBHOOK_HMAC_KEY is a process-global webhook signing key. It is used for
	// instances that have no per-instance key configured.
	WEBHOOK_HMAC_KEY = "WEBHOOK_HMAC_KEY"
	// WEBHOOK_HMAC_ENCRYPTION_KEY (or GLOBAL_ENCRYPTION_KEY) encrypts the
	// per-instance HMAC keys at rest. When neither is set the key is derived
	// from GLOBAL_API_KEY so the service works with no extra configuration.
	WEBHOOK_HMAC_ENCRYPTION_KEY = "WEBHOOK_HMAC_ENCRYPTION_KEY"
	GLOBAL_ENCRYPTION_KEY       = "GLOBAL_ENCRYPTION_KEY"
	// WEBHOOK_ERROR_QUEUE_NAME is the RabbitMQ queue that receives webhooks
	// which failed permanently (all retries exhausted, or a non-retryable
	// response). Used only when AMQP_URL is set.
	WEBHOOK_ERROR_QUEUE_NAME = "WEBHOOK_ERROR_QUEUE_NAME"
	CLIENT_NAME              = "CLIENT_NAME"
	API_AUDIO_CONVERTER      = "API_AUDIO_CONVERTER"
	API_AUDIO_CONVERTER_KEY  = "API_AUDIO_CONVERTER_KEY"
	MINIO_ENDPOINT           = "MINIO_ENDPOINT"
	MINIO_ACCESS_KEY         = "MINIO_ACCESS_KEY"
	MINIO_SECRET_KEY         = "MINIO_SECRET_KEY"
	MINIO_BUCKET             = "MINIO_BUCKET"
	MINIO_USE_SSL            = "MINIO_USE_SSL"
	MINIO_ENABLED            = "MINIO_ENABLED"
	MINIO_REGION             = "MINIO_REGION"
	WHATSAPP_VERSION_MAJOR   = "WHATSAPP_VERSION_MAJOR"
	WHATSAPP_VERSION_MINOR   = "WHATSAPP_VERSION_MINOR"
	WHATSAPP_VERSION_PATCH   = "WHATSAPP_VERSION_PATCH"
	PROXY_PROTOCOL           = "PROXY_PROTOCOL"
	PROXY_HOST               = "PROXY_HOST"
	PROXY_PORT               = "PROXY_PORT"
	PROXY_USERNAME           = "PROXY_USERNAME"
	PROXY_PASSWORD           = "PROXY_PASSWORD"
	NATS_URL                 = "NATS_URL"
	NATS_GLOBAL_ENABLED      = "NATS_GLOBAL_ENABLED"
	NATS_GLOBAL_EVENTS       = "NATS_GLOBAL_EVENTS"
	EVENT_IGNORE_GROUP       = "EVENT_IGNORE_GROUP"
	EVENT_IGNORE_STATUS      = "EVENT_IGNORE_STATUS"
	QRCODE_MAX_COUNT         = "QRCODE_MAX_COUNT"
	CHECK_USER_EXISTS        = "CHECK_USER_EXISTS"
	SWAGGER_ENABLED          = "SWAGGER_ENABLED"

	// Typebot flood/loop protections, read at boot.
	TYPEBOT_CONTACT_RATE_LIMIT  = "TYPEBOT_CONTACT_RATE_LIMIT"
	TYPEBOT_CONTACT_RATE_WINDOW = "TYPEBOT_CONTACT_RATE_WINDOW"
	TYPEBOT_SEND_RATE_LIMIT     = "TYPEBOT_SEND_RATE_LIMIT"
	TYPEBOT_SEND_RATE_BURST     = "TYPEBOT_SEND_RATE_BURST"

	// Logger configurations
	LOG_MAX_SIZE    = "LOG_MAX_SIZE"
	LOG_MAX_BACKUPS = "LOG_MAX_BACKUPS"
	LOG_MAX_AGE     = "LOG_MAX_AGE"
	LOG_DIRECTORY   = "LOG_DIRECTORY"
	LOG_COMPRESS    = "LOG_COMPRESS"

	// How long the dashboard's message aggregations may be cached (seconds).
	DASHBOARD_CACHE_TTL_SECONDS = "DASHBOARD_CACHE_TTL_SECONDS"

	// How long persisted messages are kept before the cleanup job removes them.
	MESSAGE_RETENTION_DAYS = "MESSAGE_RETENTION_DAYS"

	// Postgres connection-pool sizing.
	DB_MAX_OPEN_CONNS = "DB_MAX_OPEN_CONNS"
	DB_MAX_IDLE_CONNS = "DB_MAX_IDLE_CONNS"
	// RATE_LIMIT_PER_MINUTE bounds HTTP requests per credential/IP (0 disables).
	RATE_LIMIT_PER_MINUTE = "RATE_LIMIT_PER_MINUTE"
	// CORS_ALLOWED_ORIGINS is a comma-separated allowlist ("*" reflects any).
	CORS_ALLOWED_ORIGINS = "CORS_ALLOWED_ORIGINS"
)
