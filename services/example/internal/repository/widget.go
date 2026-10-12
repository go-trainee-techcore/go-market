// Package repository отвечает за доступ к базе и только за него.
//
// Здесь нет бизнес-правил: репозиторий переводит SQL-ошибки в доменные
// и возвращает сущности, а решения принимает слой service.
package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/go-trainee-techcore/go-market/services/example/internal/entity"
	"github.com/go-trainee-techcore/go-market/shared/apperr"
)

type Widget struct {
	db *sqlx.DB
}

func NewWidget(db *sqlx.DB) *Widget {
	return &Widget{db: db}
}

func (r *Widget) Create(ctx context.Context, w entity.Widget) error {
	// Плейсхолдеры, никогда не склейка строк: иначе SQL-инъекция.
	const q = `
		INSERT INTO widgets (id, name, price_kop)
		VALUES ($1, $2, $3)`

	if _, err := r.db.ExecContext(ctx, q, w.ID, w.Name, w.PriceKop); err != nil {
		return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка записи в базу")
	}
	return nil
}

func (r *Widget) GetByID(ctx context.Context, id uuid.UUID) (entity.Widget, error) {
	const q = `
		SELECT id, name, price_kop, created_at, updated_at
		FROM widgets
		WHERE id = $1`

	var w entity.Widget
	if err := r.db.GetContext(ctx, &w, q, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Widget{}, entity.ErrWidgetNotFound
		}
		return entity.Widget{}, apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка чтения из базы")
	}
	return w, nil
}

func (r *Widget) List(ctx context.Context, limit, offset int) ([]entity.Widget, error) {
	const q = `
		SELECT id, name, price_kop, created_at, updated_at
		FROM widgets
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	// Ёмкость не задаём заранее по limit: он приходит из запроса, и выделять
	// память под размер, который прислал клиент, нельзя — даже если сервис
	// его ограничивает. CodeQL справедливо ловит это как go/uncontrolled-allocation-size.
	var items []entity.Widget
	if err := r.db.SelectContext(ctx, &items, q, limit, offset); err != nil {
		return nil, apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка чтения из базы")
	}
	return items, nil
}

func (r *Widget) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM widgets WHERE id = $1`

	if _, err := r.db.ExecContext(ctx, q, id); err != nil {
		return apperr.Wrap(err, apperr.KindInternal, "db_error", "ошибка удаления из базы")
	}
	return nil
}
