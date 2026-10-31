package desktop

import (
	"transcoder/model"
	"transcoder/update/managed"
	"transcoder/worker/control"
)

type Request struct {
	JobID    string    `json:"jobId,omitempty"`
	ID       int64     `json:"id,omitempty"`
	Command  string    `json:"command"`
	Settings *Settings `json:"settings,omitempty"`
	MaxJobs  int       `json:"maxJobs,omitempty"`
	CPUJobs  *int      `json:"cpuJobs,omitempty"`
	GPUJobs  *int      `json:"gpuJobs,omitempty"`
}

type Status struct {
	Configured bool                               `json:"configured"`
	Running    bool                               `json:"running"`
	Version    string                             `json:"version"`
	Pending    bool                               `json:"pending"`
	Settings   *PublicSettings                    `json:"settings,omitempty"`
	Worker     *control.AdmissionStatus           `json:"worker,omitempty"`
	Profiles   map[string]control.AdmissionStatus `json:"profiles,omitempty"`
	Queue      *model.QueueSnapshot               `json:"queue,omitempty"`
	QueueError string                             `json:"queueError,omitempty"`
	Message    string                             `json:"message,omitempty"`
	Update     *managed.Status                    `json:"update,omitempty"`
}

type Response struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}
