package redis

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic" // NEW: для downUntil
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
)

const (
	// defaultTTL используется, если TTL в конфиге не задан или некорректен:
	// в Redis нулевой TTL означает «ключ без срока жизни», а такого мы не допускаем.
	defaultTTL = 5 * time.Minute

	// NEW: используется, если FailureCooldown не задан (<= 0).
	defaultCooldown = 10 * time.Second
)

var (
	ErrRedisUnavailable = errors.New("redis: unavailable")
	ErrKeyNotFound      = errors.New("redis: key not found")
)

// Config читается через shared/config.Load (go-envconfig), поэтому значения
// по умолчанию задаются только в формате `env:"NAME, default=..."`.
type Config struct {
	Addr     string `env:"REDIS_ADDR, default=localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB, default=0"`

	PoolSize     int `env:"REDIS_POOL_SIZE, default=10"`
	MinIdleConns int `env:"REDIS_MIN_IDLE_CONNS, default=2"`

	DialTimeout  time.Duration `env:"REDIS_DIAL_TIMEOUT, default=2s"`
	ReadTimeout  time.Duration `env:"REDIS_READ_TIMEOUT, default=1s"`
	WriteTimeout time.Duration `env:"REDIS_WRITE_TIMEOUT, default=1s"`
	MaxRetries   int           `env:"REDIS_MAX_RETRIES, default=2"`

	TTL time.Duration `env:"REDIS_TTL, default=5m"`

	// NEW (возвращён, теперь реально используется): сколько после сбоя Redis
	// вообще не трогаем, а сразу отдаём ErrRedisUnavailable.
	FailureCooldown time.Duration `env:"REDIS_FAILURE_COOLDOWN, default=10s"`
}

type Client struct {
	rdb *redis.Client
	ttl time.Duration

	// NEW: предохранитель. downUntil — момент (unix nano), до которого Redis
	// считается недоступным. «Redis доступен» == now >= downUntil, поэтому
	// отдельно «возвращать флаг в true» не нужно: пауза просто заканчивается.
	cooldown  time.Duration
	downUntil atomic.Int64
	now       func() time.Time // NEW: подменяется в тестах
}

func NewClient(ctx context.Context, config Config) (*Client, *apperr.Error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		MaxRetries:   config.MaxRetries,
	})

	// Без изменений: недоступный Redis на старте по-прежнему ошибка.
	// (Решение «стартовать без кэша» — отдельное, см. описание PR.)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close() // не оставляем пул соединений висеть
		return nil, unavailable(err, "Failed to connect to redis")
	}

	ttl := config.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	// NEW: та же защита от нулевого значения, что и для TTL.
	cooldown := config.FailureCooldown
	if cooldown <= 0 {
		cooldown = defaultCooldown
	}

	return &Client{
		rdb:      rdb,
		ttl:      ttl,
		cooldown: cooldown, // NEW
		now:      time.Now, // NEW
	}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	return c.rdb.Close()
}

// Get возвращает значение по ключу.
// Промах — ErrKeyNotFound, сбой Redis или пауза предохранителя —
// ErrRedisUnavailable (проверка через errors.Is). В обоих случаях
// вызывающий (сервис) идёт в источник данных.
func (c *Client) Get(ctx context.Context, key string) (string, *apperr.Error) {
	// NEW: пока идёт пауза, в сеть не ходим — ответ мгновенный.
	if !c.available() {
		return "", bypassed()
	}

	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Без изменений. Промах — штатная ситуация: предохранитель НЕ взводим.
			return "", apperr.Wrap(ErrKeyNotFound, apperr.KindNotFound, "404", "Key not found")
		}
		// CHANGED: было `return "", unavailable(...)`; теперь ещё и взводим паузу.
		return "", c.fail(ctx, err, "Failed to read value from redis")
	}
	return val, nil
}

// Set записывает значение с TTL из конфигурации (ключей без срока жизни нет).
func (c *Client) Set(ctx context.Context, key string, val any) *apperr.Error {
	// NEW: то же правило, что в Get.
	if !c.available() {
		return bypassed()
	}

	if err := c.rdb.Set(ctx, key, val, c.ttl).Err(); err != nil {
		// CHANGED: было `return unavailable(...)`; теперь ещё и взводим паузу.
		return c.fail(ctx, err, "Failed to write value to redis")
	}
	return nil
}

// NEW: available — «можно ли идти в Redis прямо сейчас».
// Безопасно для nil-клиента (тогда просто false).
//
// Ограничение: когда пауза заканчивается, в Redis одновременно пойдут все
// запросы, пришедшие в этот момент; если он всё ещё лежит, каждый заплатит
// таймаут. Лечится пробным запросом через atomic.CompareAndSwap (пока не нужно).
func (c *Client) available() bool {
	if c == nil {
		return false
	}
	return c.now().UnixNano() >= c.downUntil.Load()
}

// NEW: fail обрабатывает ошибку Redis. Отмена контекста вызывающего
// (клиент оборвал запрос) сбоем Redis не считается: предохранитель не взводим
// и ErrRedisUnavailable не возвращаем. Остальные ошибки взводят паузу.
func (c *Client) fail(ctx context.Context, err error, msg string) *apperr.Error {
	if ctx.Err() != nil {
		return apperr.Wrap(ctx.Err(), apperr.KindInternal, "500", msg)
	}
	c.downUntil.Store(c.now().Add(c.cooldown).UnixNano())
	return unavailable(err, msg)
}

// NEW: bypassed — ответ, пока идёт пауза (в сеть не ходили).
func bypassed() *apperr.Error {
	return apperr.Wrap(ErrRedisUnavailable, apperr.KindInternal, "500", "Redis is temporarily bypassed")
}

// unavailable оборачивает ErrRedisUnavailable: исходная причина остаётся в
// cause (для логов), а в публичное сообщение попадает только константа.
func unavailable(err error, msg string) *apperr.Error {
	return apperr.Wrap(fmt.Errorf("%w: %v", ErrRedisUnavailable, err), apperr.KindInternal, "500", msg)
}