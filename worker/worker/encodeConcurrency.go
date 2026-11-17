package worker

import (
	"context"
	"errors"
	"sync"

	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/step"

	"github.com/google/uuid"
)

var ErrEncodeAlreadyStarted = errors.New("encode has already started")

type encodeConcurrency struct {
	mu                       sync.Mutex
	active                   int
	target                   int
	changed                  chan struct{}
	primary                  string
	independent              bool
	cpuJobs, gpuJobs         int
	cpuActive, gpuActive     int
	cpuTarget, gpuTarget     int
	lastCPUJobs, lastGPUJobs int
	paused                   bool
	restartBarrier           bool
	reconciliationBarrier    bool
	heldJobs                 map[uuid.UUID]bool
	activeJobs               map[uuid.UUID]bool
}

func newEncodeConcurrency(target int) *encodeConcurrency {
	if target < 1 {
		target = 1
	}
	return &encodeConcurrency{target: target, primary: config.GPUEncoderKind, gpuJobs: target, gpuTarget: target, lastGPUJobs: target, changed: make(chan struct{}, 1)}
}

func (c *encodeConcurrency) SetTarget(target int) {
	if target < 1 {
		return
	}
	c.mu.Lock()
	c.target = target
	c.independent = false
	c.cpuJobs, c.gpuJobs = 0, 0
	if c.primary == config.CPUEncoderKind {
		c.cpuJobs, c.cpuTarget = target, target
	} else {
		c.gpuJobs, c.gpuTarget = target, target
	}
	c.lastCPUJobs, c.lastGPUJobs = c.cpuJobs, c.gpuJobs
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) setPrimary(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.primary = kind
	if kind == config.CPUEncoderKind {
		c.cpuJobs, c.cpuTarget = c.target, c.target
		c.gpuJobs, c.gpuTarget = 0, 0
		c.lastCPUJobs, c.lastGPUJobs = c.cpuJobs, c.gpuJobs
	}
}

func (c *encodeConcurrency) pause() {
	c.mu.Lock()
	c.cpuJobs, c.gpuJobs = 0, 0
	// Keep the independent drain policy if it was active before pausing.
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) setLimits(cpu, gpu int) {
	c.mu.Lock()
	c.independent = true
	c.cpuJobs, c.gpuJobs = cpu, gpu
	if cpu > 0 {
		c.cpuTarget = cpu
	}
	if gpu > 0 {
		c.gpuTarget = gpu
	}
	if cpu+gpu > 0 {
		c.target = cpu + gpu
		c.lastCPUJobs, c.lastGPUJobs = cpu, gpu
	} else {
		c.target = safeEncoderSum(c.cpuTarget, c.gpuTarget)
	}
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) admissionKind() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	cpu, gpu := c.cpuJobs, c.gpuJobs
	if cpu+gpu == 0 {
		cpu, gpu = c.lastCPUJobs, c.lastGPUJobs
	}
	if cpu > 0 && gpu == 0 {
		return config.CPUEncoderKind
	}
	if gpu > 0 && cpu == 0 {
		return config.GPUEncoderKind
	}
	return ""
}

func safeEncoderSum(cpu, gpu int) int {
	max := int(^uint(0) >> 1)
	if cpu > max-gpu {
		return max
	}
	return cpu + gpu
}

func (c *encodeConcurrency) encoderStatus() (cpuJobs, gpuJobs, cpuActive, gpuActive, cpuTarget, gpuTarget int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cpuJobs, c.gpuJobs, c.cpuActive, c.gpuActive, c.cpuTarget, c.gpuTarget
}

func (c *encodeConcurrency) hasIndependentLimits() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.independent
}

// A pinned class belongs to accepted/restored work. It may drain at its last
// positive target even when newly unassigned work can no longer use that class.
func (c *encodeConcurrency) tryStartEncoder(pinned string, ids ...uuid.UUID) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused || c.restartBarrier || c.reconciliationBarrier || (!c.independent && c.active >= c.target) {
		return "", false
	}
	id := uuid.Nil
	if len(ids) > 0 {
		id = ids[0]
	}
	if id != uuid.Nil && (c.heldJobs[id] || c.activeJobs[id]) {
		return "", false
	}
	try := func(kind string, restored bool) bool {
		active, target, desired := &c.gpuActive, &c.gpuTarget, c.gpuJobs
		if kind == config.CPUEncoderKind {
			active, target, desired = &c.cpuActive, &c.cpuTarget, c.cpuJobs
		}
		if c.independent && desired == 0 && !restored && c.cpuJobs+c.gpuJobs > 0 {
			return false
		}
		if restored && *target == 0 {
			*target = 1
		}
		if *active >= *target || *target == 0 {
			return false
		}
		*active++
		c.active++
		if id != uuid.Nil {
			if c.activeJobs == nil {
				c.activeJobs = make(map[uuid.UUID]bool)
			}
			c.activeJobs[id] = true
		}
		return true
	}
	if pinned == config.CPUEncoderKind || pinned == config.GPUEncoderKind {
		return pinned, try(pinned, true)
	}
	if try(c.primary, false) {
		return c.primary, true
	}
	if !c.independent {
		return "", false
	}
	other := config.CPUEncoderKind
	if c.primary == other {
		other = config.GPUEncoderKind
	}
	if try(other, false) {
		return other, true
	}
	return "", false
}

func (c *encodeConcurrency) finishEncoder(kind string, ids ...uuid.UUID) {
	c.mu.Lock()
	if len(ids) > 0 {
		delete(c.activeJobs, ids[0])
	}
	if kind == config.CPUEncoderKind && c.cpuActive > 0 {
		c.cpuActive--
	}
	if kind == config.GPUEncoderKind && c.gpuActive > 0 {
		c.gpuActive--
	}
	if c.active > 0 {
		c.active--
	}
	c.mu.Unlock()
	c.notify()
}

// holdEncoder serializes parking with the exact instant an encode slot starts.
// A hold is kept after an uncertain park response until ownership is resolved.
func (c *encodeConcurrency) holdEncoder(id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("cannot hold an unknown encode")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeJobs[id] {
		return ErrEncodeAlreadyStarted
	}
	if c.heldJobs == nil {
		c.heldJobs = make(map[uuid.UUID]bool)
	}
	c.heldJobs[id] = true
	return nil
}

func (c *encodeConcurrency) releaseEncoder(id uuid.UUID) {
	c.mu.Lock()
	delete(c.heldJobs, id)
	c.mu.Unlock()
	c.notify()
}

func runEncoderQueue(ctx context.Context, queue <-chan *job.Context, c *encodeConcurrency, process func(*job.Context, string)) {
	var running sync.WaitGroup
	defer running.Wait()
	pending := []*job.Context{}
	for {
		if ctx.Err() != nil {
			return
		}
		for i := 0; i < len(pending); {
			current := pending[i]
			if kind, ok := c.tryStartEncoder(current.EncodeKind, current.JobId); ok {
				pending = append(pending[:i], pending[i+1:]...)
				running.Add(1)
				go func(j *job.Context, kind string) {
					defer running.Done()
					defer c.finishEncoder(kind, j.JobId)
					process(j, kind)
				}(current, kind)
				continue
			}
			i++
		}
		if queue == nil && len(pending) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case next, ok := <-queue:
			if !ok {
				queue = nil
			} else {
				pending = append(pending, next)
			}
		case <-c.changed:
		}
	}
}

func (c *encodeConcurrency) Status() (active, target int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active, c.target
}

func (c *encodeConcurrency) SetPaused(paused bool) {
	c.mu.Lock()
	c.paused = paused
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) Paused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused || c.restartBarrier || c.reconciliationBarrier
}

// Reconciliation is independent of operator restart barriers: releasing one
// must never accidentally release the other.
func (c *encodeConcurrency) SetReconciliationBarrier(held bool) {
	c.mu.Lock()
	c.reconciliationBarrier = held
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) SetRestartBarrier(held bool) {
	c.mu.Lock()
	c.restartBarrier = held
	c.mu.Unlock()
	c.notify()
}

func (c *encodeConcurrency) tryStart() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused || c.restartBarrier || c.reconciliationBarrier || c.active >= c.target {
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
