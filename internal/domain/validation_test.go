package domain

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestSyntheticInputBoundaries(t *testing.T) {
	for _, s := range []string{"imsi-001010000000001", "imsi-001019999999999"} {
		if !SUPI(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"imsi-208010000000001", "001010000000001", "imsi-0010100001"} {
		if SUPI(s) {
			t.Fatal(s)
		}
	}
	if UUID("00000000-0000-0000-0000-000000000000") || UUID("bad") {
		t.Fatal("invalid UUID accepted")
	}
	if status.Code(IDs("bad")) != codes.InvalidArgument || status.Code(RPCError(context.DeadlineExceeded)) != codes.DeadlineExceeded {
		t.Fatal("incorrect error classification")
	}
}
