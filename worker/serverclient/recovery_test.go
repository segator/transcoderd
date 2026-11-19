package serverclient

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestRetryStartupRequestRecoversFromDNSFailure(t *testing.T) {
	calls := 0
	err := retryStartupRequest(t.Context(), time.Millisecond, "test presence", func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("startup attempt has no deadline")
		}
		calls++
		if calls == 1 {
			return &net.DNSError{Err: "no such host", IsNotFound: true}
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Errorf("startup result = %v after %d attempts, want success after two", err, calls)
	}
}

func TestRetryStartupRequestStopsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	err := retryStartupRequest(ctx, time.Hour, "test presence", func(context.Context) error {
		calls++
		cancel()
		return errors.New("offline")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Errorf("startup result = %v after %d attempts, want cancellation after one", err, calls)
	}
}
