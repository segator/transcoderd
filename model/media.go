package model

import "time"

type MediaMetadata struct {
	DurationSeconds float64     `json:"durationSeconds"`
	Width           int         `json:"width"`
	Height          int         `json:"height"`
	Codec           string      `json:"codec"`
	BitrateMbps     float64     `json:"bitrateMbps"`
	Probe           *MediaProbe `json:"-"`
}

type MediaJobState struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Role  string `json:"role"`
}

type MediaRoot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Queueable bool   `json:"queueable"`
	Available bool   `json:"available"`
}

type MediaFile struct {
	ID         string         `json:"id"`
	Root       string         `json:"root"`
	Path       string         `json:"path"`
	Name       string         `json:"name"`
	Directory  bool           `json:"directory"`
	Size       int64          `json:"size"`
	ModifiedAt time.Time      `json:"modifiedAt"`
	ModifiedNS int64          `json:"-"`
	Metadata   *MediaMetadata `json:"metadata,omitempty"`
	Job        *MediaJobState `json:"job,omitempty"`
	Eligible   bool           `json:"eligible"`
	Reason     string         `json:"reason,omitempty"`
}

type MediaFilters struct {
	Root           string   `json:"root"`
	Path           string   `json:"path"`
	Recursive      bool     `json:"recursive"`
	Contains       string   `json:"contains"`
	Exclude        string   `json:"exclude"`
	MinBytes       int64    `json:"minBytes"`
	MaxBytes       int64    `json:"maxBytes"`
	MinWidth       int      `json:"minWidth"`
	MinBitrateMbps float64  `json:"minBitrateMbps"`
	IncludeCodecs  []string `json:"includeCodecs"`
	ExcludeCodecs  []string `json:"excludeCodecs"`
	CandidateLimit int      `json:"candidateLimit"`
	OnlyEligible   bool     `json:"onlyEligible"`
	RetryFailed    bool     `json:"retryFailed"`
	RetryCanceled  bool     `json:"retryCanceled"`
}

type MediaBatchRequest struct {
	ScanID string   `json:"scanId"`
	IDs    []string `json:"ids"`
	Target int      `json:"target"`
}

type MediaLoadResult struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	State   string `json:"state"`
	JobID   string `json:"jobId,omitempty"`
	Message string `json:"message,omitempty"`
}

type MediaOperation struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	State      string            `json:"state"`
	Stage      string            `json:"stage"`
	StartedAt  time.Time         `json:"startedAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
	Filters    MediaFilters      `json:"filters"`
	Discovered int               `json:"discovered"`
	Candidates int               `json:"candidates"`
	Examined   int               `json:"examined"`
	Matched    int               `json:"matched"`
	Target     int               `json:"target"`
	Scheduled  int               `json:"scheduled"`
	Skipped    int               `json:"skipped"`
	Failed     int               `json:"failed"`
	Truncated  bool              `json:"truncated"`
	Error      string            `json:"error,omitempty"`
	Files      []MediaFile       `json:"files"`
	Results    []MediaLoadResult `json:"results"`
}
