package apperr

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToGRPCCode(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		expected codes.Code
	}{
		{
			name:     "NotFound converts to codes.NotFound",
			kind:     KindNotFound,
			expected: codes.NotFound,
		},
		{
			name:     "Invalid converts to codes.InvalidArgument",
			kind:     KindInvalid,
			expected: codes.InvalidArgument,
		},
		{
			name:     "Internal converts to codes.Internal",
			kind:     KindInternal,
			expected: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := ToGRPCCode(tt.kind)
			if code != tt.expected {
				t.Errorf("We expected the code %v, but received %v.", tt.expected, code)
			}
		})
	}
}

func TestFromGRPCError(t *testing.T) {
	grpcErr := status.Error(codes.NotFound, "item not found")

	err := FromGRPCError(grpcErr)

	domainErr, ok := As(err)
	if !ok {
		t.Fatalf("We expected a domain error *apperr.Error, but received %T.", err)
	}

	if domainErr.Kind != KindNotFound {
		t.Errorf("We expected a KindNotFound, but received %v", domainErr.Kind)
	}
}
