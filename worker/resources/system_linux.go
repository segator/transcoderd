package resources

import (
	"os"
	"strconv"
	"strings"

	"transcoder/model"
)

func pinCPUThread() func() { return func() {} }

func parseCPU(raw string) *cpuCounters {
	var cpu *cpuCounters
	cores := 0
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "cpu" {
			if len(fields) < 5 {
				return nil
			}
			cpu = &cpuCounters{scope: "host"}
			// Guest time is already included in user/nice; count only the first
			// eight counters, and treat idle plus I/O wait as non-busy time.
			for i := 1; i < len(fields) && i <= 8; i++ {
				n, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					return nil
				}
				cpu.total += n
				if i == 4 || i == 5 {
					cpu.idle += n
				}
			}
		} else if strings.HasPrefix(fields[0], "cpu") {
			if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err == nil {
				cores++
			}
		}
	}
	if cpu != nil {
		cpu.cores = cores
	}
	return cpu
}

func parseMemory(raw string) *model.MemoryResources {
	values := map[string]uint64{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || (f[0] != "MemTotal:" && f[0] != "MemAvailable:") || f[2] != "kB" {
			continue
		}
		n, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			return nil
		}
		values[f[0]] = n * 1024
	}
	total, t := values["MemTotal:"]
	available, a := values["MemAvailable:"]
	if !t || !a || total == 0 || available > total {
		return nil
	}
	return &model.MemoryResources{TotalBytes: total, UsedBytes: total - available}
}

func readSystem() systemSample {
	var s systemSample
	if raw, err := os.ReadFile("/proc/stat"); err == nil {
		s.cpu = parseCPU(string(raw))
	}
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		s.memory = parseMemory(string(raw))
	}
	return s
}
