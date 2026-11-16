package main

import "transcoder/worker/control"

// A combined idle report is unknown until both profiles provide a signal.
// Contradictory counters never turn into an affirmative safety claim.
func combinedProfileSafeIdle(profiles map[string]control.AdmissionStatus) (*bool, bool) {
	cpu, cpuOK := profiles["cpu"]
	gpu, gpuOK := profiles["gpu"]
	if !cpuOK || !gpuOK {
		return nil, false
	}
	barrier := cpu.RestartBarrier && gpu.RestartBarrier
	if cpu.SafeIdle == nil || gpu.SafeIdle == nil {
		return nil, barrier
	}
	safe := barrier
	for _, status := range []control.AdmissionStatus{cpu, gpu} {
		safe = safe && *status.SafeIdle && !status.Accepting && status.ReservedJobs == 0 && status.ActiveEncodes == 0 && status.SuspendedEncodes == 0 && status.RunningJobs == 0 && status.PendingJobs == 0 && uint64(status.ActiveJobs) == uint64(status.ParkedJobs)
	}
	return &safe, barrier
}
