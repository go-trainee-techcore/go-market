// Package entity содержит доменные типы сервиса.
//
// Здесь нет ни JSON-, ни SQL-специфики за пределами тегов для sqlx: сущность
// не должна знать ни про HTTP, ни про формат ответа наружу.
package entity

import (
	"time"

	"github.com/google/uuid"
)

// Widget — учебная сущность эталонного сервиса.
type Widget struct {
	ID        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	PriceKop  int64     `db:"price_kop"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
