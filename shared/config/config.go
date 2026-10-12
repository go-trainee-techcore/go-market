// Package config загружает конфигурацию сервиса из переменных окружения.
//
// Почему env, а не файлы: в Kubernetes конфиг приходит из env и секретов,
// и локальная разработка должна повторять этот путь, а не расходиться с ним.
package config

import (
	"context"
	"fmt"

	"github.com/sethvargo/go-envconfig"
)

// Load читает структуру конфигурации из окружения.
// Поля размечаются тегами: `env:"POSTGRES_HOST, default=localhost"`.
func Load[T any](ctx context.Context) (*T, error) {
	var cfg T
	if err := envconfig.Process(ctx, &cfg); err != nil {
		return nil, fmt.Errorf("загрузка конфигурации: %w", err)
	}
	return &cfg, nil
}
