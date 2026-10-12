#!/usr/bin/env bash
# Проверяет покрытие бизнес-логики (пакеты internal/service).
#
# Покрытие считается не по всему репозиторию: в shared/ и handler/ много
# транспортного кода, который размывает цифру и позволяет прятать за ней
# непокрытую логику.
set -euo pipefail

PROFILE="${1:-coverage.out}"
MIN="${2:-70}"
FILTERED="$(mktemp)"
trap 'rm -f "$FILTERED"' EXIT

head -n 1 "$PROFILE" > "$FILTERED"
if ! grep '/internal/service/' "$PROFILE" >> "$FILTERED"; then
    echo "пакетов internal/service с покрытием не найдено — проверка пропущена"
    exit 0
fi

total=$(go tool cover -func="$FILTERED" | awk '/^total:/ {print substr($3, 1, length($3) - 1)}')
echo "покрытие бизнес-логики: ${total}% (минимум ${MIN}%)"

awk -v t="$total" -v m="$MIN" 'BEGIN { exit (t + 0 < m + 0) }' || {
    echo "::error::покрытие ${total}% ниже порога ${MIN}%"
    exit 1
}
