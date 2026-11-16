package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"transcoder/worker/serverclient"
)

var ErrProtectedLeaseLost = errors.New("protected assignment lease was lost")

type leaseRenewal func(context.Context) (time.Time, error)

// leaseGuard supplies the context for every process and transfer belonging to
// an assignment. Its deadline is conservative and local; on renewal failure it
// cancels the context before the last confirmed server lease can expire.
type leaseGuard struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	done   chan struct{}
}

func startLeaseGuard(parent context.Context, renew leaseRenewal, margin time.Duration) (*leaseGuard, error) {
	if renew == nil || margin <= 0 {
		return nil, errors.New("invalid protected lease configuration")
	}
	initialCtx, cancelInitial := context.WithTimeout(parent, 10*time.Second)
	deadline, err := renew(initialCtx)
	cancelInitial()
	if err != nil {
		return nil, fmt.Errorf("acquire protected lease: %w", err)
	}
	if !time.Now().Add(margin).Before(deadline) {
		return nil, ErrProtectedLeaseLost
	}
	ctx, cancel := context.WithCancelCause(parent)
	guard := &leaseGuard{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go guard.run(renew, deadline, margin)
	return guard, nil
}

func (g *leaseGuard) run(renew leaseRenewal, deadline time.Time, margin time.Duration) {
	defer close(g.done)
	for {
		remaining := time.Until(deadline.Add(-margin))
		if remaining <= 0 {
			g.cancel(ErrProtectedLeaseLost)
			return
		}
		wait := remaining / 3
		if wait > 10*time.Second {
			wait = 10 * time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-g.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		requestCtx, cancelRequest := context.WithDeadline(g.ctx, deadline.Add(-margin))
		next, err := renew(requestCtx)
		cancelRequest()
		if err != nil {
			if g.ctx.Err() != nil {
				return
			}
			if errors.Is(err, serverclient.ErrProtectedConflict) || !time.Now().Before(deadline.Add(-margin)) {
				g.cancel(ErrProtectedLeaseLost)
				return
			}
			continue
		}
		if !time.Now().Add(margin).Before(next) {
			g.cancel(ErrProtectedLeaseLost)
			return
		}
		deadline = next
	}
}

func (g *leaseGuard) Close() {
	g.cancel(context.Canceled)
	<-g.done
}
