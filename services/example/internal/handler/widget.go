// Package handler отвечает за транспорт: разбор запроса, вызов service,
// сериализацию ответа. Бизнес-правил здесь нет.
package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/go-trainee-techcore/go-market/services/example/internal/entity"
	"github.com/go-trainee-techcore/go-market/services/example/internal/service"
	"github.com/go-trainee-techcore/go-market/shared/apperr"
	"github.com/go-trainee-techcore/go-market/shared/httpx"
)

type Widget struct {
	svc *service.Widget
	log *slog.Logger
}

func NewWidget(svc *service.Widget, log *slog.Logger) *Widget {
	return &Widget{svc: svc, log: log}
}

// createWidgetRequest и widgetResponse — DTO транспортного слоя.
// Доменная сущность наружу не отдаётся: иначе любое изменение внутри
// ломает публичный контракт.
type createWidgetRequest struct {
	Name     string `json:"name"`
	PriceKop int64  `json:"price_kop"`
}

type widgetResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	PriceKop  int64     `json:"price_kop"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type listResponse struct {
	Items []widgetResponse `json:"items"`
}

func toResponse(w entity.Widget) widgetResponse {
	return widgetResponse{
		ID:        w.ID,
		Name:      w.Name,
		PriceKop:  w.PriceKop,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}

func (h *Widget) Create(w http.ResponseWriter, r *http.Request) {
	var req createWidgetRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	created, err := h.svc.Create(r.Context(), service.CreateWidgetInput{
		Name:     req.Name,
		PriceKop: req.PriceKop,
	})
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toResponse(created))
}

// parseID достаёт идентификатор виджета из пути.
func parseID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, apperr.KindInternal, "invalid_id", "идентификатор не является UUID")
	}
	return id, nil
}

func (h *Widget) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	found, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(found))
}

func (h *Widget) List(w http.ResponseWriter, r *http.Request) {
	// Некорректные значения пагинации не ошибка: service приведёт их
	// к допустимому диапазону.
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	items, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	resp := listResponse{Items: make([]widgetResponse, 0, len(items))}
	for _, item := range items {
		resp.Items = append(resp.Items, toResponse(item))
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Widget) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}
