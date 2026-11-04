package resources

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"transcoder/model"
)

func TestCPUCounterDeltas(t *testing.T) {
	tests := []struct {
		name              string
		previous, current *cpuCounters
		want              *float64
	}{
		{"warmup", nil, &cpuCounters{total: 10}, nil},
		{"busy", &cpuCounters{total: 100, idle: 20}, &cpuCounters{total: 200, idle: 40}, copyPointerValue(80.0)},
		{"real idle zero", &cpuCounters{total: 100, idle: 20}, &cpuCounters{total: 200, idle: 120}, copyPointerValue(0.0)},
		{"no elapsed ticks", &cpuCounters{total: 100, idle: 20}, &cpuCounters{total: 100, idle: 20}, nil},
		{"counter reset", &cpuCounters{total: 100, idle: 20}, &cpuCounters{total: 10, idle: 2}, nil},
		{"invalid idle", &cpuCounters{total: 100, idle: 20}, &cpuCounters{total: 110, idle: 40}, nil},
		{"scope changed", &cpuCounters{total: 100, scope: "host"}, &cpuCounters{total: 200, scope: "processor-group"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuPercent(tt.previous, tt.current)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("cpuPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func copyPointerValue[T any](value T) *T { return &value }

func TestSamplerSnapshotIsolationAndStop(t *testing.T) {
	s := NewSampler()
	s.latest = &model.WorkerResources{CPU: &model.CPUResources{Percent: copyPointerValue(12.0)}, Memory: &model.MemoryResources{TotalBytes: 100, UsedBytes: 50}, GPUs: []model.GPUResources{{Percent: copyPointerValue(44.0), MemoryUsedBytes: copyPointerValue(uint64(4))}}}
	copy := s.Snapshot()
	*copy.CPU.Percent = 99
	copy.Memory.UsedBytes = 99
	*copy.GPUs[0].Percent = 99
	*copy.GPUs[0].MemoryUsedBytes = 99
	if got := s.Snapshot(); *got.CPU.Percent != 12 || got.Memory.UsedBytes != 50 || *got.GPUs[0].Percent != 44 || *got.GPUs[0].MemoryUsedBytes != 4 {
		t.Fatal("Snapshot mutated cached hardware readings")
	}
	entered := make(chan struct{})
	s.system = func() systemSample { return systemSample{cpu: &cpuCounters{total: 10}} }
	s.gpu = func(ctx context.Context) ([]model.GPUResources, string, string) {
		close(entered)
		<-ctx.Done()
		return nil, "", "unavailable"
	}
	s.Start(t.Context())
	<-entered
	done := make(chan struct{})
	go func() { s.Stop(); s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel collection")
	}
}

func TestMain(m *testing.M) {
	switch os.Getenv("TRANSCODERD_RESOURCE_TEST_HELPER") {
	case "wait":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "oversize":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", maxGPUQueryBytes+1))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGPUCommandBoundsAndCancellation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"wait", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TRANSCODERD_RESOURCE_TEST_HELPER", mode)
			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			defer cancel()
			start := time.Now()
			gpus, driver, status := queryGPU(ctx, executable)
			if len(gpus) != 0 || driver != "" || status != "unavailable" || time.Since(start) > 2*time.Second {
				t.Fatalf("queryGPU() = %v %s %s; elapsed %s", gpus, driver, status, time.Since(start))
			}
		})
	}
}

func TestGPUOutputLimitDuringCopy(t *testing.T) {
	var output boundedOutput
	reader := struct{ io.Reader }{strings.NewReader(strings.Repeat("x", maxGPUQueryBytes+1000))}
	n, err := io.Copy(&output, reader)
	if err == nil || n > maxGPUQueryBytes || output.buffer.Len() > maxGPUQueryBytes {
		t.Fatalf("bounded copy = %d bytes, %v; buffer=%d", n, err, output.buffer.Len())
	}
}

func TestLiveResourceSample(t *testing.T) {
	if os.Getenv("TRANSCODERD_RESOURCE_LIVE_TEST") != "1" {
		t.Skip("explicit live host telemetry check")
	}
	s := NewSampler()
	s.Start(t.Context())
	defer s.Stop()
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		r := s.Snapshot()
		if r != nil && r.CPU != nil && r.CPU.Percent != nil && r.Memory != nil {
			if os.Getenv("TRANSCODERD_RESOURCE_LIVE_GPU") == "1" && (r.GPUStatus != "available" || len(r.GPUs) == 0) {
				t.Fatalf("live GPU telemetry unavailable: %s", r.GPUStatus)
			}
			t.Logf("Measured CPU %.1f%% (%d logical processors, %s), RAM %d/%d bytes; %d NVIDIA devices", *r.CPU.Percent, r.CPU.Cores, r.CPU.Scope, r.Memory.UsedBytes, r.Memory.TotalBytes, len(r.GPUs))
			for _, g := range r.GPUs {
				t.Logf("GPU %s: GPU=%v NVENC=%v temperature=%v power=%v", g.Name, value(g.Percent), value(g.EncoderPercent), value(g.TemperatureC), value(g.PowerWatts))
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("live CPU/memory sample not available after warm-up")
		case <-ticker.C:
		}
	}
}

func value(v *float64) any {
	if v == nil {
		return "unavailable"
	}
	return *v
}
