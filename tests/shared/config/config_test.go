package configtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ledgerline/internal/shared/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600), "write env file")
	return p
}

// Regression: env-only deploys (no .env) used to yield an empty Config.
func TestLoadConfig_EnvOnlyNoFile(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", testDSN)
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	require.NoError(t, err, "LoadConfig should succeed with env-only config")

	assert.Equal(t, testDSN, conf.DB.DSN, "DB_DSN should be loaded from the environment")
	assert.Equal(t, "9090", conf.App.Port, "APP_PORT should be loaded from the environment")
}

func TestLoadConfig_Defaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	require.NoError(t, err, "LoadConfig should succeed with only the required values")

	assert.Equal(t, "8080", conf.App.Port, "default port")
	assert.Equal(t, "development", conf.App.Env, "default env")
	assert.Equal(t, "Asia/Jakarta", conf.App.Timezone, "default timezone")
	assert.Equal(t, int32(10), conf.DB.MaxOpenConns, "default max conns")
	assert.Equal(t, time.Hour, conf.DB.MaxConnLifetime, "default conn lifetime")
	// JWT expire should default to 24 when not set or zero.
	assert.Equal(t, 24, conf.App.JWTExpire, "default JWTExpire")
}

func TestLoadConfig_FileValues(t *testing.T) {
	cleanEnv(t)
	path := writeEnvFile(t, "APP_PORT=7000\nLOG_LEVEL=debug\nDB_DSN=postgres://from-file\nAPP_JWT_SECRET="+testJWTSecret+"\n")

	conf, err := config.LoadConfig(path)
	require.NoError(t, err, "LoadConfig should succeed with a readable env file")

	assert.Equal(t, "7000", conf.App.Port, "APP_PORT should come from the file")
	assert.Equal(t, "debug", conf.Log.Level, "LOG_LEVEL should come from the file")
	assert.Equal(t, "postgres://from-file", conf.DB.DSN, "DB_DSN should come from the file")
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	cleanEnv(t)
	path := writeEnvFile(t, "APP_PORT=7000\nDB_DSN=postgres://from-file\nAPP_JWT_SECRET="+testJWTSecret+"\n")
	t.Setenv("APP_PORT", "7001")

	conf, err := config.LoadConfig(path)
	require.NoError(t, err, "LoadConfig should succeed with a readable env file")

	assert.Equal(t, "7001", conf.App.Port, "the environment should override the file")
	assert.Equal(t, "postgres://from-file", conf.DB.DSN, "a file value that is not overridden should survive")
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
	require.NoError(t, err, "LoadConfig should succeed with duration strings")

	assert.Equal(t, 90*time.Minute, conf.DB.MaxConnLifetime, "DB_MAX_CONN_LIFETIME")
	assert.Equal(t, 45*time.Second, conf.DB.MaxConnIdleTime, "DB_MAX_CONN_IDLE_TIME")
	assert.Equal(t, 2*time.Minute, conf.DB.HealthCheckPeriod, "DB_HEALTH_CHECK_PERIOD")
}

func TestLoadConfig_InvalidDurationIsError(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("DB_MAX_CONN_LIFETIME", "3600") // bare number, no unit
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	_, err := config.LoadConfig(missingFile(t))
	require.Error(t, err, "LoadConfig should fail for a duration without a unit")
}

func TestLoadConfig_MissingDSN(t *testing.T) {
	cleanEnv(t)

	_, err := config.LoadConfig(missingFile(t))
	require.Error(t, err, "LoadConfig should fail when DB_DSN is missing")
}

func TestLoadConfig_MissingJWTSecret(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")

	_, err := config.LoadConfig(missingFile(t))
	require.Error(t, err, "LoadConfig should fail when APP_JWT_SECRET is missing")
}

func TestLoadConfig_JWTSecretTooShort(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", "short")

	_, err := config.LoadConfig(missingFile(t))
	require.Error(t, err, "LoadConfig should fail when APP_JWT_SECRET is too short")
}

func TestLoadConfig_JWTSecretPlaceholderInProduction(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", "example")
	t.Setenv("APP_ENV", "production")

	_, err := config.LoadConfig(missingFile(t))
	require.Error(t, err, "LoadConfig should fail when APP_JWT_SECRET is 'example' in production")
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
			if tt.wantErr {
				require.Error(t, err, "LoadConfig should fail for the CORS origins in production")
				return
			}
			require.NoError(t, err, "LoadConfig should succeed with explicit CORS origins")
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

	_, err := config.LoadConfig(missingFile(t))
	require.NoError(t, err, "LoadConfig should succeed without CORS origins in development")
}

// TestLoadConfig_JWTExpireMustBePositive checks that a lifetime of zero or
// less is refused: such a token would be expired the moment it is created.
func TestLoadConfig_JWTExpireMustBePositive(t *testing.T) {
	for _, hours := range []string{"0", "-1"} {
		cleanEnv(t)
		t.Setenv("DB_DSN", "postgres://x")
		t.Setenv("APP_JWT_SECRET", testJWTSecret)
		t.Setenv("APP_JWT_EXPIRE_HOURS", hours)

		_, err := config.LoadConfig(missingFile(t))
		require.Error(t, err, "LoadConfig should fail for APP_JWT_EXPIRE_HOURS=%s", hours)
	}
}

func TestLoadConfig_JWTExpireDefaultsTo24(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)
	// Explicitly unset APP_JWT_EXPIRE_HOURS so viper SetDefault("APP_JWT_EXPIRE_HOURS", 24) applies.

	conf, err := config.LoadConfig(missingFile(t))
	require.NoError(t, err, "LoadConfig should succeed when the JWT lifetime is not set")

	assert.Equal(t, 24, conf.App.JWTExpire, "JWTExpire should fall back to the default of 24")
}

func TestLoadConfig_RedisDefaults(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("APP_JWT_SECRET", testJWTSecret)

	conf, err := config.LoadConfig(missingFile(t))
	require.NoError(t, err, "LoadConfig should succeed without Redis settings")

	assert.Equal(t, "localhost", conf.Redis.Host, "default Redis host")
	assert.Equal(t, "6379", conf.Redis.Port, "default Redis port")
	assert.Equal(t, 0, conf.Redis.DB, "default Redis DB")
	assert.Equal(t, "", conf.Redis.Password, "default Redis password")
}
