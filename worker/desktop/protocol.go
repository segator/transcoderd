package desktop

import (
	"transcoder/model"
	"transcoder/worker/control"
)

type Request struct {
	Command  string    `json:"command"`
	Settings *Settings `json:"settings,omitempty"`
	MaxJobs  int       `json:"maxJobs,omitempty"`
}

type Status struct {
	Configured bool                     `json:"configured"`
	Running    bool                     `json:"running"`
	Pending    bool                     `json:"pending"`
	Settings   *PublicSettings          `json:"settings,omitempty"`
	Worker     *control.AdmissionStatus `json:"worker,omitempty"`
	Queue      *model.QueueSnapshot     `json:"queue,omitempty"`
	QueueError string                   `json:"queueError,omitempty"`
	Message    string                   `json:"message,omitempty"`
}

type Response struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}
