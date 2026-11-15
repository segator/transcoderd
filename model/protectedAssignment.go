package model

import "github.com/google/uuid"

// ProtectedAssignment identifies one generation of a job assignment. The
// capability is confidential and must not be written to logs or telemetry.
type ProtectedAssignment struct {
	RequestJobResponse
	Generation uint64 `json:"generation"`
	KeyID      string `json:"keyId"`
	Capability string `json:"capability"`
}

type AssignmentClaim struct {
	JobID      uuid.UUID `json:"jobId"`
	Generation uint64    `json:"generation"`
	Capability string    `json:"capability"`
	InstanceID uuid.UUID `json:"instanceId"`
}

type ProtectedPublication struct {
	JobID          uuid.UUID
	Generation     uint64
	WorkerName     string
	StagingPath    string
	Checksum       string
	SizeBytes      int64
	EventID        int
	Message        string
	State          string
	FinalPath      string
	DeleteSource   bool
	CleanupPending bool
}
