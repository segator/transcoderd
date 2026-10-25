package worker

import (
	"context"
	"sync"

	"transcoder/worker/job"
	"transcoder/worker/step"
)

type encodeConcurrency struct {
	mu      sync.Mutex
	active  int
	target  int
	changed chan struct{}
}

func newEncodeConcurrency(target int) *encodeConcurrency {
	if target < 1 {
		target = 1
	}
	return &encodeConcurrency{target: target, changed: make(chan struct{}, 1)}
}

func (c *encodeConcurrency) SetTarget(target int) {
	if target < 1 {
		return
	}
	c.mu.Lock()
	c.target = target
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) Status() (active, target int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active, c.target
}

func (c *encodeConcurrency) tryStart() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active >= c.target {
		return false
	}
	c.active++
	return true
}

func (c *encodeConcurrency) finish() {
	c.mu.Lock()
	if c.active > 0 {
		c.active--
	}
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) notify() {
	select {
	case c.changed <- struct{}{}:
	default:
	}
}

func runDynamicStepQueue(ctx context.Context, stepExecutor *step.Executor, concurrency *encodeConcurrency, process func(*job.Context)) {
	var pending *job.Context
	for {
		if ctx.Err() != nil {
			return
		}
		if pending == nil {
			select {
			case <-ctx.Done():
				return
			case next, ok := <-stepExecutor.GetJobChan():
				if !ok {
					return
				}
				pending = next
			}
		}
		if concurrency.tryStart() {
			current := pending
			pending = nil
			go func() {
				defer concurrency.finish()
				process(current)
			}()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-concurrency.changed:
		}
	}
}
