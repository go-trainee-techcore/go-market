# Example Service

Эталонный сервис. Не деплоится и бизнес-смысла не несёт — существует, чтобы
с него копировали структуру и договорённости.

Домен намеренно примитивный: `Widget` с именем и ценой. Смотреть нужно не на
домен, а на то, как всё устроено.

## Что здесь показано

| Файл | Что демонстрирует |
|---|---|
| `cmd/main.go` | сборка зависимостей, graceful shutdown, `run()` вместо логики в `main()` |
| `internal/config/` | конфигурация из окружения, без файлов |
| `internal/entity/` | доменные типы и доменные ошибки в самом внутреннем слое |
| `internal/service/` | бизнес-логика, интерфейс репозитория на стороне потребителя |
| `internal/service/widget_test.go` | табличные тесты, заглушка вместо базы, проверка аргументов |
| `internal/repository/` | sqlx, параметризованные запросы, перевод SQL-ошибок в доменные |
| `internal/handler/` | DTO, единый формат ошибки, `/healthz`, `/readyz`, `/metrics` |
| `migrations/` | обратимая goose-миграция |
| `Dockerfile` | multi-stage, distroless, непривилегированный пользователь |

## Запуск

```bash
task up
task migrate:up -- example
POSTGRES_DB=example POSTGRES_PASSWORD=gomarket go run ./services/example/cmd
```

```bash
curl -s localhost:8080/healthz
curl -s -X POST localhost:8080/api/v1/widgets \
  -H 'Content-Type: application/json' \
  -d '{"name":"виджет","price_kop":19900}'
curl -s localhost:8080/api/v1/widgets
curl -s localhost:8080/metrics | grep http_requests_total
```
