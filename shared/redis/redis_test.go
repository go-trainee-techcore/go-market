package redis

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
)

// testConfig — короткие таймауты и без ретраев: тесты с «упавшим» Redis быстрые.
func testConfig(addr string) Config {
	return Config{
		Addr:            addr,
		PoolSize:        2,
		DialTimeout:     200 * time.Millisecond,
		ReadTimeout:     200 * time.Millisecond,
		WriteTimeout:    200 * time.Millisecond,
		MaxRetries:      -1,
		TTL:             time.Minute,
		FailureCooldown: 10 * time.Second, // NEW
	}
}

func newTestClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, aerr := NewClient(context.Background(), cfg)
	// require.Nil, а не NoError: *apperr.Error в интерфейсе error даст typed-nil.
	require.Nil(t, aerr)
	require.NotNil(t, c)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Регрессия на замечание ревью: go-envconfig читает только
// `env:"NAME, default=..."`, тег `default:"..."` он игнорирует.
func TestConfig_EnvTags(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{"Addr", "REDIS_ADDR, default=localhost:6379"},
		{"Password", "REDIS_PASSWORD"},
		{"DB", "REDIS_DB, default=0"},
		{"PoolSize", "REDIS_POOL_SIZE, default=10"},
		{"MinIdleConns", "REDIS_MIN_IDLE_CONNS, default=2"},
		{"DialTimeout", "REDIS_DIAL_TIMEOUT, default=2s"},
		{"ReadTimeout", "REDIS_READ_TIMEOUT, default=1s"},
		{"WriteTimeout", "REDIS_WRITE_TIMEOUT, default=1s"},
		{"MaxRetries", "REDIS_MAX_RETRIES, default=2"},
		{"TTL", "REDIS_TTL, default=5m"},
		{"FailureCooldown", "REDIS_FAILURE_COOLDOWN, default=10s"}, // NEW
	}

	typ := reflect.TypeOf(Config{})
	require.Equal(t, len(tests), typ.NumField(), "новое поле Config нужно добавить в эту таблицу")

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := typ.FieldByName(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.want, f.Tag.Get("env"))
			assert.Empty(t, f.Tag.Get("default"), "тег default игнорируется go-envconfig")
		})
	}
}

func TestNewClient(t *testing.T) {
	live := func(mutate func(*Config)) func(t *testing.T) Config {
		return func(t *testing.T) Config {
			mr := miniredis.RunT(t)
			cfg := testConfig(mr.Addr())
			mutate(&cfg)
			return cfg
		}
	}

	tests := []struct {
		name         string
		cfg          func(t *testing.T) Config
		wantErr      bool
		wantTTL      time.Duration
		wantCooldown time.Duration // NEW
	}{
		{
			name:         "ttl и cooldown из конфига",
			cfg:          live(func(*Config) {}),
			wantTTL:      time.Minute,
			wantCooldown: 10 * time.Second,
		},
		{
			name:         "нулевой ttl заменяется дефолтом",
			cfg:          live(func(c *Config) { c.TTL = 0 }),
			wantTTL:      defaultTTL,
			wantCooldown: 10 * time.Second,
		},
		{
			name:         "отрицательный ttl заменяется дефолтом",
			cfg:          live(func(c *Config) { c.TTL = -time.Second }),
			wantTTL:      defaultTTL,
			wantCooldown: 10 * time.Second,
		},
		{ // NEW
			name:         "нулевой cooldown заменяется дефолтом",
			cfg:          live(func(c *Config) { c.FailureCooldown = 0 }),
			wantTTL:      time.Minute,
			wantCooldown: defaultCooldown,
		},
		{
			name: "redis недоступен",
			cfg: func(t *testing.T) Config {
				mr := miniredis.RunT(t)
				addr := mr.Addr()
				mr.Close()
				return testConfig(addr)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, aerr := NewClient(context.Background(), tt.cfg(t))

			if tt.wantErr {
				require.NotNil(t, aerr)
				require.ErrorIs(t, aerr, ErrRedisUnavailable)
				assert.Nil(t, c)
				return
			}

			require.Nil(t, aerr)
			require.NotNil(t, c)
			t.Cleanup(func() { _ = c.Close() })
			assert.Equal(t, tt.wantTTL, c.ttl)
			assert.Equal(t, tt.wantCooldown, c.cooldown) // NEW
		})
	}
}

func TestClient_Set(t *testing.T) {
	tests := []struct {
		name       string
		cfgTTL     time.Duration
		val        any
		down       bool
		wantStored string
		wantTTL    time.Duration
		wantErr    bool
	}{
		{name: "строка с ttl из конфига", cfgTTL: time.Minute, val: "v", wantStored: "v", wantTTL: time.Minute},
		{name: "число", cfgTTL: time.Minute, val: 42, wantStored: "42", wantTTL: time.Minute},
		{name: "нулевой ttl не даёт вечный ключ", cfgTTL: 0, val: "v", wantStored: "v", wantTTL: defaultTTL},
		{name: "redis недоступен", cfgTTL: time.Minute, val: "v", down: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			cfg := testConfig(mr.Addr())
			cfg.TTL = tt.cfgTTL
			c := newTestClient(t, cfg)

			if tt.down {
				mr.Close()
			}

			aerr := c.Set(context.Background(), "key", tt.val)

			if tt.wantErr {
				require.NotNil(t, aerr)
				assert.ErrorIs(t, aerr, ErrRedisUnavailable)
				return
			}

			require.Nil(t, aerr)
			got, err := mr.Get("key")
			require.NoError(t, err)
			assert.Equal(t, tt.wantStored, got)
			assert.Equal(t, tt.wantTTL, mr.TTL("key"))
		})
	}
}

func TestClient_Get(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(mr *miniredis.Miniredis)
		want    string
		wantErr error
	}{
		{
			name:    "попадание",
			prepare: func(mr *miniredis.Miniredis) { _ = mr.Set("key", "v") },
			want:    "v",
		},
		{
			name:    "промах",
			prepare: func(*miniredis.Miniredis) {},
			wantErr: ErrKeyNotFound,
		},
		{
			name: "ключ протух",
			prepare: func(mr *miniredis.Miniredis) {
				_ = mr.Set("key", "v")
				mr.SetTTL("key", time.Second)
				mr.FastForward(2 * time.Second)
			},
			wantErr: ErrKeyNotFound,
		},
		{
			name:    "redis недоступен",
			prepare: func(mr *miniredis.Miniredis) { mr.Close() },
			wantErr: ErrRedisUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			c := newTestClient(t, testConfig(mr.Addr()))
			tt.prepare(mr)

			got, aerr := c.Get(context.Background(), "key")

			if tt.wantErr != nil {
				require.NotNil(t, aerr)
				assert.ErrorIs(t, aerr, tt.wantErr)
				assert.Empty(t, got)
				if tt.wantErr == ErrRedisUnavailable {
					assert.NotErrorIs(t, aerr, ErrKeyNotFound, "сбой Redis не должен выглядеть как промах")
				}
				// NEW: промах предохранитель не взводит, сбой — взводит.
				assert.Equal(t, tt.wantErr == ErrKeyNotFound, c.available())
				return
			}

			require.Nil(t, aerr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClient_SetThenGet(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestClient(t, testConfig(mr.Addr()))
	ctx := context.Background()

	require.Nil(t, c.Set(ctx, "key", "value"))

	got, aerr := c.Get(ctx, "key")
	require.Nil(t, aerr)
	assert.Equal(t, "value", got)
}

func TestClient_Close(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) *Client
	}{
		{name: "nil-клиент", build: func(*testing.T) *Client { return nil }},
		{name: "рабочий клиент", build: func(t *testing.T) *Client {
			return newTestClient(t, testConfig(miniredis.RunT(t).Addr()))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.build(t)
			require.NoError(t, c.Close())
		})
	}
}

func TestClient_UseAfterClose(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestClient(t, testConfig(mr.Addr()))
	require.NoError(t, c.Close())

	_, aerr := c.Get(context.Background(), "key")
	require.NotNil(t, aerr)
	assert.ErrorIs(t, aerr, ErrRedisUnavailable)

	aerr = c.Set(context.Background(), "key", "v")
	require.NotNil(t, aerr)
	assert.ErrorIs(t, aerr, ErrRedisUnavailable)
}

// ---- NEW: тесты предохранителя ----

// Сценарий из объяснения: Redis упал → первый вызов идёт в сеть и взводит паузу →
// пока пауза идёт, сеть не трогаем (даже если Redis уже вернулся) →
// после паузы Redis пробуется снова.
func TestClient_Breaker(t *testing.T) {
	ctx := context.Background()
	ops := []struct {
		name string
		do   func(c *Client) *apperr.Error
		// что вернёт операция, когда Redis жив
		wantWhenAlive error
	}{
		{"get", func(c *Client) *apperr.Error { _, e := c.Get(ctx, "key"); return e }, ErrKeyNotFound},
		{"set", func(c *Client) *apperr.Error { return c.Set(ctx, "key", "v") }, nil},
	}

	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			c := newTestClient(t, testConfig(mr.Addr())) // cooldown = 10s
			base := time.Now()
			c.now = func() time.Time { return base }

			// 1. Redis упал: вызов идёт в сеть, получает ошибку, взводит паузу.
			mr.Close()
			aerr := op.do(c)
			require.NotNil(t, aerr)
			require.ErrorIs(t, aerr, ErrRedisUnavailable)
			assert.False(t, c.available())

			// 2. Redis уже вернулся, но пауза не вышла: сеть не трогаем.
			// (Если бы клиент пошёл в сеть, get вернул бы ErrKeyNotFound, set — nil.)
			mr.Restart()
			aerr = op.do(c)
			require.NotNil(t, aerr)
			require.ErrorIs(t, aerr, ErrRedisUnavailable)

			// 3. Пауза вышла: Redis пробуется снова и работает.
			c.now = func() time.Time { return base.Add(11 * time.Second) }
			assert.True(t, c.available())
			aerr = op.do(c)
			if op.wantWhenAlive == nil {
				require.Nil(t, aerr)
			} else {
				require.NotNil(t, aerr)
				require.ErrorIs(t, aerr, op.wantWhenAlive)
			}
			assert.True(t, c.available(), "успешная проба не должна взводить паузу")
		})
	}
}

func TestClient_Breaker_StillDownAfterCooldown(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestClient(t, testConfig(mr.Addr()))
	base := time.Now()
	c.now = func() time.Time { return base }

	mr.Close()
	_, aerr := c.Get(context.Background(), "key")
	require.NotNil(t, aerr)

	// Пауза вышла, но Redis всё ещё лежит: проба провалится и продлит паузу.
	c.now = func() time.Time { return base.Add(11 * time.Second) }
	require.True(t, c.available())
	_, aerr = c.Get(context.Background(), "key")
	require.NotNil(t, aerr)
	require.ErrorIs(t, aerr, ErrRedisUnavailable)

	assert.False(t, c.available(), "пауза должна быть продлена")
	c.now = func() time.Time { return base.Add(22 * time.Second) }
	assert.True(t, c.available(), "после новой паузы Redis пробуется снова")
}

// Отмена контекста вызывающего — не сбой Redis.
func TestClient_Breaker_CanceledContext(t *testing.T) {
	mr := miniredis.RunT(t)
	c := newTestClient(t, testConfig(mr.Addr()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, aerr := c.Get(ctx, "key")
	require.NotNil(t, aerr)
	assert.NotErrorIs(t, aerr, ErrRedisUnavailable)
	assert.True(t, c.available(), "отмена контекста не должна взводить паузу")
}

func TestClient_NilClient(t *testing.T) {
	var c *Client

	_, aerr := c.Get(context.Background(), "key")
	require.NotNil(t, aerr)
	assert.True(t, errors.Is(aerr, ErrRedisUnavailable))

	aerr = c.Set(context.Background(), "key", "v")
	require.NotNil(t, aerr)
	assert.True(t, errors.Is(aerr, ErrRedisUnavailable))
}
