package worker

import (
	"context"
	"sync"

	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/step"
)

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
func (c *encodeConcurrency) tryStartEncoder(pinned string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused || (!c.independent && c.active >= c.target) {
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

func (c *encodeConcurrency) finishEncoder(kind string) {
	c.mu.Lock()
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

func runEncoderQueue(ctx context.Context, queue <-chan *job.Context, c *encodeConcurrency, process func(*job.Context, string)) {
	pending := []*job.Context{}
	for {
		if ctx.Err() != nil {
			return
		}
		for i := 0; i < len(pending); {
			current := pending[i]
			if kind, ok := c.tryStartEncoder(current.EncodeKind); ok {
				pending = append(pending[:i], pending[i+1:]...)
				go func(j *job.Context, kind string) { defer c.finishEncoder(kind); process(j, kind) }(current, kind)
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
	return c.paused
}

func (c *encodeConcurrency) tryStart() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused || c.active >= c.target {
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
