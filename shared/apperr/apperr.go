// Package apperr описывает доменные ошибки проекта.
//
// Правило: слой service возвращает только *apperr.Error, слой handler
// не принимает решений о HTTP-статусе сам, а отдаёт ошибку в httpx.WriteError.
// Технические ошибки (БД, сеть) оборачиваются через Wrap, чтобы причина
// осталась в цепочке и попала в лог, но наружу не утекла.
package apperr

import (
	"errors"
	"fmt"
)

// Kind — класс ошибки. По нему транспорт выбирает HTTP-статус или gRPC-код.
type Kind string

const (
	KindInvalid         Kind = "invalid_argument"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnauthenticated Kind = "unauthenticated"
	KindForbidden       Kind = "forbidden"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

// Error — доменная ошибка.
//
// Code — машинный код, часть публичного контракта: клиенты вправе на него
// полагаться, поэтому менять его можно только вместе с версией API.
// Message предназначен человеку и может меняться свободно.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// New создаёт доменную ошибку без нижележащей причины.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Wrap оборачивает техническую ошибку в доменную, сохраняя причину в цепочке.
func Wrap(cause error, kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message, cause: cause}
}

// KindOf возвращает класс ошибки. Всё, что не является доменной ошибкой,
// считается внутренней ошибкой сервиса.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

// As достаёт доменную ошибку из цепочки.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
