//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

// TestServerWorkerIntegration es un test de integración que:
// 1. Levanta un PostgreSQL real con testcontainers
// 2. Levanta un servidor con un scheduler
// 3. Simula un worker que pide un job
// 4. El worker "encodea" el archivo (simulado)
// 5. Verifica que el servidor registre el job como completado
func TestServerWorkerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Levantar PostgreSQL con testcontainers
	t.Log("🐳 Starting PostgreSQL container...")
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("transcoderd_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start postgres container: %v", err)
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate postgres container: %v", err)
		}
	}()

	// Obtener el puerto mapeado
	mappedPort, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Obtener connection string para logging
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	t.Logf("✅ PostgreSQL started: %s (port: %d)", connStr, mappedPort.Int())

	// 2. Configurar el repositorio real con PostgreSQL
	dbConfig := &repository.SQLServerConfig{
		Host:     "localhost",
		Port:     mappedPort.Int(),
		User:     "test",
		Password: "test",
		Scheme:   "transcoderd_test",
		Driver:   "postgres",
	}

	repo, err := repository.NewSQLRepository(dbConfig)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	// Inicializar el schema de la base de datos
	if err := repo.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize repository: %v", err)
	}
	t.Log("✅ Repository initialized")

	// 3. Setup: Crear directorios temporales
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}

	// Crear un archivo de video de prueba
	testVideoPath := filepath.Join(sourceDir, "test-video.mkv")
	if err := createTestVideoFile(testVideoPath, 1024*1024); err != nil {
		t.Fatalf("Failed to create test video: %v", err)
	}
	t.Log("✅ Test files created")

	// 4. Configurar el scheduler
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
	t.Log("✅ Scheduler created")

	testCtx, cancel := context.WithTimeout(ctx, time.Minute*2)
	defer cancel()

	// Test 1: Programar un job
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

		t.Logf("✅ Job scheduled: %s", result.ScheduledJobs[0].Id)
	})

	// Test 2: Worker solicita un job
	t.Run("Request Job", func(t *testing.T) {
		workerName := "test-worker-1"

		jobResponse, err := sched.RequestJob(testCtx, workerName)
		if err != nil {
			t.Fatalf("Failed to request job: %v", err)
		}

		if jobResponse == nil {
			t.Fatal("No job was assigned")
		}

		t.Logf("✅ Job assigned to worker: %s (EventID: %d)", jobResponse.Id, jobResponse.EventID)

		// Verificar que el job está en estado Assigned
		job, err := repo.GetJob(testCtx, jobResponse.Id.String())
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		status := job.Events.GetStatus()
		if status != model.AssignedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.AssignedNotificationStatus, status)
		}
	})

	// Test 3: Worker reporta inicio del job
	t.Run("Worker Starts Job", func(t *testing.T) {
		jobs, _ := repo.GetJobsByStatus(testCtx, model.JobNotification, model.AssignedNotificationStatus)
		if len(jobs) == 0 {
			t.Fatal("No assigned jobs found")
		}

		job := jobs[0]
		workerName := "test-worker-1"

		// Simular evento de inicio
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

		err := sched.HandleWorkerEvent(testCtx, envelope)
		if err != nil {
			t.Fatalf("Failed to handle start event: %v", err)
		}

		// Verificar estado
		job, _ = repo.GetJob(testCtx, job.Id.String())
		status := job.Events.GetStatus()
		if status != model.StartedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.StartedNotificationStatus, status)
		}

		t.Logf("✅ Job started by worker")
	})

	// Test 4: Simular encoding (download, encode, upload)
	t.Run("Simulate Encoding Process", func(t *testing.T) {
		jobs, _ := repo.GetJobsByStatus(testCtx, model.JobNotification, model.StartedNotificationStatus)
		if len(jobs) == 0 {
			t.Fatal("No started jobs found")
		}

		job := jobs[0]
		workerName := "test-worker-1"

		// Simular progreso de download
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

				err := sched.HandleWorkerEvent(testCtx, envelope)
				if err != nil {
					t.Fatalf("Failed to handle progress event: %v", err)
				}
			}
			t.Logf("✅ Download progress simulated")
		})

		// Crear archivo de salida simulado
		targetPath := filepath.Join(tmpDir, job.TargetPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			t.Fatalf("Failed to create target dir: %v", err)
		}
		if err := createTestVideoFile(targetPath, 512*1024); err != nil {
			t.Fatalf("Failed to create encoded file: %v", err)
		}

		// Reportar completado de download
		downloadCompleteEvent := &model.TaskEventType{
			Event: model.Event{
				EventTime:  time.Now(),
				WorkerName: workerName,
			},
			JobId:            job.Id,
			EventID:          job.Events.GetLatest().EventID + 1,
			NotificationType: model.DownloadNotification,
			Status:           model.CompletedNotificationStatus,
			Message:          "Download completed",
		}

		envelope := &model.EnvelopEvent{
			EventType:  model.NotificationEvent,
			EventData:  mustMarshal(downloadCompleteEvent),
			RemoteAddr: "127.0.0.1:12345",
		}

		err := sched.HandleWorkerEvent(testCtx, envelope)
		if err != nil {
			t.Fatalf("Failed to handle download complete: %v", err)
		}

		t.Logf("✅ Download completed")

		// Simular encoding
		ffmpegCompleteEvent := &model.TaskEventType{
			Event: model.Event{
				EventTime:  time.Now(),
				WorkerName: workerName,
			},
			JobId:            job.Id,
			EventID:          job.Events.GetLatest().EventID + 1,
			NotificationType: model.FFMPEGSNotification,
			Status:           model.CompletedNotificationStatus,
			Message:          "Encoding completed",
		}

		envelope = &model.EnvelopEvent{
			EventType:  model.NotificationEvent,
			EventData:  mustMarshal(ffmpegCompleteEvent),
			RemoteAddr: "127.0.0.1:12345",
		}

		err = sched.HandleWorkerEvent(testCtx, envelope)
		if err != nil {
			t.Fatalf("Failed to handle ffmpeg complete: %v", err)
		}

		t.Logf("✅ Encoding completed")

		// Simular upload
		uploadCompleteEvent := &model.TaskEventType{
			Event: model.Event{
				EventTime:  time.Now(),
				WorkerName: workerName,
			},
			JobId:            job.Id,
			EventID:          job.Events.GetLatest().EventID + 1,
			NotificationType: model.UploadNotification,
			Status:           model.CompletedNotificationStatus,
			Message:          "Upload completed",
		}

		envelope = &model.EnvelopEvent{
			EventType:  model.NotificationEvent,
			EventData:  mustMarshal(uploadCompleteEvent),
			RemoteAddr: "127.0.0.1:12345",
		}

		err = sched.HandleWorkerEvent(testCtx, envelope)
		if err != nil {
			t.Fatalf("Failed to handle upload complete: %v", err)
		}

		t.Logf("✅ Upload completed")
	})

	// Test 5: Completar el job
	t.Run("Complete Job", func(t *testing.T) {
		jobs, _ := repo.GetJobsByStatus(testCtx, model.JobNotification, model.StartedNotificationStatus)
		if len(jobs) == 0 {
			t.Fatal("No started jobs found")
		}

		job := jobs[0]
		workerName := "test-worker-1"

		// Reportar job completado
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

		err := sched.HandleWorkerEvent(testCtx, envelope)
		if err != nil {
			t.Fatalf("Failed to handle complete event: %v", err)
		}

		// Verificar estado final
		job, err = repo.GetJob(testCtx, job.Id.String())
		if err != nil {
			t.Fatalf("Failed to get final job: %v", err)
		}

		status := job.Events.GetStatus()
		if status != model.CompletedNotificationStatus {
			t.Errorf("Expected status %s, got %s", model.CompletedNotificationStatus, status)
		}

		// Verificar que se actualizó el tamaño del target
		if job.TargetSize == 0 {
			t.Error("Target size should be updated")
		}

		// Verificar que el archivo existe
		targetPath := filepath.Join(tmpDir, job.TargetPath)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			t.Error("Target file should exist")
		}

		t.Logf("✅ Job completed successfully!")
		t.Logf("   Source: %s (%d bytes)", job.SourcePath, job.SourceSize)
		t.Logf("   Target: %s (%d bytes)", job.TargetPath, job.TargetSize)
		if job.SourceSize > 0 {
			t.Logf("   Compression: %.2f%%", (1.0-float64(job.TargetSize)/float64(job.SourceSize))*100)
		}
	})

	// Test 6: Verificar historial de eventos
	t.Run("Verify Event History", func(t *testing.T) {
		jobs, _ := repo.GetJobsByStatus(testCtx, model.JobNotification, model.CompletedNotificationStatus)
		if len(jobs) == 0 {
			t.Fatal("No completed jobs found")
		}

		job := jobs[0]

		expectedEvents := []model.NotificationType{
			model.JobNotification,      // Queued
			model.JobNotification,      // Assigned
			model.JobNotification,      // Started
			model.DownloadNotification, // Download complete
			model.FFMPEGSNotification,  // Encoding complete
			model.UploadNotification,   // Upload complete
			model.JobNotification,      // Job complete
		}

		if len(job.Events) < len(expectedEvents) {
			t.Errorf("Expected at least %d events, got %d", len(expectedEvents), len(job.Events))
		}

		t.Logf("✅ Event history verified (%d events)", len(job.Events))
		for i, event := range job.Events {
			t.Logf("   %d. %s - %s: %s", i+1, event.NotificationType, event.Status, event.Message)
		}
	})
}

// Helper functions

func createTestVideoFile(path string, size int64) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Escribir datos dummy
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
