# shared — общий код сервисов

Здесь лежит то, что одинаково во всех сервисах: формат ошибок, логирование,
метрики, HTTP-обвязка, загрузка конфигурации и подключение к базе. Если
сервисы делают одно и то же разными способами, по логам и метрикам
невозможно разбираться в системе целиком — поэтому это вынесено сюда.

| Пакет | Что делает |
|---|---|
| [`apperr`](#apperr--доменные-ошибки) | доменные ошибки и их классы; по классу выбирается HTTP-статус |
| [`logging`](#logging--логирование) | единый JSON-логгер, сам подставляет `request_id` |
| [`httpx`](#httpx--http-обвязка) | формат ответа с ошибкой, разбор JSON, заголовки Gateway, middleware |
| [`metrics`](#metrics--метрики) | RED-метрики HTTP и ручка `/metrics` |
| [`config`](#config--конфигурация) | загрузка конфигурации сервиса из переменных окружения |
| [`postgres`](#postgres--база-данных) | подключение к PostgreSQL с едиными настройками пула |

Живой пример использования всех пакетов — [`services/example`](../services/example).

## Правила для shared

- **Изменения — только через ментора.** Пакеты используют все сервисы, ошибка
  здесь ломает всех сразу. CODEOWNERS требует апрув команды менторов.
- **Отдельный PR.** Не смешивай правку в `shared/` с фичей сервиса — так её
  проще отревьюить и откатить.
- **Никакой бизнес-логики и знаний о конкретном сервисе.** Если код нужен одному
  сервису, он живёт в этом сервисе. В `shared/` его переносят, когда он
  понадобился второму.
- **Сменил сигнатуру — поправь все сервисы в том же PR.** Сломанный `develop`
  блокирует обоих.
- **На всё — тесты.** У каждого пакета здесь они уже есть, новый код без тестов
  не мержится.

## Как пакеты работают вместе: путь одного запроса

```
запрос
  │
  ▼ httpx.RequestID     берёт X-Request-Id от Gateway или генерирует, кладёт в context
  ▼ httpx.Logger        после ответа пишет одну строку: метод, путь, статус, время
  ▼ httpx.Recoverer     паника → 500 в едином формате, процесс не падает
  ▼ роутинг chi
  ▼ metrics.Middleware  только на /api/...: счётчик и гистограмма по шаблону маршрута
  ▼ handler             httpx.DecodeJSON → service → httpx.WriteJSON / httpx.WriteError
      ▼ service         бизнес-правила; возвращает доменные ошибки
      ▼ repository      SQL; ошибки базы превращает в доменные
```

Ошибка идёт обратно тем же путём. Репозиторий превращает `sql.ErrNoRows` в
`entity.ErrCartNotFound` (класс `not_found`), сервис добавляет контекст через
`%w`, хендлер отдаёт ошибку в `httpx.WriteError`. Тот по классу выбирает 404,
пишет тело `{"error": {...}}` и, если это 5xx, логирует причину с
`request_id`. По этому `request_id` запрос находится в логах всех сервисов,
через которые он прошёл.

Порядок middleware в роутере важен: внешний оборачивает следующие. `Recoverer`
стоит внутри `Logger`, поэтому запрос с паникой тоже попадает в лог — со
статусом 500.

---

## apperr — доменные ошибки

Вся ошибка, которая может дойти до клиента, — это `*apperr.Error`. У неё есть
**класс** (`Kind`), по которому транспорт выбирает статус, **код** для машины
и **сообщение** для человека.

```go
type Error struct {
	Kind    Kind   // класс: по нему выбирается HTTP-статус
	Code    string // машинный код — часть публичного контракта
	Message string // для человека, можно менять свободно
	// причина (cause) скрыта, достаётся через errors.Unwrap
}
```

### Классы

| `Kind` | HTTP | Когда использовать |
|---|---|---|
| `KindInvalid` | 400 | клиент прислал некорректные данные: пустое имя, отрицательная цена, кривой UUID |
| `KindUnauthenticated` | 401 | нет пользователя: Gateway не проставил `X-User-Id` |
| `KindForbidden` | 403 | пользователь известен, но прав нет: чужая корзина, нет роли |
| `KindNotFound` | 404 | сущности нет |
| `KindConflict` | 409 | состояние не позволяет операцию: корзина полна, заказ уже оплачен, товара не хватает |
| `KindUnavailable` | 503 | зависимость недоступна: Catalog не отвечает, открыт circuit breaker. Можно повторить |
| `KindInternal` | 500 | всё остальное: ошибка базы, баг |

Сомневаешься между `Invalid` и `Conflict`? Если тот же запрос мог бы пройти
при другом состоянии системы — это `Conflict`. Если он некорректен сам по
себе — `Invalid`.

### Функции

| Функция | Что делает |
|---|---|
| `New(kind, code, message)` | новая доменная ошибка |
| `Wrap(cause, kind, code, message)` | превращает **недоменную** ошибку (база, сеть) в доменную, сохраняя причину в цепочке |
| `KindOf(err)` | класс ошибки; для всего, что не `*apperr.Error`, — `KindInternal` |
| `As(err)` | достаёт `*apperr.Error` из цепочки |

### Как пользоваться по слоям

Ошибки объявляются один раз, в `entity/errors.go` своего сервиса:

```go
var (
	ErrCartNotFound = apperr.New(apperr.KindNotFound, "cart_not_found", "корзина не найдена")
	ErrCartFull     = apperr.New(apperr.KindConflict, "cart_full", "в корзине уже максимум позиций")
)
```

Репозиторий переводит ошибки базы в доменные:

```go
if err := r.db.GetContext(ctx, &cart, q, id); err != nil {
	if errors.Is(err, sql.ErrNoRows) {
		return entity.Cart{}, entity.ErrCartNotFound
	}
	return entity.Cart{}, apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка чтения из базы")
}
```

Сервис добавляет контекст и возвращает ошибку дальше, ничего не решая за
транспорт:

```go
cart, err := s.repo.GetByID(ctx, id)
if err != nil {
	return entity.Cart{}, fmt.Errorf("получение корзины %s: %w", id, err)
}
```

Хендлер не выбирает статус сам:

```go
if err != nil {
	httpx.WriteError(w, r, h.log, err)
	return
}
```

Проверка в коде — только через `errors.Is`:

```go
if errors.Is(err, entity.ErrCartNotFound) { ... }
```

### Подводные камни

**Ошибка сравнивается по указателю.** `errors.Is` находит ту самую
переменную `ErrCartNotFound`. Второй `apperr.New` с тем же кодом в другом
файле — это другая ошибка, и `errors.Is` её не найдёт. Поэтому одна ошибка —
одно объявление, в `entity/errors.go`.

**Не оборачивай доменную ошибку через `apperr.Wrap`.** `KindOf` берёт **первую**
`*apperr.Error` в цепочке, то есть внешнюю:

```go
// Плохо: снаружи KindInternal — клиент получит 500 вместо 404
return apperr.Wrap(entity.ErrCartNotFound, apperr.KindInternal, "db_error", "...")

// Хорошо: контекст добавлен, класс сохранён
return fmt.Errorf("получение корзины: %w", entity.ErrCartNotFound)
```

`apperr.Wrap` нужен ровно для одного — превратить чужую ошибку (`pgx`, сеть,
`json`) в доменную. Для всего остального — `fmt.Errorf` с `%w`.

**`KindInternal` наружу не раскрывается.** Клиент получает код `internal` и
общее сообщение, даже если ты передал `db_error`. Причина видна только в логе
— так в ответ не утекают строки SQL и детали реализации.

**`Code` — часть контракта.** Клиенты вправе на него полагаться, поэтому код
меняется только вместе с версией API. Формат — `snake_case`,
`<сущность>_<что_случилось>`: `cart_not_found`, `price_negative`,
`order_already_paid`. Сообщение меняй свободно.

**gRPC пока не поддержан.** Перевода `Kind` в gRPC-коды ещё нет — он появится
вместе с первым межсервисным вызовом (см. [ниже](#чего-в-shared-пока-нет)).

---

## logging — логирование

```go
log := logging.New(cfg.LogLevel, "cart")
```

`New(level, service)` возвращает `*slog.Logger`, который пишет JSON в stdout,
с полем `service` в каждой строке. Уровни: `debug`, `info`, `warn`, `error`;
неизвестный уровень считается `info`. Уровень задаёт `LOG_LEVEL`.

**`request_id` подставляется сам, если пишешь методом с контекстом:**

```go
log.InfoContext(ctx, "позиция добавлена",
	slog.String("cart_id", cartID.String()),
	slog.Int("quantity", qty))
```

```json
{"time":"2026-10-12T14:03:11.52+03:00","level":"INFO","msg":"позиция добавлена","service":"cart","cart_id":"8f2c…","quantity":2,"request_id":"6f1c…"}
```

Без контекста (`log.Info`) `request_id` в строку не попадёт, и запрос будет
не найти. В хендлерах и сервисах — всегда методы с `Context`.

`WithRequestID` и `RequestID` кладут идентификатор в контекст и достают его.
В HTTP это делает `httpx.RequestID`, руками они нужны там, где HTTP нет: в
консьюмерах Kafka и фоновых задачах. Положи туда свой идентификатор — например,
`order_id` из сообщения.

### Правила

- ключи — `snake_case`; для сквозного поиска — `user_id`, `order_id`, `cart_id`,
  `product_id`;
- в лог не попадают пароли, токены, тела запросов и персональные данные;
- уровень `Error` — только для неожиданного. «Корзина не найдена» — это не
  ошибка сервиса;
- **одну ошибку логирует кто-то один.** Возвращаемую ошибку логирует
  `httpx.WriteError` (5xx — на `Error`, 4xx — на `Debug`), поэтому в сервисе её
  не логируй. В сервисе логируй то, что **не** возвращаешь: проглоченную ошибку
  фоновой задачи, очередной ретрай;
- используй логгер, переданный через конструктор, а не `slog.Default()`: глобальный
  логгер не настроен и пишет в другом формате.

---

## httpx — HTTP-обвязка

### Формат ошибки

Один на все сервисы, менять только вместе с контрактом:

```json
{
  "error": {
    "code": "cart_not_found",
    "message": "корзина не найдена",
    "request_id": "6f1c2a9e-…"
  }
}
```

`request_id` клиент передаёт в баг-репорт, и по нему запрос находится в логах.

### Функции

| Функция | Что делает |
|---|---|
| `WriteJSON(w, status, v)` | пишет ответ с `Content-Type: application/json`; при `v == nil` — только статус (для 204) |
| `WriteError(w, r, log, err)` | переводит ошибку в статус и тело по таблице из [`apperr`](#классы); 5xx логирует на `Error` вместе с причиной, 4xx — на `Debug` |
| `DecodeJSON(r, dst)` | читает тело запроса: не больше 1 МБ, **неизвестные поля — ошибка**; при ошибке возвращает `KindInvalid` с кодом `invalid_body` |
| `UserID(r)` | `user_id` из заголовка `X-User-Id`, который ставит Gateway; если заголовка нет — `KindUnauthenticated` |

Константы заголовков: `HeaderRequestID` (`X-Request-Id`), `HeaderUserID`
(`X-User-Id`), `HeaderUserRoles` (`X-User-Roles`). Токен сервисы повторно не
проверяют: этим занимается Gateway, и заголовкам от него доверяют.

Почему `DecodeJSON` запрещает неизвестные поля: опечатка `"prise"` вместо
`"price"` иначе молча оставит цену нулём, и запрос пройдёт успешно. Лучше
получить 400.

### Типичный хендлер

```go
func (h *Cart) AddItem(w http.ResponseWriter, r *http.Request) {
	userID, err := httpx.UserID(r)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	var req addItemRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	item, err := h.svc.AddItem(r.Context(), userID, req.ProductID, req.Quantity)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toItemResponse(item))
}
```

После `WriteError` — всегда `return`. Иначе код продолжит работу и попытается
записать второй ответ поверх первого.

### Middleware

| Middleware | Что делает |
|---|---|
| `RequestID` | берёт `X-Request-Id` от Gateway или генерирует UUID; кладёт в контекст и возвращает клиенту в том же заголовке |
| `Logger(log)` | одна строка `INFO` на каждый запрос: метод, путь, статус, размер ответа, время |
| `Recoverer(log)` | паника в обработчике → 500 в едином формате и запись в лог; процесс не падает |

Подключение и порядок — как в
[`services/example/internal/handler/router.go`](../services/example/internal/handler/router.go):

```go
r.Use(httpx.RequestID)
r.Use(httpx.Logger(log))
r.Use(httpx.Recoverer(log))
```

Что `Recoverer` **не** делает:

- не ловит паники в горутинах, которые запустил обработчик, — их нужно
  перехватывать внутри самих горутин, иначе упадёт весь процесс;
- не может исправить ответ, если обработчик успел записать заголовки до паники:
  клиент получит то, что уже ушло;
- не перехватывает `http.ErrAbortHandler` — это штатный способ net/http оборвать
  ответ, а не ошибка.

---

## metrics — метрики

```go
r.Handle("/metrics", metrics.Handler())

r.Route("/api/v1/carts", func(r chi.Router) {
	r.Use(metrics.Middleware)
	// ...
})
```

`Middleware` считает **RED-метрики** — Rate, Errors, Duration — по каждому
маршруту:

| Метрика | Тип | Лейблы |
|---|---|---|
| `http_requests_total` | counter | `method`, `route`, `status` |
| `http_request_duration_seconds` | histogram | `method`, `route` |

- **`route` — это шаблон маршрута chi** (`/api/v1/carts/{id}`), а не реальный
  путь. Иначе каждый id породил бы отдельную серию, и Prometheus раздулся бы.
- **Подключай только на группу `/api/...`.** На `/metrics`, `/healthz` и
  `/readyz` метрики не нужны: Prometheus и Kubernetes дёргают их постоянно и
  только засоряют картину.
- **Паника считается как 500**, после чего уходит дальше к `Recoverer`. Без этого
  самые тяжёлые сбои не попадали бы в error rate, и алерт на него молчал бы.

Лейбл `service` в Prometheus появляется из его конфигурации
(`deploy/config/prometheus.yml`), в коде его ставить не нужно.

### Запросы, которые пригодятся

```promql
# Доля ошибок 5xx по сервисам за 5 минут
sum by (service) (rate(http_requests_total{status=~"5.."}[5m]))
  / sum by (service) (rate(http_requests_total[5m]))

# 95-й перцентиль времени ответа по маршрутам
histogram_quantile(0.95,
  sum by (le, service, route) (rate(http_request_duration_seconds_bucket[5m])))
```

### Бизнес-метрики

Заводятся в самом сервисе, рядом с логикой, которую считают:

```go
var sagaOutcomes = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "gomarket_order_saga_outcomes_total",
	Help: "Исходы саги оформления заказа.",
}, []string{"outcome"}) // completed, cancelled_no_stock, cancelled_no_funds, ...

sagaOutcomes.WithLabelValues("completed").Inc()
```

- имя — `gomarket_<сервис>_<что>_<единица>`, для счётчиков суффикс `_total`;
- **в лейблы — только значения с малым числом вариантов.** `user_id`, `order_id`,
  `product_id` в лейблах убьют Prometheus: каждое значение — новая серия.
  Идентификаторы место в логах;
- метрики объявляются **один раз на уровне пакета**, не в конструкторе:
  повторная регистрация того же имени — паника при старте.

---

## config — конфигурация

```go
cfg, err := config.Load[config.Config](ctx)
```

`Load[T]` заполняет структуру из переменных окружения по тегам. Вся
конфигурация — только из окружения: в Kubernetes она придёт из env и секретов,
и локальный запуск должен идти тем же путём.

```go
type Config struct {
	HTTPAddr        string        `env:"HTTP_ADDR, default=:8083"`
	LogLevel        string        `env:"LOG_LEVEL, default=info"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT, default=10s"`
	CatalogAddr     string        `env:"CATALOG_GRPC_ADDR, required"`
	CacheTTL        time.Duration `env:"CART_CACHE_TTL, default=5m"`

	Postgres postgres.Config // вложенная структура заполняется сама, тег не нужен
}
```

- `default=...` — значение по умолчанию, `required` — без переменной сервис не
  стартует и пишет, какой именно не хватает;
- поддерживаются строки, числа, `bool`, `time.Duration` (`"10s"`, `"5m"`),
  срезы через запятую;
- **у секретов нет значения по умолчанию, только `required`.** Пароль в
  `default=` — это пароль в репозитории; gitleaks в CI такое ловит;
- **новая переменная → строка в `.env.example`.** Это пункт чек-листа PR: иначе
  напарник не запустит сервис после твоего мержа.

Библиотека — [`sethvargo/go-envconfig`](https://github.com/sethvargo/go-envconfig).
Локально переменные из `.env` подхватываются так:

```bash
set -a; source .env; set +a
```

---

## postgres — база данных

```go
db, err := postgres.Connect(ctx, cfg.Postgres)
if err != nil {
	return err
}
defer db.Close()
```

`Connect` открывает пул, выставляет его лимиты и проверяет связь с базой
(таймаут 5 секунд). Если база недоступна — возвращает ошибку и закрывает пул.
Возвращает `*sqlx.DB`.

### Настройки

| Переменная | По умолчанию | Что это |
|---|---|---|
| `POSTGRES_HOST` | `localhost` | |
| `POSTGRES_PORT` | `5432` | |
| `POSTGRES_USER` | `gomarket` | |
| `POSTGRES_PASSWORD` | — | **обязательна** |
| `POSTGRES_DB` | — | **обязательна**: у каждого сервиса своя база |
| `POSTGRES_SSLMODE` | `disable` | |
| `POSTGRES_MAX_OPEN_CONNS` | `10` | максимум соединений в пуле |
| `POSTGRES_MAX_IDLE_CONNS` | `5` | сколько держать открытыми в простое |
| `POSTGRES_CONN_MAX_LIFETIME` | `30m` | через сколько пересоздавать соединение |

**Почему пул 10, а не больше.** Postgres в `docker-compose` разрешает 100
соединений. 6 сервисов × 10 = 60, остаётся запас на миграции, `psql` и тесты.
В Kubernetes считай так же, но умножай на число реплик: 5 сервисов × 3 реплики ×
10 = 150 — уже больше лимита.

### База на сервис

У каждого сервиса своя база: `identity`, `catalog`, `cart`, `payment`, `order`.
**Ходить в чужую базу запрещено** — только через gRPC или события Kafka. Иначе
сервисы связаны схемой, и изменить таблицу одного значит сломать другой.

Базы создаёт `deploy/config/postgres-init.sql`, но только при **первом** запуске
контейнера, на пустом томе. Добавил базу позже — пересоздай том
(`task clean && task up`) или создай базу руками.

### Миграции

- лежат в `services/<сервис>/migrations`, формат goose;
- накатываются отдельной командой: `task migrate:up -- cart`. **Сервис при
  старте миграции не запускает** — в Kubernetes это отдельный шаг, иначе
  несколько реплик начнут одну миграцию одновременно;
- каждая миграция обратима: секция `-- +goose Down` не пустая;
- уже смерженную миграцию **не редактируют** — добавляют новую. У напарника
  старая уже накатилась, и правка до него не дойдёт.

### Запросы через sqlx

```go
// одна строка
err := r.db.GetContext(ctx, &cart, `SELECT id, user_id FROM carts WHERE id = $1`, id)

// несколько строк
err := r.db.SelectContext(ctx, &items, `SELECT ... FROM cart_items WHERE cart_id = $1`, id)

// изменение
res, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE id = $1`, itemID)
```

- **только плейсхолдеры `$1, $2…`**, никакой склейки строк — это SQL-инъекция;
- поля структуры размечаются тегами `db:"..."`;
- `SelectContext` на пустой выборке возвращает пустой срез без ошибки, а **не**
  `sql.ErrNoRows`. `ErrNoRows` бывает только у `GetContext`.

### Транзакции

Нужны, когда несколько запросов должны выполниться атомарно: проверка лимита
корзины и вставка, денежная операция и запись в outbox.

```go
tx, err := r.db.BeginTxx(ctx, nil)
if err != nil {
	return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка базы")
}
// После Commit откат ничего не делает, поэтому defer безопасен.
defer func() { _ = tx.Rollback() }()

// FOR UPDATE блокирует строку корзины до конца транзакции: параллельный
// запрос на эту же корзину подождёт и увидит уже обновлённое число позиций.
var id uuid.UUID
if err := tx.GetContext(ctx, &id, `SELECT id FROM carts WHERE id = $1 FOR UPDATE`, cartID); err != nil {
	if errors.Is(err, sql.ErrNoRows) {
		return entity.ErrCartNotFound
	}
	return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка базы")
}

var count int
if err := tx.GetContext(ctx, &count, `SELECT count(*) FROM cart_items WHERE cart_id = $1`, cartID); err != nil {
	return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка базы")
}
if count >= maxItems {
	return entity.ErrCartFull
}
if _, err := tx.ExecContext(ctx, `INSERT INTO cart_items ...`, ...); err != nil {
	return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка базы")
}
return tx.Commit()
```

### Ошибки PostgreSQL

Нарушение ограничений базы — это обычно ошибка клиента, а не сервиса, и её
нужно превратить в доменную, а не отдать 500:

```go
var pgErr *pgconn.PgError // github.com/jackc/pgx/v5/pgconn
if errors.As(err, &pgErr) {
	switch pgErr.Code {
	case "23503": // foreign_key_violation — ссылка на несуществующую запись
		return entity.ErrCartNotFound
	case "23505": // unique_violation — такая запись уже есть
		return entity.ErrAlreadyExists
	case "23514": // check_violation — нарушено ограничение CHECK
		return entity.ErrInvalidState
	}
}
```

`unique_violation` пригодится для идемпотентности: повторный `Charge` с тем же
`order_id` упрётся в уникальный ключ — значит, операция уже выполнена.

---

## Чего в shared пока нет

Появится, когда понадобится, — каждое отдельным PR с ревью ментора:

| Что | Когда | Зачем |
|---|---|---|
| gRPC-обвязка | первый межсервисный вызов (Cart → Catalog) | `Kind` ↔ gRPC-коды, interceptors для `request_id`, логов, метрик и таймаутов |
| Redis | кэш корзины | клиент и cache-aside с единым TTL и инвалидацией |
| Kafka | первое событие (`user_registered`) | продюсер, консьюмер с идемпотентной обработкой, outbox |
| Надёжность | сага в Order | retry с exponential backoff и jitter, circuit breaker |
| Ошибки PostgreSQL | второй сервис, которому нужен разбор кодов | хелперы вместо `switch` по кодам выше |

Если выбор неочевиден (какую библиотеку взять, как устроить интерфейс) —
сначала короткий ADR в [`docs/adr`](../docs/adr), потом код.
