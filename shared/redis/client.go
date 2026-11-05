package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
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
	rdb      *redis.Client
	ttl      time.Duration
	cooldown time.Duration
	log      *slog.Logger

	downUntil atomic.Int64 // unix nano; до этого момента Redis обходится
	now       func() time.Time
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
		ttl:      config.TTL,
		cooldown: config.FailureCooldown,
		log:      logger,
		now:      time.Now,
	}

	_ = c.ping(ctx)
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
func (c *Client) Get(ctx context.Context, key string) ([]byte, *apperr.Error) {
	if !c.available() {
		return nil, apperr.Wrap(ErrRedisUnavailable, apperr.KindInternal, "500", "Redis client is unavailable")
	}

	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, apperr.Wrap(ErrKeyNotFound, apperr.KindNotFound, "404", "Key not found")
		}
		return nil, apperr.Wrap(err, apperr.KindInternal, "500", "Failed to connect to read value from redis: "+err.Error())
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

func (c *Client) ping(ctx context.Context) error {
	if c == nil {
		return ErrRedisUnavailable
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := c.rdb.Ping(pctx).Err(); err != nil {
		return c.fail(ctx, "ping", err) // родительский ctx: наш таймаут — это сбой
	}
	c.downUntil.Store(0)
	return nil
}

func (c *Client) fail(ctx context.Context, op string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("redis %s: %w", op, ctx.Err())
	}
	c.downUntil.Store(c.now().Add(c.cooldown).UnixNano())
	c.log.Warn("redis failure, bypassing cache",
		"op", op, "cooldown", c.cooldown, "error", err)
	return fmt.Errorf("%w: %s: %v", ErrRedisUnavailable, op, err)
}

func (c *Client) available() bool {
	if c == nil {
		return false
	}
	return c.now().UnixNano() >= c.downUntil.Load()
}
