// Эталонный сервис. Любой новый сервис начинается с копирования этой структуры.
//
// Что здесь обязательно и воспроизводится везде:
//   - конфигурация из окружения;
//   - структурированный логгер единого формата;
//   - зависимости собираются в run() и передаются конструкторами;
//   - ручки /healthz, /readyz, /metrics;
//   - graceful shutdown по SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-trainee-techcore/go-market/services/example/internal/config"
	"github.com/go-trainee-techcore/go-market/services/example/internal/handler"
	"github.com/go-trainee-techcore/go-market/services/example/internal/repository"
	"github.com/go-trainee-techcore/go-market/services/example/internal/service"
	sharedconfig "github.com/go-trainee-techcore/go-market/shared/config"
	"github.com/go-trainee-techcore/go-market/shared/logging"
	"github.com/go-trainee-techcore/go-market/shared/postgres"
)

const serviceName = "example"

func main() {
	// Вся работа в run(), чтобы отложенные вызовы отрабатывали:
	// os.Exit их не выполняет.
	if err := run(); err != nil {
		slog.Error("сервис остановлен с ошибкой", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := sharedconfig.Load[config.Config](ctx)
	if err != nil {
		return err
	}

	log := logging.New(cfg.LogLevel, serviceName)

	db, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Error("не удалось закрыть пул соединений", slog.Any("error", closeErr))
		}
	}()

	// Сборка зависимостей: снизу вверх, всё через конструкторы.
	widgetRepo := repository.NewWidget(db)
	widgetSvc := service.NewWidget(widgetRepo)
	widgetHandler := handler.NewWidget(widgetSvc, log)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.NewRouter(widgetHandler, log, db),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("сервис запущен", slog.String("addr", cfg.HTTPAddr))
		if listenErr := srv.ListenAndServe(); !errors.Is(listenErr, http.ErrServerClosed) {
			serverErr <- listenErr
		}
		close(serverErr)
	}()

	select {
	case listenErr := <-serverErr:
		return listenErr
	case <-ctx.Done():
		log.Info("получен сигнал остановки, завершаем текущие запросы")
	}

	// Свой контекст: родительский уже отменён сигналом.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("сервис остановлен")
	return nil
}
