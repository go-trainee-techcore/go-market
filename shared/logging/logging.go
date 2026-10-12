// Package logging даёт единый структурированный логгер для всех сервисов.
//
// Формат один на весь проект: по нему ищут заказ по order_id и строят
// дашборды. Не заводи свой логгер и не пиши в stdout напрямую.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey int

const requestIDKey ctxKey = 0

// New возвращает JSON-логгер, автоматически подмешивающий request_id из контекста.
func New(level, service string) *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(&contextHandler{Handler: base}).With(slog.String("service", service))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// contextHandler добавляет request_id в каждую запись, если он есть в контексте.
// Благодаря этому не нужно протаскивать логгер с полями через все слои —
// достаточно передавать context.
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

// WithRequestID кладёт идентификатор запроса в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID достаёт идентификатор запроса из контекста.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
