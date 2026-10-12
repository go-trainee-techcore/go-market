package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-trainee-techcore/go-market/services/example/internal/entity"
	"github.com/go-trainee-techcore/go-market/services/example/internal/service"
)

// Пакет с суффиксом _test: тест видит сервис только через его публичный API,
// как его увидит любой другой пакет. Это не даёт тесту опереться на внутренности
// и сломаться при первом же рефакторинге.

// widgetRepoStub — ручная заглушка. Для интерфейса из четырёх методов этого
// достаточно и читается лучше сгенерированного мока; mockery имеет смысл
// на больших интерфейсах.
type widgetRepoStub struct {
	createFn  func(ctx context.Context, w entity.Widget) error
	getByIDFn func(ctx context.Context, id uuid.UUID) (entity.Widget, error)
	listFn    func(ctx context.Context, limit, offset int) ([]entity.Widget, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error

	// Записываем то, с чем сервис позвал репозиторий, чтобы проверять
	// не только результат, но и переданные аргументы.
	gotLimit  int
	gotOffset int
}

func (s *widgetRepoStub) Create(ctx context.Context, w entity.Widget) error {
	if s.createFn != nil {
		return s.createFn(ctx, w)
	}
	return nil
}

func (s *widgetRepoStub) GetByID(ctx context.Context, id uuid.UUID) (entity.Widget, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return entity.Widget{ID: id}, nil
}

func (s *widgetRepoStub) List(ctx context.Context, limit, offset int) ([]entity.Widget, error) {
	s.gotLimit, s.gotOffset = limit, offset
	if s.listFn != nil {
		return s.listFn(ctx, limit, offset)
	}
	return nil, nil
}

func (s *widgetRepoStub) Delete(ctx context.Context, id uuid.UUID) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func TestWidget_Create_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   service.CreateWidgetInput
		wantErr error
	}{
		{
			name:    "пустое имя отклоняется",
			input:   service.CreateWidgetInput{Name: "", PriceKop: 100},
			wantErr: entity.ErrNameRequired,
		},
		{
			name:    "имя из одних пробелов отклоняется",
			input:   service.CreateWidgetInput{Name: "   ", PriceKop: 100},
			wantErr: entity.ErrNameRequired,
		},
		{
			name:    "слишком длинное имя отклоняется",
			input:   service.CreateWidgetInput{Name: strings.Repeat("a", 129), PriceKop: 100},
			wantErr: entity.ErrNameTooLong,
		},
		{
			name:    "граница длины имени проходит",
			input:   service.CreateWidgetInput{Name: strings.Repeat("a", 128), PriceKop: 100},
			wantErr: nil,
		},
		{
			name:    "кириллица считается в рунах, а не в байтах",
			input:   service.CreateWidgetInput{Name: strings.Repeat("я", 128), PriceKop: 100},
			wantErr: nil,
		},
		{
			name:    "отрицательная цена отклоняется",
			input:   service.CreateWidgetInput{Name: "виджет", PriceKop: -1},
			wantErr: entity.ErrPriceNegative,
		},
		{
			name:    "нулевая цена допустима",
			input:   service.CreateWidgetInput{Name: "виджет", PriceKop: 0},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := service.NewWidget(&widgetRepoStub{})
			_, err := svc.Create(t.Context(), tt.input)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWidget_Create_TrimsName(t *testing.T) {
	t.Parallel()

	var saved entity.Widget
	repo := &widgetRepoStub{
		createFn: func(_ context.Context, w entity.Widget) error {
			saved = w
			return nil
		},
	}

	_, err := service.NewWidget(repo).Create(t.Context(), service.CreateWidgetInput{
		Name:     "  виджет  ",
		PriceKop: 100,
	})

	require.NoError(t, err)
	assert.Equal(t, "виджет", saved.Name)
	assert.NotEqual(t, uuid.Nil, saved.ID, "сервис обязан присвоить идентификатор")
}

func TestWidget_Create_PropagatesRepoError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("база недоступна")
	repo := &widgetRepoStub{
		createFn: func(context.Context, entity.Widget) error { return wantErr },
	}

	_, err := service.NewWidget(repo).Create(t.Context(), service.CreateWidgetInput{
		Name:     "виджет",
		PriceKop: 100,
	})

	require.ErrorIs(t, err, wantErr, "ошибка репозитория должна оставаться в цепочке")
}

func TestWidget_List_NormalizesPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		limit, offset         int
		wantLimit, wantOffset int
	}{
		{name: "нулевой лимит заменяется на значение по умолчанию", limit: 0, offset: 0, wantLimit: 20, wantOffset: 0},
		{name: "отрицательный лимит заменяется на значение по умолчанию", limit: -5, offset: 0, wantLimit: 20, wantOffset: 0},
		{name: "лимит выше потолка обрезается", limit: 1000, offset: 0, wantLimit: 100, wantOffset: 0},
		{name: "отрицательный сдвиг обнуляется", limit: 10, offset: -3, wantLimit: 10, wantOffset: 0},
		{name: "корректные значения проходят как есть", limit: 50, offset: 100, wantLimit: 50, wantOffset: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &widgetRepoStub{}
			_, err := service.NewWidget(repo).List(t.Context(), tt.limit, tt.offset)

			require.NoError(t, err)
			assert.Equal(t, tt.wantLimit, repo.gotLimit)
			assert.Equal(t, tt.wantOffset, repo.gotOffset)
		})
	}
}
