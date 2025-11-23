package model

import (
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestNotificationTypes(t *testing.T) {
	tests := []struct {
		name  string
		value NotificationType
		want  string
	}{
		{"Job notification", JobNotification, "Job"},
		{"Download notification", DownloadNotification, "Download"},
		{"Upload notification", UploadNotification, "Upload"},
		{"MKVExtract notification", MKVExtractNotification, "MKVExtract"},
		{"PGS notification", PGSNotification, "PGS"},
		{"FFMPEG notification", FFMPEGSNotification, "FFMPEG"},
		{"JobVerify", JobVerify, "JobVerify"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("value = %s, want %s", tt.value, tt.want)
			}
		})
	}
}

func TestNotificationStatuses(t *testing.T) {
	tests := []struct {
		name  string
		value NotificationStatus
		want  string
	}{
		{"Queued", QueuedNotificationStatus, "queued"},
		{"Assigned", AssignedNotificationStatus, "assigned"},
		{"Started", StartedNotificationStatus, "started"},
		{"Completed", CompletedNotificationStatus, "completed"},
		{"Canceled", CanceledNotificationStatus, "canceled"},
		{"Failed", FailedNotificationStatus, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("value = %s, want %s", tt.value, tt.want)
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	tests := []struct {
		name  string
		value EventType
		want  string
	}{
		{"Ping", PingEvent, "Ping"},
		{"Notification", NotificationEvent, "Notification"},
		{"Progress", ProgressEvent, "Progress"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("value = %s, want %s", tt.value, tt.want)
			}
		})
	}
}

func TestJobSerialization(t *testing.T) {
	now := time.Now()
	job := &Job{
		SourcePath:    "/path/to/source.mp4",
		TargetPath:    "/path/to/target.mp4",
		Id:            uuid.New(),
		Status:        "processing",
		StatusPhase:   DownloadNotification,
		StatusMessage: "Downloading file",
		LastUpdate:    &now,
		SourceSize:    1024000,
		TargetSize:    512000,
	}

	// Test JSON serialization
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("Failed to marshal Job: %v", err)
	}

	// Test JSON deserialization
	var decoded Job
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal Job: %v", err)
	}

	// Verify fields
	if decoded.SourcePath != job.SourcePath {
		t.Errorf("SourcePath = %s, want %s", decoded.SourcePath, job.SourcePath)
	}
	if decoded.Id != job.Id {
		t.Errorf("Id = %v, want %v", decoded.Id, job.Id)
	}
	if decoded.Status != job.Status {
		t.Errorf("Status = %s, want %s", decoded.Status, job.Status)
	}
	if decoded.StatusPhase != job.StatusPhase {
		t.Errorf("StatusPhase = %v, want %v", decoded.StatusPhase, job.StatusPhase)
	}
	if decoded.SourceSize != job.SourceSize {
		t.Errorf("SourceSize = %d, want %d", decoded.SourceSize, job.SourceSize)
	}
}

func TestTaskEventTypeSerialization(t *testing.T) {
	now := time.Now()
	event := &TaskEventType{
		Event: Event{
			EventTime:  now,
			WorkerName: "worker-1",
		},
		JobId:            uuid.New(),
		EventID:          123,
		NotificationType: DownloadNotification,
		Status:           CompletedNotificationStatus,
		Message:          "Download completed successfully",
	}

	// Test JSON serialization
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal TaskEventType: %v", err)
	}

	// Test JSON deserialization
	var decoded TaskEventType
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal TaskEventType: %v", err)
	}

	// Verify fields
	if decoded.WorkerName != event.WorkerName {
		t.Errorf("WorkerName = %s, want %s", decoded.WorkerName, event.WorkerName)
	}
	if decoded.JobId != event.JobId {
		t.Errorf("JobId = %v, want %v", decoded.JobId, event.JobId)
	}
	if decoded.EventID != event.EventID {
		t.Errorf("EventID = %d, want %d", decoded.EventID, event.EventID)
	}
	if decoded.NotificationType != event.NotificationType {
		t.Errorf("NotificationType = %v, want %v", decoded.NotificationType, event.NotificationType)
	}
	if decoded.Status != event.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, event.Status)
	}
	if decoded.Message != event.Message {
		t.Errorf("Message = %s, want %s", decoded.Message, event.Message)
	}
}

func TestRequestJobResponseSerialization(t *testing.T) {
	response := &RequestJobResponse{
		Id:      uuid.New(),
		EventID: 42,
	}

	// Test JSON serialization
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal RequestJobResponse: %v", err)
	}

	// Test JSON deserialization
	var decoded RequestJobResponse
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal RequestJobResponse: %v", err)
	}

	// Verify fields
	if decoded.Id != response.Id {
		t.Errorf("Id = %v, want %v", decoded.Id, response.Id)
	}
	if decoded.EventID != response.EventID {
		t.Errorf("EventID = %d, want %d", decoded.EventID, response.EventID)
	}
}

func TestWorker(t *testing.T) {
	now := time.Now()
	worker := &Worker{
		Name:      "worker-1",
		Ip:        "192.168.1.100",
		QueueName: "queue-1",
		LastSeen:  now,
	}

	if worker.Name != "worker-1" {
		t.Errorf("Name = %s, want %s", worker.Name, "worker-1")
	}
	if worker.Ip != "192.168.1.100" {
		t.Errorf("Ip = %s, want %s", worker.Ip, "192.168.1.100")
	}
	if worker.QueueName != "queue-1" {
		t.Errorf("QueueName = %s, want %s", worker.QueueName, "queue-1")
	}
	if worker.LastSeen != now {
		t.Errorf("LastSeen mismatch")
	}
}

func TestTaskPGS(t *testing.T) {
	pgs := &TaskPGS{
		PGSID:         1,
		PGSSourcePath: "/path/to/source.pgs",
		PGSLanguage:   "eng",
		PGSTargetPath: "/path/to/target.srt",
	}

	if pgs.PGSID != 1 {
		t.Errorf("PGSID = %d, want %d", pgs.PGSID, 1)
	}
	if pgs.PGSLanguage != "eng" {
		t.Errorf("PGSLanguage = %s, want %s", pgs.PGSLanguage, "eng")
	}
}
