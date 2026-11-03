package model

import "time"

// QueueSnapshot exposes only names, not server filesystem paths.
type QueueSnapshot struct {
	Total int64       `json:"total"`
	Jobs  []QueuedJob `json:"jobs"`
}
type QueuedJob struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	QueuedAt time.Time `json:"queuedAt"`
}
