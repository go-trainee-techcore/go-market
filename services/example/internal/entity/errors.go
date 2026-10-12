package entity

import "github.com/go-trainee-techcore/go-market/shared/apperr"

// Доменные ошибки живут рядом с сущностью, в самом внутреннем слое.
// Так на них могут ссылаться и service, и repository, не завися друг от друга:
// зависимости всегда направлены внутрь, к домену.
//
// Коды — часть публичного контракта, менять их можно только вместе с версией API.
var (
	ErrWidgetNotFound = apperr.New(apperr.KindNotFound, "widget_not_found", "виджет не найден")
	ErrNameRequired   = apperr.New(apperr.KindInvalid, "name_required", "имя обязательно")
	ErrNameTooLong    = apperr.New(apperr.KindInvalid, "name_too_long", "имя длиннее 128 символов")
	ErrPriceNegative  = apperr.New(apperr.KindInvalid, "price_negative", "цена не может быть отрицательной")
)
