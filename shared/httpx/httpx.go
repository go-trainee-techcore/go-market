// Package httpx содержит общий HTTP-обвяз: формат ошибок, middleware,
// чтение заголовков, которые проставляет Gateway.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
	"github.com/go-trainee-techcore/go-market/shared/logging"
)

// Заголовки, которые проставляет Gateway после проверки токена.
// Сервисы доверяют им и повторно токен не валидируют.
const (
	HeaderRequestID = "X-Request-Id"
	HeaderUserID    = "X-User-Id"
	HeaderUserRoles = "X-User-Roles"
)

// ErrorResponse — единый формат ошибки во всех сервисах проекта.
// Меняется только вместе с контрактом.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

var statusByKind = map[apperr.Kind]int{
	apperr.KindInvalid:         http.StatusBadRequest,
	apperr.KindUnauthenticated: http.StatusUnauthorized,
	apperr.KindForbidden:       http.StatusForbidden,
	apperr.KindNotFound:        http.StatusNotFound,
	apperr.KindConflict:        http.StatusConflict,
	apperr.KindUnavailable:     http.StatusServiceUnavailable,
	apperr.KindInternal:        http.StatusInternalServerError,
}

// WriteJSON отправляет тело ответа. Если v == nil, пишет только статус.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	if v == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Заголовки уже отправлены — исправить нечего, только зафиксировать.
		slog.Error("не удалось записать тело ответа", slog.Any("error", err))
	}
}

// WriteError переводит доменную ошибку в HTTP-ответ.
//
// Наружу уходит только код и сообщение доменной ошибки. Причина внутренних
// ошибок пишется в лог и клиенту не показывается — иначе утекают детали
// реализации и строки SQL.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	ctx := r.Context()
	kind := apperr.KindOf(err)

	status, ok := statusByKind[kind]
	if !ok {
		status = http.StatusInternalServerError
	}

	body := ErrorBody{
		Code:      string(apperr.KindInternal),
		Message:   "внутренняя ошибка сервиса",
		RequestID: logging.RequestID(ctx),
	}
	if domain, isDomain := apperr.As(err); isDomain && kind != apperr.KindInternal {
		body.Code = domain.Code
		body.Message = domain.Message
	}

	if status >= http.StatusInternalServerError {
		log.ErrorContext(ctx, "запрос завершился ошибкой",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err))
	} else {
		log.DebugContext(ctx, "запрос отклонён",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("code", body.Code),
			slog.Any("error", err))
	}

	WriteJSON(w, status, ErrorResponse{Error: body})
}

// DecodeJSON читает тело запроса, запрещая неизвестные поля:
// опечатка в имени поля должна быть ошибкой, а не молчаливым нулём.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apperr.Wrap(err, apperr.KindInvalid, "invalid_body", "некорректное тело запроса")
	}
	return nil
}

// UserID возвращает идентификатор пользователя, проставленный Gateway.
func UserID(r *http.Request) (string, error) {
	id := r.Header.Get(HeaderUserID)
	if id == "" {
		return "", apperr.New(apperr.KindUnauthenticated, "no_user", "запрос без идентификатора пользователя")
	}
	return id, nil
}
