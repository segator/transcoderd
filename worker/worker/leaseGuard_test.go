package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"transcoder/worker/serverclient"
)

func TestLeaseGuardStopsOnConflict(t *testing.T) {
	var calls atomic.Int32
	guard, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) {
		if calls.Add(1) == 1 {
			return time.Now().Add(300 * time.Millisecond), nil
		}
		return time.Time{}, serverclient.ErrProtectedConflict
	}, 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	select {
	case <-guard.ctx.Done():
		if !errors.Is(context.Cause(guard.ctx), ErrProtectedLeaseLost) {
			t.Fatalf("lease context cause = %v", context.Cause(guard.ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after lease conflict")
	}
}

func TestLeaseGuardFailsClosedDuringOutage(t *testing.T) {
	var calls atomic.Int32
	guard, err := startLeaseGuard(t.Context(), func(ctx context.Context) (time.Time, error) {
		if calls.Add(1) == 1 {
			return time.Now().Add(200 * time.Millisecond), nil
		}
		<-ctx.Done()
		return time.Time{}, ctx.Err()
	}, 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	select {
	case <-guard.ctx.Done():
		if !errors.Is(context.Cause(guard.ctx), ErrProtectedLeaseLost) {
			t.Fatalf("lease context cause = %v", context.Cause(guard.ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop when lease renewal timed out")
	}
}

func TestLeaseGuardRequiresInitialLease(t *testing.T) {
	if _, err := startLeaseGuard(t.Context(), func(context.Context) (time.Time, error) {
		return time.Time{}, errors.New("offline")
	}, time.Second); err == nil {
		t.Fatal("lease guard started without a lease")
	}
}
