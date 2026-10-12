// Package service содержит бизнес-логику сервиса.
//
// Слой не знает ни про HTTP, ни про SQL: зависимости приходят интерфейсами
// через конструктор. Именно это позволяет тестировать логику без базы.
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/go-trainee-techcore/go-market/services/example/internal/entity"
)

const maxNameLen = 128

// WidgetRepository описывает то, что нужно сервису от хранилища.
//
// Интерфейс объявлен здесь, на стороне потребителя, а не рядом с реализацией —
// это идиома Go: пакет объявляет то, что ему нужно, и не зависит от чужих
// абстракций.
type WidgetRepository interface {
	Create(ctx context.Context, w entity.Widget) error
	GetByID(ctx context.Context, id uuid.UUID) (entity.Widget, error)
	List(ctx context.Context, limit, offset int) ([]entity.Widget, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type Widget struct {
	repo WidgetRepository
}

func NewWidget(repo WidgetRepository) *Widget {
	return &Widget{repo: repo}
}

// CreateWidgetInput — входные данные операции создания.
type CreateWidgetInput struct {
	Name     string
	PriceKop int64
}

func (s *Widget) Create(ctx context.Context, in CreateWidgetInput) (entity.Widget, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return entity.Widget{}, entity.ErrNameRequired
	case len([]rune(name)) > maxNameLen:
		return entity.Widget{}, entity.ErrNameTooLong
	case in.PriceKop < 0:
		return entity.Widget{}, entity.ErrPriceNegative
	}

	w := entity.Widget{
		ID:       uuid.New(),
		Name:     name,
		PriceKop: in.PriceKop,
	}
	if err := s.repo.Create(ctx, w); err != nil {
		return entity.Widget{}, fmt.Errorf("создание виджета: %w", err)
	}
	return s.repo.GetByID(ctx, w.ID)
}

func (s *Widget) GetByID(ctx context.Context, id uuid.UUID) (entity.Widget, error) {
	return s.repo.GetByID(ctx, id)
}

// List возвращает страницу виджетов. Значения пагинации приводятся к
// допустимому диапазону здесь, а не в обработчике: это правило домена.
func (s *Widget) List(ctx context.Context, limit, offset int) ([]entity.Widget, error) {
	const (
		defaultLimit = 20
		maxLimit     = 100
	)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *Widget) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
