package configtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ledgerline/internal/shared/config"
)

// allKeys are every env key LoadConfig knows. Tests blank them first so the
// developer's real shell/.env can't leak into results (viper ignores empty env vars).
var allKeys = []string{
	"APP_NAME", "APP_SERVER_HEADER", "APP_PORT", "APP_ENV", "APP_TIMEZONE",
	"APP_COMPRESS_LEVEL", "APP_CORS_ALLOWED_ORIGINS", "APP_JWT_SECRET",
	"LOG_LEVEL",
	"DB_DSN", "DB_MAX_OPEN_CONNS", "DB_MIN_OPEN_CONNS",
	"DB_MAX_CONN_LIFETIME", "DB_MAX_CONN_IDLE_TIME", "DB_HEALTH_CHECK_PERIOD",
	"REDIS_HOST", "REDIS_PORT", "REDIS_DB", "REDIS_PASSWORD",
	"RABBITMQ_URL",
}

// testDSN is a placeholder DSN used only in tests.
//
//nolint:gosec // Test-only credential placeholder, not a real password.
const testDSN = "postgres://u:p@localhost:5432/db"

// testJWTSecret is a valid 32+ byte secret for testing.
const testJWTSecret = "this-is-a-test-secret-at-least-32-bytes-long"

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
}

func missingFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".env")
}

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return p
}

// Regression: env-only deploys (no .env) used to yield an empty Config.
func TestLoadConfig_EnvOnlyNoFile(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", testDSN)
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.DB.DSN != testDSN {
		t.Errorf("DSN from env not loaded: %q", conf.DB.DSN)
	}
	if conf.App.Port != "9090" {
		t.Errorf("APP_PORT from env not loaded: %q", conf.App.Port)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.App.Port != "8080" {
		t.Errorf("default port = %q, want 8080", conf.App.Port)
	}
	if conf.App.Env != "development" {
		t.Errorf("default env = %q, want development", conf.App.Env)
	}
	if conf.App.Timezone != "Asia/Jakarta" {
		t.Errorf("default timezone = %q", conf.App.Timezone)
	}
	if conf.DB.MaxOpenConns != 10 {
		t.Errorf("default max conns = %d, want 10", conf.DB.MaxOpenConns)
	}
	if conf.DB.MaxConnLifetime != time.Hour {
		t.Errorf("default conn lifetime = %v, want 1h", conf.DB.MaxConnLifetime)
	}
	// JWT expire should default to 24 when not set or zero.
	if conf.App.JWTExpire != 24 {
		t.Errorf("default JWTExpire = %d, want 24", conf.App.JWTExpire)
	}
}

func TestLoadConfig_FileValues(t *testing.T) {
	cleanEnv(t)
	path := writeEnvFile(t, "APP_PORT=7000\nLOG_LEVEL=debug\nDB_DSN=postgres://from-file\nAPP_JWT_SECRET="+testJWTSecret+"\n")

	conf, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.App.Port != "7000" || conf.Log.Level != "debug" || conf.DB.DSN != "postgres://from-file" {
		t.Errorf("file values not loaded: %+v", conf)
	}
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	cleanEnv(t)
	path := writeEnvFile(t, "APP_PORT=7000\nDB_DSN=postgres://from-file\nAPP_JWT_SECRET="+testJWTSecret+"\n")
	t.Setenv("APP_PORT", "7001")

	conf, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.App.Port != "7001" {
		t.Errorf("env should override file, got port %q", conf.App.Port)
	}
	if conf.DB.DSN != "postgres://from-file" {
		t.Errorf("non-overridden file value lost: %q", conf.DB.DSN)
	}
}

// Regression: durations were int64 cast straight to time.Duration (nanoseconds).
func TestLoadConfig_Durations(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("DB_MAX_CONN_LIFETIME", "90m")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "45s")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "2m")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.DB.MaxConnLifetime != 90*time.Minute {
		t.Errorf("MaxConnLifetime = %v", conf.DB.MaxConnLifetime)
	}
	if conf.DB.MaxConnIdleTime != 45*time.Second {
		t.Errorf("MaxConnIdleTime = %v", conf.DB.MaxConnIdleTime)
	}
	if conf.DB.HealthCheckPeriod != 2*time.Minute {
		t.Errorf("HealthCheckPeriod = %v", conf.DB.HealthCheckPeriod)
	}
}

func TestLoadConfig_InvalidDurationIsError(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("DB_MAX_CONN_LIFETIME", "3600") // bare number, no unit
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	if _, err := config.LoadConfig(missingFile(t)); err == nil {
		t.Fatal("expected error for duration without unit, got nil")
	}
}

func TestLoadConfig_MissingDSN(t *testing.T) {
	cleanEnv(t)

	if _, err := config.LoadConfig(missingFile(t)); err == nil {
		t.Fatal("expected error when DB_DSN is missing, got nil")
	}
}

func TestLoadConfig_MissingJWTSecret(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")

	_, err := config.LoadConfig(missingFile(t))
	if err == nil {
		t.Fatal("expected error when APP_JWT_SECRET is missing, got nil")
	}
}

func TestLoadConfig_JWTSecretTooShort(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", "short")

	_, err := config.LoadConfig(missingFile(t))
	if err == nil {
		t.Fatal("expected error when APP_JWT_SECRET is too short, got nil")
	}
}

func TestLoadConfig_JWTSecretPlaceholderInProduction(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", "example")
	t.Setenv("APP_ENV", "production")

	_, err := config.LoadConfig(missingFile(t))
	if err == nil {
		t.Fatal("expected error when APP_JWT_SECRET is 'example' in production, got nil")
	}
}

// TestLoadConfig_CorsOriginsRequiredInProduction checks that production has
// to name the origins it trusts. An empty list lets every site call the API.
func TestLoadConfig_CorsOriginsRequiredInProduction(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		wantErr bool
	}{
		{
			name:    "no origins configured",
			origins: "",
			wantErr: true,
		},
		{
			name:    "wildcard origin",
			origins: "*",
			wantErr: true,
		},
		{
			name:    "wildcard among explicit origins",
			origins: "https://app.example.com,*",
			wantErr: true,
		},
		{
			name:    "explicit origins",
			origins: "https://app.example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("DB_DSN", "postgres://x")
			t.Setenv("APP_JWT_SECRET", testJWTSecret)
			t.Setenv("APP_ENV", "production")
			t.Setenv("APP_CORS_ALLOWED_ORIGINS", tt.origins)

			_, err := config.LoadConfig(missingFile(t))
			if tt.wantErr && err == nil {
				t.Fatal("LoadConfig() error = nil, want an error for the CORS origins in production")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("LoadConfig() error = %v, want nil", err)
			}
		})
	}
}

// TestLoadConfig_CorsOriginsOptionalInDevelopment checks that development
// keeps the "allow every origin" default.
func TestLoadConfig_CorsOriginsOptionalInDevelopment(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_CORS_ALLOWED_ORIGINS", "")

	if _, err := config.LoadConfig(missingFile(t)); err != nil {
		t.Fatalf("LoadConfig() error = %v, want nil in development", err)
	}
}

// TestLoadConfig_JWTExpireMustBePositive checks that a lifetime of zero or
// less is refused: such a token would be expired the moment it is created.
func TestLoadConfig_JWTExpireMustBePositive(t *testing.T) {
	for _, hours := range []string{"0", "-1"} {
		cleanEnv(t)
		t.Setenv("DB_DSN", "postgres://x")
		t.Setenv("APP_JWT_SECRET", testJWTSecret)
		t.Setenv("APP_JWT_EXPIRE_HOURS", hours)

		if _, err := config.LoadConfig(missingFile(t)); err == nil {
			t.Errorf("LoadConfig() error = nil, want an error for APP_JWT_EXPIRE_HOURS=%s", hours)
		}
	}
}

func TestLoadConfig_JWTExpireDefaultsTo24(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)
	// Explicitly unset APP_JWT_EXPIRE_HOURS so viper SetDefault("APP_JWT_EXPIRE_HOURS", 24) applies.

	conf, err := config.LoadConfig(missingFile(t))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.App.JWTExpire != 24 {
		t.Errorf("JWTExpire = %d, want 24 (default from SetDefault)", conf.App.JWTExpire)
	}
}

func TestLoadConfig_RedisDefaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if conf.Redis.Host != "localhost" {
		t.Errorf("Redis.Host = %q, want localhost", conf.Redis.Host)
	}
	if conf.Redis.Port != "6379" {
		t.Errorf("Redis.Port = %q, want 6379", conf.Redis.Port)
	}
	if conf.Redis.DB != 0 {
		t.Errorf("Redis.DB = %d, want 0", conf.Redis.DB)
	}
	if conf.Redis.Password != "" {
		t.Errorf("Redis.Password = %q, want empty string", conf.Redis.Password)
	}
}
