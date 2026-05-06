//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

func TestServerWorkerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pgHost := os.Getenv("INTEGRATION_PG_HOST")
	pgPortStr := os.Getenv("INTEGRATION_PG_PORT")
	pgUser := os.Getenv("INTEGRATION_PG_USER")
	pgPass := os.Getenv("INTEGRATION_PG_PASSWORD")
	pgDB := os.Getenv("INTEGRATION_PG_DATABASE")
	if pgHost == "" || pgPortStr == "" || pgUser == "" || pgPass == "" || pgDB == "" {
		t.Skip("Integration env vars not set (INTEGRATION_PG_HOST, INTEGRATION_PG_PORT, INTEGRATION_PG_USER, INTEGRATION_PG_PASSWORD, INTEGRATION_PG_DATABASE)")
	}

	pgPort, err := strconv.Atoi(pgPortStr)
	if err != nil {
		t.Fatalf("Invalid INTEGRATION_PG_PORT: %v", err)
	}

	ctx := context.Background()

	dbConfig := &repository.SQLServerConfig{
		Host:     pgHost,
		Port:     pgPort,
		User:     pgUser,
		Password: pgPass,
		Scheme:   pgDB,
		Driver:   "postgres",
	}

	repo, err := repository.NewSQLRepository(dbConfig)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	if err := repo.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize repository: %v", err)
	}

	tmpDir := t.TempDir()

	testVideoPath := filepath.Join(tmpDir, "test-video.mkv")
	if err := createTestVideoFile(testVideoPath, 1024*1024); err != nil {
		t.Fatalf("Failed to create test video: %v", err)
	}

	schedulerConfig := &scheduler.Config{
		ScheduleTime:           time.Second * 5,
		JobTimeout:             time.Minute * 5,
		SourcePath:             tmpDir,
		DeleteSourceOnComplete: false,
		MinFileSize:            100,
	}

	sched, err := scheduler.NewScheduler(schedulerConfig, repo)
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	testCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	var jobID string
	workerName := "test-worker-1"

	t.Run("Schedule Job", func(t *testing.T) {
		jobRequest := &model.JobRequest{
			SourcePath: "test-video.mkv",
			TargetPath: "test-video_encoded.mkv",
			SourceSize: 1024 * 1024,
		}

		result, err := sched.ScheduleJobRequests(testCtx, jobRequest)
		if err != nil {
			t.Fatalf("Failed to schedule job: %v", err)
		}

		if len(result.ScheduledJobs) != 1 {
			t.Fatalf("Expected 1 scheduled job, got %d", len(result.ScheduledJobs))
		}

		if len(result.FailedJobRequest) > 0 {
			t.Errorf("Unexpected failed requests: %v", result.FailedJobRequest)
		}

		jobID = result.ScheduledJobs[0].Id.String()
		t.Logf("Job scheduled: %s", jobID)
	})

	t.Run("Request Job", func(t *testing.T) {
		jobResponse, err := sched.RequestJob(testCtx, workerName)
		if err != nil {
			t.Fatalf("Failed to request job: %v", err)
		}

		if jobResponse == nil {
			t.Fatal("No job was assigned")
		}

		t.Logf("Job assigned to worker: %s (EventID: %d)", jobResponse.Id, jobResponse.EventID)

		job, err := repo.GetJob(testCtx, jobResponse.Id.String())
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		status := job.Events.GetStatus()
		if status != model.AssignedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.AssignedNotificationStatus, status)
		}
	})

	t.Run("Worker Starts Job", func(t *testing.T) {
		job, err := repo.GetJob(testCtx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		startEvent := &model.TaskEventType{
			Event: model.Event{
				EventTime:  time.Now(),
				WorkerName: workerName,
			},
			JobId:            job.Id,
			EventID:          job.Events.GetLatest().EventID + 1,
			NotificationType: model.JobNotification,
			Status:           model.StartedNotificationStatus,
			Message:          "Job started",
		}

		envelope := &model.EnvelopEvent{
			EventType:  model.NotificationEvent,
			EventData:  mustMarshal(startEvent),
			RemoteAddr: "127.0.0.1:12345",
		}

		if err := sched.HandleWorkerEvent(testCtx, envelope); err != nil {
			t.Fatalf("Failed to handle start event: %v", err)
		}

		job, _ = repo.GetJob(testCtx, jobID)
		status := job.Events.GetStatus()
		if status != model.StartedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.StartedNotificationStatus, status)
		}
	})

	t.Run("Simulate Encoding Process", func(t *testing.T) {
		job, err := repo.GetJob(testCtx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		t.Run("Download Progress", func(t *testing.T) {
			for i := 0; i <= 100; i += 25 {
				progressEvent := &model.TaskProgressType{
					Event: model.Event{
						EventTime:  time.Now(),
						WorkerName: workerName,
					},
					JobId:            job.Id,
					ProgressID:       fmt.Sprintf("download-%s", job.Id.String()),
					Percent:          float64(i),
					NotificationType: model.DownloadNotification,
					Status:           model.ProgressingTaskProgressTypeStatus,
				}

				envelope := &model.EnvelopEvent{
					EventType:  model.ProgressEvent,
					EventData:  mustMarshal(progressEvent),
					RemoteAddr: "127.0.0.1:12345",
				}

				if err := sched.HandleWorkerEvent(testCtx, envelope); err != nil {
					t.Fatalf("Failed to handle progress event: %v", err)
				}
			}
		})

		targetPath := filepath.Join(tmpDir, job.TargetPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			t.Fatalf("Failed to create target dir: %v", err)
		}
		if err := createTestVideoFile(targetPath, 512*1024); err != nil {
			t.Fatalf("Failed to create encoded file: %v", err)
		}

		steps := []struct {
			notifType model.NotificationType
			message   string
		}{
			{model.DownloadNotification, "Download completed"},
			{model.FFMPEGSNotification, "Encoding completed"},
			{model.UploadNotification, "Upload completed"},
		}

		for _, step := range steps {
			event := &model.TaskEventType{
				Event: model.Event{
					EventTime:  time.Now(),
					WorkerName: workerName,
				},
				JobId:            job.Id,
				EventID:          job.Events.GetLatest().EventID + 1,
				NotificationType: step.notifType,
				Status:           model.CompletedNotificationStatus,
				Message:          step.message,
			}

			envelope := &model.EnvelopEvent{
				EventType:  model.NotificationEvent,
				EventData:  mustMarshal(event),
				RemoteAddr: "127.0.0.1:12345",
			}

			if err := sched.HandleWorkerEvent(testCtx, envelope); err != nil {
				t.Fatalf("Failed to handle %s event: %v", step.notifType, err)
			}

			job, _ = repo.GetJob(testCtx, job.Id.String())
		}
	})

	t.Run("Complete Job", func(t *testing.T) {
		job, err := repo.GetJob(testCtx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		completeEvent := &model.TaskEventType{
			Event: model.Event{
				EventTime:  time.Now(),
				WorkerName: workerName,
			},
			JobId:            job.Id,
			EventID:          job.Events.GetLatest().EventID + 1,
			NotificationType: model.JobNotification,
			Status:           model.CompletedNotificationStatus,
			Message:          "Job completed successfully",
		}

		envelope := &model.EnvelopEvent{
			EventType:  model.NotificationEvent,
			EventData:  mustMarshal(completeEvent),
			RemoteAddr: "127.0.0.1:12345",
		}

		if err := sched.HandleWorkerEvent(testCtx, envelope); err != nil {
			t.Fatalf("Failed to handle complete event: %v", err)
		}

		job, err = repo.GetJob(testCtx, jobID)
		if err != nil {
			t.Fatalf("Failed to get final job: %v", err)
		}

		status := job.Events.GetStatus()
		if status != model.CompletedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.CompletedNotificationStatus, status)
		}

		if job.TargetSize == 0 {
			t.Error("Target size should be updated")
		}

		targetPath := filepath.Join(tmpDir, job.TargetPath)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			t.Error("Target file should exist")
		}

		t.Logf("Source: %s (%d bytes)", job.SourcePath, job.SourceSize)
		t.Logf("Target: %s (%d bytes)", job.TargetPath, job.TargetSize)
		if job.SourceSize > 0 {
			t.Logf("Compression: %.2f%%", (1.0-float64(job.TargetSize)/float64(job.SourceSize))*100)
		}
	})

	t.Run("Verify Event History", func(t *testing.T) {
		job, err := repo.GetJob(testCtx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		expectedMinEvents := 7
		if len(job.Events) < expectedMinEvents {
			t.Errorf("Expected at least %d events, got %d", expectedMinEvents, len(job.Events))
		}

		t.Logf("Event history (%d events):", len(job.Events))
		for i, event := range job.Events {
			t.Logf("  %d. %s - %s: %s", i+1, event.NotificationType, event.Status, event.Message)
		}
	})
}

func createTestVideoFile(path string, size int64) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	written := int64(0)
	buffer := make([]byte, 4096)
	for written < size {
		toWrite := size - written
		if toWrite > int64(len(buffer)) {
			toWrite = int64(len(buffer))
		}
		n, err := file.Write(buffer[:toWrite])
		if err != nil {
			return err
		}
		written += int64(n)
	}

	return nil
}

func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
