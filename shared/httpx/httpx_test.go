package httpx_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
	"github.com/go-trainee-techcore/go-market/shared/httpx"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) httpx.ErrorBody {
	t.Helper()
	var resp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp.Error
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "доменная ошибка отдаёт свой код и сообщение",
			err:         apperr.New(apperr.KindNotFound, "cart_not_found", "корзина не найдена"),
			wantStatus:  http.StatusNotFound,
			wantCode:    "cart_not_found",
			wantMessage: "корзина не найдена",
		},
		{
			name:        "обёрнутая через %w доменная ошибка распознаётся",
			err:         fmt.Errorf("add item: %w", apperr.New(apperr.KindConflict, "cart_full", "корзина заполнена")),
			wantStatus:  http.StatusConflict,
			wantCode:    "cart_full",
			wantMessage: "корзина заполнена",
		},
		{
			name:        "внутренняя доменная ошибка не раскрывает код и сообщение",
			err:         apperr.Wrap(errors.New("pq: connection refused"), apperr.KindInternal, "db_error", "ошибка базы"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "internal",
			wantMessage: "внутренняя ошибка сервиса",
		},
		{
			name:        "обычная ошибка считается внутренней",
			err:         errors.New("что-то сломалось"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "internal",
			wantMessage: "внутренняя ошибка сервиса",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			httpx.WriteError(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody), discard, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
			body := decodeError(t, rec)
			assert.Equal(t, tt.wantCode, body.Code)
			assert.Equal(t, tt.wantMessage, body.Message)
		})
	}
}

func TestDecodeJSON_RejectsUnknownFields(t *testing.T) {
	t.Parallel()

	var dst struct {
		Price int64 `json:"price"`
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(`{"prise": 100}`))

	err := httpx.DecodeJSON(req, &dst)

	require.Error(t, err)
	assert.Equal(t, apperr.KindInvalid, apperr.KindOf(err), "опечатка в поле — ошибка клиента, а не молчаливый ноль")
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	var seen string
	h := httpx.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(httpx.HeaderRequestID)
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("берёт идентификатор от Gateway", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		req.Header.Set(httpx.HeaderRequestID, "req-from-gateway")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, "req-from-gateway", rec.Header().Get(httpx.HeaderRequestID))
		assert.Equal(t, "req-from-gateway", seen)
	})

	t.Run("генерирует, если заголовка нет", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))

		assert.NotEmpty(t, rec.Header().Get(httpx.HeaderRequestID))
	})
}

func TestRecoverer(t *testing.T) {
	t.Parallel()

	t.Run("паника превращается в 500 с request_id", func(t *testing.T) {
		h := httpx.RequestID(httpx.Recoverer(discard)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		})))
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		req.Header.Set(httpx.HeaderRequestID, "req-1")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		body := decodeError(t, rec)
		assert.Equal(t, "internal", body.Code)
		assert.Equal(t, "req-1", body.RequestID)
	})

	t.Run("http.ErrAbortHandler не перехватывается", func(t *testing.T) {
		h := httpx.Recoverer(discard)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))

		assert.Panics(t, func() {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
		})
	})
}
