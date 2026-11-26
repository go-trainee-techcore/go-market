package grpcx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
	"github.com/go-trainee-techcore/go-market/shared/logging"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUnaryServerInterceptor_RequestIDAndErrorMapping(t *testing.T) {
	interceptor := UnaryServerInterceptor(testLogger())

	md := metadata.Pairs(requestIDHeader, "req-123")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/catalog.CatalogService/GetItem"}

	_, err := interceptor(ctx, "req", info, func(ctx context.Context, req any) (any, error) {
		gotID := logging.RequestID(ctx)
		if gotID != "req-123" {
			t.Errorf("ожидали request_id 'req-123', получили %q", gotID)
		}
		return nil, apperr.New(apperr.KindNotFound, "item_not_found", "товар не найден")
	})

	if err == nil {
		t.Fatal("ожидали ошибку, но получили nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("ожидали gRPC статус, получили %T", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("ожидали код NotFound, получили %v", st.Code())
	}
}

func TestUnaryServerInterceptor_RecoversPanic(t *testing.T) {
	interceptor := UnaryServerInterceptor(testLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/catalog.CatalogService/PanicMethod"}

	_, err := interceptor(context.Background(), "req", info, func(ctx context.Context, req any) (any, error) {
		panic("что-то пошло не так")
	})

	if err == nil {
		t.Fatal("ожидали ошибку после паники, но получили nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("ожидали gRPC статус, получили %T", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("ожидали код Internal после паники, получили %v", st.Code())
	}
}

func TestUnaryClientInterceptor_RequestIDTimeoutAndError(t *testing.T) {
	interceptor := UnaryClientInterceptor(2 * time.Second)

	ctx := logging.WithRequestID(context.Background(), "req-client-456")

	var capturedHasDeadline bool
	var capturedReqIDs []string

	err := interceptor(
		ctx,
		"/catalog.CatalogService/GetItem",
		"req",
		"reply",
		nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			_, capturedHasDeadline = ctx.Deadline()
			if md, ok := metadata.FromOutgoingContext(ctx); ok {
				capturedReqIDs = md.Get(requestIDHeader)
			}
			return status.Error(codes.NotFound, "не найдено")
		},
	)

	if !capturedHasDeadline {
		t.Error("ожидали, что клиентский перехватчик поставит таймаут (deadline)")
	}
	if len(capturedReqIDs) == 0 || capturedReqIDs[0] != "req-client-456" {
		t.Errorf("ожидали исходящий x-request-id 'req-client-456', получили %v", capturedReqIDs)
	}

	if err == nil {
		t.Fatal("ожидали ошибку, но получили nil")
	}
	domErr, ok := apperr.As(err)
	if !ok {
		t.Fatalf("ожидали *apperr.Error, но получили %T", err)
	}
	if domErr.Kind != apperr.KindNotFound {
		t.Errorf("ожидали KindNotFound, получили %v", domErr.Kind)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("не ожидали DeadlineExceeded в этом тесте")
	}
}
