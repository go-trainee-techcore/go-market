package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/go-trainee-techcore/go-market/shared/httpx"
	"github.com/go-trainee-techcore/go-market/shared/metrics"
)

// Pinger — то, что роутер должен уметь спросить у зависимости, чтобы
// ответить на /readyz.
type Pinger interface {
	PingContext(ctx context.Context) error
}

func NewRouter(h *Widget, log *slog.Logger, db Pinger) http.Handler {
	r := chi.NewRouter()

	// Порядок важен: внешний middleware оборачивает следующие. Recoverer
	// стоит внутри Logger, поэтому запрос с паникой всё равно попадёт в лог
	// со статусом 500.
	r.Use(httpx.RequestID)
	r.Use(httpx.Logger(log))
	r.Use(httpx.Recoverer(log))

	// Служебные ручки без метрик: иначе скрейп Prometheus сам себя накручивает.
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(db))
	r.Handle("/metrics", metrics.Handler())

	r.Route("/api/v1/widgets", func(r chi.Router) {
		r.Use(metrics.Middleware)

		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})

	return r
}

// healthz отвечает, пока процесс жив. Не ходит в зависимости: иначе
// Kubernetes перезапустит сервис из-за недоступной базы, что делу не поможет.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// readyz отвечает, готов ли сервис принимать трафик. Здесь зависимости
// проверять нужно: без базы обслуживать запросы нечем.
func readyz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable,
				map[string]string{"status": "база недоступна"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
