package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/go-trainee-techcore/go-market/shared/logging"
)

// RequestID кладёт идентификатор запроса в контекст и возвращает его клиенту.
// Если Gateway уже проставил заголовок — используется он, чтобы запрос можно
// было проследить сквозь все сервисы.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

// Logger пишет по одной строке на запрос.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Обработчик ничего не записал — net/http сам отдаст клиенту 200.
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			log.InfoContext(r.Context(), "запрос обработан",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("took", time.Since(start)))
		})
	}
}

// Recoverer превращает панику в 500 вместо падения процесса.
//
// Ставится в цепочке после Logger, чтобы запрос с паникой попал в лог
// со статусом 500. Паники в горутинах, запущенных обработчиком, он не ловит —
// их нужно перехватывать внутри самих горутин.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// http.ErrAbortHandler — штатный способ net/http оборвать ответ,
					// это не ошибка: пробрасываем как есть.
					if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
						panic(rec)
					}
					log.ErrorContext(r.Context(), "паника в обработчике",
						slog.Any("panic", rec),
						slog.String("path", r.URL.Path))
					WriteJSON(w, http.StatusInternalServerError, ErrorResponse{
						Error: ErrorBody{
							Code:      "internal",
							Message:   "внутренняя ошибка сервиса",
							RequestID: logging.RequestID(r.Context()),
						},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
