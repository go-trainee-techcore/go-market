package grpcx

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/go-trainee-techcore/go-market/shared/apperr"
	"github.com/go-trainee-techcore/go-market/shared/logging"
)

const requestIDHeader = "x-request-id"

var (
	serverRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Количество обработанных gRPC-запросов.",
	}, []string{"method", "code"})

	serverRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "Длительность обработки gRPC-запроса.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"method"})
)

func UnaryServerInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		start := time.Now()

		reqID := extractRequestID(ctx)
		if reqID == "" {
			reqID = uuid.NewString()
		}
		ctx = logging.WithRequestID(ctx, reqID)

		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(ctx, "panic recovered in grpc handler",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
					slog.String("stack", string(debug.Stack())),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}

			st, _ := status.FromError(err)
			codeStr := st.Code().String()
			took := time.Since(start)

			serverRequestsTotal.WithLabelValues(info.FullMethod, codeStr).Inc()
			serverRequestDuration.WithLabelValues(info.FullMethod).Observe(took.Seconds())

			if err != nil {
				log.ErrorContext(ctx, "grpc call failed",
					slog.String("method", info.FullMethod),
					slog.String("code", codeStr),
					slog.Duration("duration", took),
					slog.String("error", err.Error()),
				)
			} else {
				log.InfoContext(ctx, "grpc call finished",
					slog.String("method", info.FullMethod),
					slog.String("code", codeStr),
					slog.Duration("duration", took),
				)
			}
		}()

		resp, err = handler(ctx, req)
		if err != nil {
			if domErr, ok := apperr.As(err); ok {
				return nil, status.New(apperr.ToGRPCCode(domErr.Kind), domErr.Message).Err()
			}
			return nil, status.Errorf(codes.Internal, "internal server error")
		}

		return resp, nil
	}
}

func extractRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(requestIDHeader)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
