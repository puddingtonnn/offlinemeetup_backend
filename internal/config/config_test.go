package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RedisDefaults(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://localhost/test")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "meetuper_redis:6379", cfg.RedisAddr)
	assert.Equal(t, "", cfg.RedisPassword)
	assert.Equal(t, 0, cfg.RedisDB)
}

func TestLoad_RedisOverride(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://localhost/test")
	t.Setenv("REDIS_ADDR", "localhost:6380")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "3")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "localhost:6380", cfg.RedisAddr)
	assert.Equal(t, "secret", cfg.RedisPassword)
	assert.Equal(t, 3, cfg.RedisDB)
}

func TestLoad_MissingDSN(t *testing.T) {
	t.Setenv("DB_DSN", "")

	_, err := Load()
	require.Error(t, err)
}
