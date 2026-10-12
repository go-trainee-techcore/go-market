// Package config описывает конфигурацию сервиса.
//
// Вся конфигурация приходит из переменных окружения — так же, как она придёт
// в Kubernetes. Секреты в репозиторий не коммитятся, образец — в .env.example.
package config

import (
	"time"

	"github.com/go-trainee-techcore/go-market/shared/postgres"
)

type Config struct {
	HTTPAddr        string        `env:"HTTP_ADDR, default=:8080"`
	LogLevel        string        `env:"LOG_LEVEL, default=info"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT, default=10s"`

	Postgres postgres.Config
}
