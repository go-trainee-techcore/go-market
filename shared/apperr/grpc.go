package apperr

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ToGRPCCode(k Kind) codes.Code {
	switch k {
	case KindInvalid:
		return codes.InvalidArgument
	case KindNotFound:
		return codes.NotFound
	case KindConflict:
		return codes.AlreadyExists
	case KindUnauthenticated:
		return codes.Unauthenticated
	case KindForbidden:
		return codes.PermissionDenied
	case KindUnavailable:
		return codes.Unavailable
	case KindInternal:
		return codes.Internal
	default:
		return codes.Unknown
	}
}

func (e *Error) gRPCStatus() *status.Status {
	if e == nil {
		return status.New(codes.OK, "")
	}
	return status.New(ToGRPCCode(e.Kind), e.Message)
}

func FromGRPCError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return Wrap(err, KindInternal, "internal_error", "unknown error")
	}

	var kind Kind
	switch st.Code() {
	case codes.InvalidArgument:
		kind = KindInvalid
	case codes.Unauthenticated:
		kind = KindUnauthenticated
	case codes.PermissionDenied:
		kind = KindForbidden
	case codes.NotFound:
		kind = KindNotFound
	case codes.AlreadyExists:
		kind = KindConflict
	case codes.Unavailable:
		kind = KindUnavailable
	default:
		kind = KindInternal
	}

	return New(kind, st.Code().String(), st.Message())
}
