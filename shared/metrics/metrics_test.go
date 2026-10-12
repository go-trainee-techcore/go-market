package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Тесты внутри пакета: счётчики не экспортируются, а проверять нужно именно их.
// Значения сравниваются как «было → стало», потому что счётчики глобальные.

func count(method, route, status string) float64 {
	return testutil.ToFloat64(requestsTotal.WithLabelValues(method, route, status))
}

func TestMiddleware_LabelsByRoutePatternNotByPath(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/metrics-test/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	before := count(http.MethodGet, "/metrics-test/items/{id}", "200")
	for _, path := range []string{"/metrics-test/items/1", "/metrics-test/items/2"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
	}

	assert.InDelta(t, before+2, count(http.MethodGet, "/metrics-test/items/{id}", "200"), 0,
		"разные id в пути должны попадать в одну серию")
}

func TestMiddleware_CountsPanicAs500AndRepanics(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/metrics-test/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })

	before := count(http.MethodGet, "/metrics-test/boom", "500")

	require.Panics(t, func() {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics-test/boom", http.NoBody))
	}, "паника должна уйти дальше, к httpx.Recoverer")

	assert.InDelta(t, before+1, count(http.MethodGet, "/metrics-test/boom", "500"), 0,
		"запрос с паникой должен попасть в метрики как 500")
}

func TestMiddleware_EmptyResponseCountsAs200(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/metrics-test/empty", func(http.ResponseWriter, *http.Request) {})

	before := count(http.MethodGet, "/metrics-test/empty", "200")
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics-test/empty", http.NoBody))

	assert.InDelta(t, before+1, count(http.MethodGet, "/metrics-test/empty", "200"), 0,
		"обработчик ничего не записал — клиент получил 200, а не 0")
}
