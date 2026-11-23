package step

import (
	"context"
	"sync/atomic"
	"time"
)

// storeMaximum ignores stale and repeated progress reports.
func storeMaximum(value *atomic.Int64, next int64) {
	for old := value.Load(); next > old; old = value.Load() {
		if value.CompareAndSwap(old, next) {
			return
		}
	}
}

// watchEncodeProgress measures only time during which an encoding process is
// running and not intentionally suspended. The callback cancels that process.
func watchEncodeProgress(ctx context.Context, started, done <-chan struct{}, timeout time.Duration, state func() (int64, int64, bool), cancel func()) bool {
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return false
	case <-started:
	}
	interval := timeout / 20
	if interval > 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastProgress := time.Now()
	var lastFrame, lastTime int64
	for {
		select {
		case <-ctx.Done():
			return false
		case <-done:
			return false
		case now := <-ticker.C:
			frame, timestamp, paused := state()
			if paused || frame > lastFrame || timestamp > lastTime {
				lastProgress = now
			}
			lastFrame, lastTime = frame, timestamp
			if !paused && now.Sub(lastProgress) >= timeout {
				select {
				case <-ctx.Done():
					return false
				case <-done:
					return false
				default:
				}
				cancel()
				return true
			}
		}
	}
}
