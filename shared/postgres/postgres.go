// Package postgres даёт единый способ подключения к базе.
//
// database-per-service: у каждого сервиса своя база, ходить в чужую запрещено.
package postgres

import (
	"context"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // драйвер для database/sql
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host     string `env:"POSTGRES_HOST, default=localhost"`
	Port     int    `env:"POSTGRES_PORT, default=5432"`
	User     string `env:"POSTGRES_USER, default=gomarket"`
	Password string `env:"POSTGRES_PASSWORD, required"`
	Database string `env:"POSTGRES_DB, required"`
	SSLMode  string `env:"POSTGRES_SSLMODE, default=disable"`

	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS, default=10"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS, default=5"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME, default=30m"`
}

// DSN собирает строку подключения. Пароль экранируется, поэтому спецсимволы
// в нём безопасны.
func (c Config) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     c.Database,
		RawQuery: url.Values{"sslmode": {c.SSLMode}}.Encode(),
	}
	return u.String()
}

// Connect открывает пул и проверяет связь с базой.
func Connect(ctx context.Context, c Config) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", c.DSN())
	if err != nil {
		return nil, fmt.Errorf("открытие пула: %w", err)
	}

	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxIdleConns)
	db.SetConnMaxLifetime(c.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("проверка связи с базой %q: %w", c.Database, err)
	}
	return db, nil
}
