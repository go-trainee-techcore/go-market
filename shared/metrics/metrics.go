// Package metrics даёт RED-метрики HTTP (Rate, Errors, Duration) и ручку /metrics.
//
// Бизнес-метрики (заказы в минуту, исходы саги, hit-rate кэша) заводятся
// в самом сервисе — здесь только общий транспортный слой.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// olfejrfjerlerjgnervgjergojevolevnjernjl
var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Количество обработанных HTTP-запросов.",
	}, []string{"method", "route", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Длительность обработки HTTP-запроса.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"method", "route"})
)

// Middleware считает RED-метрики.
//
// Лейбл route берётся из шаблона маршрута chi, а не из URL: иначе каждый
// идентификатор в пути породил бы отдельную серию и Prometheus раздулся бы.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Запись в defer: если обработчик паникует, запрос всё равно попадёт
		// в метрики как 500. Иначе паники не видны в error rate, и алерт на него
		// не сработает ровно на худшем виде сбоя. Паника пробрасывается дальше,
		// её обрабатывает httpx.Recoverer.
		defer func() {
			rec := recover()
			status := ww.Status()
			if rec != nil {
				status = http.StatusInternalServerError
			}
			observe(r, status, time.Since(start))
			if rec != nil {
				panic(rec)
			}
		}()

		next.ServeHTTP(ww, r)
	})
}

func observe(r *http.Request, status int, took time.Duration) {
	// Шаблон маршрута известен только после того, как chi отработал роутинг.
	route := "unknown"
	if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
		route = rctx.RoutePattern()
	}
	// Обработчик ничего не записал — net/http сам отдаст клиенту 200.
	if status == 0 {
		status = http.StatusOK
	}

	requestsTotal.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
	requestDuration.WithLabelValues(r.Method, route).Observe(took.Seconds())
}

// Handler возвращает обработчик ручки /metrics.
func Handler() http.Handler { return promhttp.Handler() }
