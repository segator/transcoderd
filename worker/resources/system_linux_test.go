package resources

import "testing"

func TestLinuxCountersAndMemory(t *testing.T) {
	cpu := parseCPU("cpu  10 20 30 40 50 60 70 80 1000 1000\ncpu0 1\ncpu1 1\nintr 99\n")
	if cpu == nil || cpu.total != 360 || cpu.idle != 90 || cpu.cores != 2 || cpu.scope != "host" {
		t.Fatalf("parseCPU() = %+v", cpu)
	}
	if parseCPU("cpu 1 2 bad 4") != nil {
		t.Error("invalid counters accepted")
	}
	memory := parseMemory("MemTotal: 100 kB\nMemFree: 1 kB\nMemAvailable: 40 kB\n")
	if memory == nil || memory.TotalBytes != 100*1024 || memory.UsedBytes != 60*1024 {
		t.Fatalf("parseMemory() = %+v", memory)
	}
	for _, raw := range []string{"MemTotal: 100 kB", "MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: bad kB\nMemAvailable: 1 kB"} {
		if parseMemory(raw) != nil {
			t.Error("invalid memory accepted")
		}
	}
}
