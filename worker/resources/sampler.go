package resources

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"transcoder/model"
)

const sampleInterval = 5 * time.Second

type cpuCounters struct {
	total, idle uint64
	cores       int
	scope       string
}

type systemSample struct {
	cpu    *cpuCounters
	memory *model.MemoryResources
}

type Sampler struct {
	mu        sync.RWMutex
	latest    *model.WorkerResources
	lifecycle sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	system    func() systemSample
	gpu       func(context.Context) ([]model.GPUResources, string, string)
}

func NewSampler() *Sampler {
	return &Sampler{system: readSystem, gpu: readGPUs}
}

func (s *Sampler) Start(ctx context.Context) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		release := pinCPUThread()
		defer release()
		s.run(ctx)
	}()
}

func (s *Sampler) Stop() {
	s.lifecycle.Lock()
	cancel, done := s.cancel, s.done
	s.lifecycle.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func cpuPercent(previous, current *cpuCounters) *float64 {
	if previous == nil || current == nil || previous.scope != current.scope || previous.cores != current.cores || current.total <= previous.total || current.idle < previous.idle {
		return nil
	}
	total, idle := current.total-previous.total, current.idle-previous.idle
	if idle > total {
		return nil
	}
	v := 100 * float64(total-idle) / float64(total)
	return &v
}

func (s *Sampler) run(ctx context.Context) {
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	hostname, _ := os.Hostname()
	var previous *cpuCounters
	for {
		if ctx.Err() != nil {
			return
		}
		system := s.system()
		gpus, driver, state := s.gpu(ctx)
		if ctx.Err() != nil {
			return
		}
		sample := &model.WorkerResources{SampledAt: time.Now().UTC(), Hostname: hostname,
			OS: runtime.GOOS, Architecture: runtime.GOARCH, Memory: system.memory,
			GPUs: gpus, GPUStatus: state, DriverVersion: driver}
		if system.cpu != nil {
			sample.CPU = &model.CPUResources{Percent: cpuPercent(previous, system.cpu), Cores: system.cpu.cores, Scope: system.cpu.scope}
		}
		previous = system.cpu
		s.mu.Lock()
		s.latest = sample
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func copyPointer[T any](v *T) *T {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

func (s *Sampler) Snapshot() *model.WorkerResources {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return nil
	}
	copy := *s.latest
	if copy.CPU != nil {
		cpu := *copy.CPU
		cpu.Percent = copyPointer(cpu.Percent)
		copy.CPU = &cpu
	}
	copy.Memory = copyPointer(copy.Memory)
	copy.GPUs = append([]model.GPUResources{}, copy.GPUs...)
	for i := range copy.GPUs {
		g := &copy.GPUs[i]
		g.Percent, g.EncoderPercent, g.DecoderPercent = copyPointer(g.Percent), copyPointer(g.EncoderPercent), copyPointer(g.DecoderPercent)
		g.MemoryUsedBytes, g.MemoryTotalBytes = copyPointer(g.MemoryUsedBytes), copyPointer(g.MemoryTotalBytes)
		g.TemperatureC, g.PowerWatts, g.PowerLimitWatts = copyPointer(g.TemperatureC), copyPointer(g.PowerWatts), copyPointer(g.PowerLimitWatts)
	}
	return &copy
}
