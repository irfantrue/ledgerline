package config

import (
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the main structure that combines all application configurations.
type Config struct {
	App      AppConfig      `mapstructure:",squash"`
	DB       DatabaseConfig `mapstructure:",squash"`
	Log      LoggerConfig   `mapstructure:",squash"`
	RabbitMQ RabbitMQConfig `mapstructure:",squash"`
	Redis    RedisConfig    `mapstructure:",squash"`
}

// AppConfig holds specific application settings such as port and environment.
type AppConfig struct {
	AppName            string `mapstructure:"APP_NAME"`                 // Application name
	ServerHeader       string `mapstructure:"APP_SERVER_HEADER"`        // Custom server header value
	Port               string `mapstructure:"APP_PORT"`                 // HTTP server port
	Env                string `mapstructure:"APP_ENV"`                  // Environment (development, production, etc.)
	Timezone           string `mapstructure:"APP_TIMEZONE"`             // Application timezone
	CompressLevel      int    `mapstructure:"APP_COMPRESS_LEVEL"`       // Compression level (1-9)
	CorsAllowedOrigins string `mapstructure:"APP_CORS_ALLOWED_ORIGINS"` // Allowed CORS origins
	JWTSecret          string `mapstructure:"APP_JWT_SECRET"`
	JWTExpire          int    `mapstructure:"APP_JWT_EXPIRE_HOURS"`
}

// DatabaseConfig holds PostgreSQL database connection settings.
// Duration fields accept Go duration strings in env, e.g. "30m", "1h", "30s".
type DatabaseConfig struct {
	DSN               string        `mapstructure:"DB_DSN"`
	MaxOpenConns      int32         `mapstructure:"DB_MAX_OPEN_CONNS"`
	MinOpenConns      int32         `mapstructure:"DB_MIN_OPEN_CONNS"`
	MaxConnLifetime   time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	MaxConnIdleTime   time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`
	HealthCheckPeriod time.Duration `mapstructure:"DB_HEALTH_CHECK_PERIOD"`
}

// LoggerConfig holds logging configuration settings.
type LoggerConfig struct {
	Level string `mapstructure:"LOG_LEVEL"`
}

// RabbitMQConfig holds RabbitMQ connection settings.
type RabbitMQConfig struct {
	URL string `mapstructure:"RABBITMQ_URL"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	Host     string `mapstructure:"REDIS_HOST"`     // Redis server hostname
	DB       int    `mapstructure:"REDIS_DB"`       // Redis database index
	Port     string `mapstructure:"REDIS_PORT"`     // Redis server port
	Password string `mapstructure:"REDIS_PASSWORD"` // Redis password (optional)
}

// LoadConfig reads configuration from the given file (optional) and the
// process environment. Environment variables take precedence over the file.
//
// A missing file is not an error, so the app can run from env vars only
// (containers/production). Every key is registered with a default: viper's
// Unmarshal only sees env vars for keys it already knows about, so without
// defaults an env-only deployment would silently yield an empty Config.
func LoadConfig(path string) (Config, error) {
	var conf Config

	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()

	// Defaults must use keys that match mapstructure tags exactly.
	// Viper.AutomaticEnv only binds env vars for keys it already knows about.
	v.SetDefault("APP_NAME", "app")
	v.SetDefault("APP_SERVER_HEADER", "")
	v.SetDefault("APP_PORT", "8080")
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_TIMEZONE", "Asia/Jakarta")
	v.SetDefault("APP_COMPRESS_LEVEL", 1)
	v.SetDefault("APP_CORS_ALLOWED_ORIGINS", "")
	v.SetDefault("LOG_LEVEL", "info")

	v.SetDefault("DB_DSN", "")
	v.SetDefault("DB_MAX_OPEN_CONNS", 10)
	v.SetDefault("DB_MIN_OPEN_CONNS", 2)
	v.SetDefault("DB_MAX_CONN_LIFETIME", time.Hour)
	v.SetDefault("DB_MAX_CONN_IDLE_TIME", 30*time.Minute)
	v.SetDefault("DB_HEALTH_CHECK_PERIOD", time.Minute)

	v.SetDefault("REDIS_HOST", "localhost")
	v.SetDefault("REDIS_PORT", "6379")
	v.SetDefault("REDIS_DB", 0)
	v.SetDefault("REDIS_PASSWORD", "")

	v.SetDefault("RABBITMQ_URL", "")
	v.SetDefault("APP_JWT_EXPIRE_HOURS", 24)

	// Bind explicitly so AutomaticEnv picks these up even without SetDefault.
	_ = v.BindEnv("APP_JWT_SECRET")

	if err := v.ReadInConfig(); err != nil {
		// With SetConfigFile a missing file surfaces as fs.ErrNotExist,
		// not viper.ConfigFileNotFoundError, so check both.
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, fs.ErrNotExist) {
			return conf, err
		}
	}

	if err := v.Unmarshal(&conf); err != nil {
		return conf, err
	}

	if err := validateConfig(conf); err != nil {
		return conf, err
	}

	return conf, nil
}

// validateConfig checks required fields and security constraints.
func validateConfig(conf Config) error {
	if conf.DB.DSN == "" {
		return errors.New("DB_DSN is required")
	}

	// JWT secret validation
	if conf.App.JWTSecret == "" {
		return errors.New("APP_JWT_SECRET is required")
	}
	if len(conf.App.JWTSecret) < 32 {
		return errors.New("APP_JWT_SECRET must be at least 32 bytes")
	}
	// A token with a non-positive lifetime is expired the moment it is
	// created, so the service refuses to issue one.
	if conf.App.JWTExpire <= 0 {
		return errors.New("APP_JWT_EXPIRE_HOURS must be greater than zero")
	}
	if conf.App.Env == "production" {
		lower := strings.ToLower(conf.App.JWTSecret)
		if lower == "example" || lower == "secret" || lower == "changeme" {
			return errors.New("APP_JWT_SECRET cannot be a common placeholder value in production")
		}
		// An empty or wildcard origin list lets every site call the API, so
		// production has to name the origins it trusts.
		if conf.App.CorsAllowedOrigins == "" || hasWildcardOrigin(conf.App.CorsAllowedOrigins) {
			return errors.New("APP_CORS_ALLOWED_ORIGINS must list the allowed origins in production")
		}
	}

	return nil
}

// hasWildcardOrigin reports whether the origin list allows every origin.
func hasWildcardOrigin(raw string) bool {
	for _, origin := range strings.Split(raw, ",") {
		if strings.TrimSpace(origin) == "*" {
			return true
		}
	}
	return false
}
