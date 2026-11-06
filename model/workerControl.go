package model

import "fmt"

const RemoteControlCapability = "2"

// WorkerControl is a durable per-worker command; limits are its queue-resume snapshot.
type WorkerControl struct {
	ID            string `json:"id"`
	JobID         string `json:"jobId,omitempty"`
	Action        string `json:"action"`
	State         string `json:"state"`
	Error         string `json:"error,omitempty"`
	CPUJobs       *int   `json:"cpuJobs,omitempty"`
	GPUJobs       *int   `json:"gpuJobs,omitempty"`
	MaxActiveJobs *int   `json:"maxActiveJobs,omitempty"`
}

func ValidateWorkerAction(action string) error {
	switch action {
	case "pause-encodes", "resume-encodes", "stop-after-current", "resume-queue", "pause-job", "resume-job":
		return nil
	}
	return fmt.Errorf("unknown worker control action")
}

func (c WorkerControl) Settings() WorkerSettings {
	return WorkerSettings{CPUJobs: c.CPUJobs, GPUJobs: c.GPUJobs, MaxActiveJobs: c.MaxActiveJobs}.Clone()
}
func (c WorkerControl) Clone() WorkerControl {
	s := c.Settings()
	c.CPUJobs, c.GPUJobs, c.MaxActiveJobs = s.CPUJobs, s.GPUJobs, s.MaxActiveJobs
	return c
}

type WorkerControlAck struct {
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
}
