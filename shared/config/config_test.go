package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-trainee-techcore/go-market/shared/config"
	"github.com/go-trainee-techcore/go-market/shared/postgres"
)

// t.Setenv несовместим с t.Parallel, поэтому тесты последовательные.

type serviceConfig struct {
	HTTPAddr        string        `env:"CONFIG_TEST_HTTP_ADDR, default=:8080"`
	ShutdownTimeout time.Duration `env:"CONFIG_TEST_SHUTDOWN_TIMEOUT, default=10s"`

	Postgres postgres.Config
}

func TestLoad_FillsNestedStructAndDefaults(t *testing.T) {
	t.Setenv("POSTGRES_DB", "cart")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("CONFIG_TEST_SHUTDOWN_TIMEOUT", "3s")

	cfg, err := config.Load[serviceConfig](t.Context())

	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.HTTPAddr, "значение по умолчанию")
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout, "длительность разбирается из строки")
	assert.Equal(t, "cart", cfg.Postgres.Database, "вложенная структура заполняется без тега")
	assert.Equal(t, "secret", cfg.Postgres.Password)
	assert.Equal(t, 10, cfg.Postgres.MaxOpenConns)
}

func TestLoad_FailsWithoutRequired(t *testing.T) {
	t.Setenv("POSTGRES_DB", "cart")
	// Сначала Setenv — он вернёт исходное значение после теста, — потом убираем
	// переменную совсем: у разработчика она может быть выставлена в шелле.
	t.Setenv("POSTGRES_PASSWORD", "placeholder")
	require.NoError(t, os.Unsetenv("POSTGRES_PASSWORD"))

	_, err := config.Load[serviceConfig](t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "POSTGRES_PASSWORD")
}
