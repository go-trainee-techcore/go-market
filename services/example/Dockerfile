# Multi-stage: в финальный образ попадает только бинарник.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Зависимости отдельным слоем — он переиспользуется, пока go.mod не менялся.
COPY go.mod go.sum ./
RUN go mod download

COPY shared/ ./shared/
COPY services/example/ ./services/example/

# CGO выключен, чтобы бинарник был статическим и запускался на пустом образе.
# Флаги -s -w убирают отладочные таблицы.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /bin/service ./services/example/cmd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/service /service
COPY --from=builder /src/services/example/migrations /migrations

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/service"]
