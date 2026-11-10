package redis

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
)

var ErrRedisUnavailable = errors.New("redis: connection refused")
var ErrKeyNotFound = errors.New("redis: key not found")

type Config struct {
	Addr     string `env:"REDIS_ADDR" default:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" default:"0"`

	PoolSize     int `env:"REDIS_POOL_SIZE" default:"10"`
	MinIdleConns int `env:"REDIS_MIN_IDLE_CONNS" default:"2"`

	DialTimeout  time.Duration `env:"REDIS_DIAL_TIMEOUT" default:"2s"`
	ReadTimeout  time.Duration `env:"REDIS_READ_TIMEOUT" default:"1s"`
	WriteTimeout time.Duration `env:"REDIS_WRITE_TIMEOUT" default:"1s"`
	MaxRetries   int           `env:"REDIS_MAX_RETRIES" default:"2"`

	TTL             time.Duration `env:"REDIS_TTL" default:"5m"`
	FailureCooldown time.Duration `env:"REDIS_FAILURE_COOLDOWN" default:"10s"`
}

type Client struct {
	rdb *redis.Client
	ttl time.Duration
	log *slog.Logger
}

func NewClient(ctx context.Context, config Config, logger *slog.Logger) (*Client, *apperr.Error) {
	c := &Client{
		rdb: redis.NewClient(&redis.Options{
			Addr:         config.Addr,
			Password:     config.Password,
			DB:           config.DB,
			PoolSize:     config.PoolSize,
			MinIdleConns: config.MinIdleConns,
			DialTimeout:  config.DialTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
			MaxRetries:   config.MaxRetries,
		}),
		ttl: config.TTL,
		log: logger,
	}

	err := c.rdb.Ping(ctx).Err()
	if err != nil {
		return nil, apperr.Wrap(ErrRedisUnavailable, apperr.KindInternal, "500", "Redis client is :"+err.Error())
	}
	return c, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	return c.rdb.Close()
}

// Get returns an error that implies going into db
// Error check through error.Is.
func (c *Client) Get(ctx context.Context, key string) (string, *apperr.Error) {

	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", apperr.Wrap(ErrKeyNotFound, apperr.KindNotFound, "404", "Key not found")
		}
		return "", apperr.Wrap(err, apperr.KindInternal, "500", "Failed to connect to read value from redis: "+err.Error())
	}
	return val, nil
}

func (c *Client) Set(ctx context.Context, key string, val any) *apperr.Error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := c.rdb.Set(ctx, key, val, c.ttl).Err(); err != nil {
		return apperr.Wrap(err, apperr.KindInternal, "500", "Failed to write to redis: "+err.Error())
	}
	return nil
}
