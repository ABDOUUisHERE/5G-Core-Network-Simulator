package domain

import (
	"context"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"regexp"
	"time"
)

var supiPattern = regexp.MustCompile(`^imsi-00101[0-9]{10}$`)

func SUPI(value string) bool { return supiPattern.MatchString(value) }
func UUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
func IDs(values ...string) error {
	for _, value := range values {
		if !UUID(value) {
			return status.Error(codes.InvalidArgument, "canonical nonzero UUID required")
		}
	}
	return nil
}
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func RPCError(err error) error {
	if err == nil {
		return nil
	}
	if err == context.Canceled {
		return status.Error(codes.Canceled, "request canceled")
	}
	if err == context.DeadlineExceeded {
		return status.Error(codes.DeadlineExceeded, "dependency deadline exceeded")
	}
	if status.Code(err) != codes.Unknown {
		return err
	}
	return status.Error(codes.Internal, "persistence operation failed")
}
