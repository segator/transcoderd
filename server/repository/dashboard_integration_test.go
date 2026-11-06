//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

func TestDashboardPostgres(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("DASHBOARD_TEST_POSTGRES is not set")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "dashboard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	db, err := sql.Open("postgres", dsn+" search_path="+schema+" TimeZone=Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &SQLRepository{db: &SQLDatabase{db: db}}
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	// Re-running migrations must preserve an already initialized database.
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	// Library cache persists across repository instances and honors fingerprints.
	probe := &model.MediaProbe{DurationSeconds: 123, Streams: []model.MediaStream{{CodecType: "video", CodecName: "hevc", Width: 1920}}}
	rootKey := strings.Repeat("a", 64)
	if err := repo.PutMediaProbe(ctx, rootKey, "movie.mkv", 1234, 5678, probe); err != nil {
		t.Fatal(err)
	}
	cacheReader := &SQLRepository{db: &SQLDatabase{db: db}}
	cached, err := cacheReader.GetMediaProbe(ctx, rootKey, "movie.mkv", 1234, 5678)
	if err != nil || !reflect.DeepEqual(cached, probe) {
		t.Fatalf("durable media cache = %+v, %v", cached, err)
	}
	for _, fingerprint := range [][2]int64{{1235, 5678}, {1234, 5679}} {
		got, err := repo.GetMediaProbe(ctx, rootKey, "movie.mkv", fingerprint[0], fingerprint[1])
		if err != nil || got != nil {
			t.Fatalf("stale cache = %+v, %v", got, err)
		}
	}
	// Intake transaction errors must rollback both the job and its events.
	rollbackJob := &model.Job{Id: uuid.New(), SourcePath: "rollback.mkv", TargetPath: "rollback_encoded.mkv"}
	callbackErr := errors.New("reject intake")
	if err := repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		if err := tx.AddJob(ctx, rollbackJob); err != nil {
			return err
		}
		if err := tx.AddNewTaskEvent(ctx, rollbackJob.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
			return err
		}
		return callbackErr
	}); !errors.Is(err, callbackErr) {
		t.Fatalf("transaction error = %v", err)
	}
	if _, err := repo.GetJob(ctx, rollbackJob.Id.String()); !errors.Is(err, ErrElementNotFound) {
		t.Fatalf("rolled-back job = %v", err)
	}

	// Deferred PostgreSQL constraints surface commit failures, not success.
	if _, err := db.ExecContext(ctx, "CREATE TABLE media_commit_check (n integer UNIQUE DEFERRABLE INITIALLY DEFERRED)"); err != nil {
		t.Fatal(err)
	}
	if err := repo.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		connection, err := tx.(*SQLRepository).getConnection()
		if err != nil {
			return err
		}
		_, err = connection.ExecContext(ctx, "INSERT INTO media_commit_check VALUES(1),(1)")
		return err
	}); err == nil {
		t.Fatal("commit failure was swallowed")
	}
	now := time.Now().UTC()
	empty, err := repo.GetDashboard(ctx, 24, now)
	if err != nil || empty.Summary.Total != 0 {
		t.Fatalf("empty SQL dashboard = %+v, %v", empty, err)
	}
	seedDashboard(t, repo, now)
	memory := NewInMemoryRepository()
	seedDashboard(t, memory, now)

	jobs, err := repo.GetJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, j := range *jobs {
		paths = append(paths, j.SourcePath)
		if j.TargetPath != "" {
			paths = append(paths, j.TargetPath)
		}
	}
	states, err := repo.GetMediaJobStates(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	stateMemory := NewInMemoryRepository()
	for _, row := range *jobs {
		job, err := repo.GetJob(ctx, row.Id.String())
		if err != nil {
			t.Fatal(err)
		}
		if err := stateMemory.AddJob(ctx, job); err != nil {
			t.Fatal(err)
		}
		for _, event := range job.Events {
			if err := stateMemory.AddNewTaskEvent(ctx, event); err != nil {
				t.Fatal(err)
			}
		}
	}
	wantStates, err := stateMemory.GetMediaJobStates(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(states, wantStates) {
		t.Errorf("SQL media associations = %+v, want %+v", states, wantStates)
	}
	for _, hours := range []int{24, 168, 720} {
		d, err := repo.GetDashboard(ctx, hours, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := memory.GetDashboard(ctx, hours, now.Add(time.Second))
		if !reflect.DeepEqual(d.Summary, want.Summary) {
			t.Errorf("SQL summary = %+v, want %+v", d.Summary, want.Summary)
		}
		if !reflect.DeepEqual(d.Timeline, want.Timeline) {
			t.Errorf("SQL timeline = %+v, want %+v", d.Timeline, want.Timeline)
		}
		if !reflect.DeepEqual(d.Codecs, want.Codecs) {
			t.Errorf("SQL codecs = %+v, want %+v", d.Codecs, want.Codecs)
		}
		if len(d.Workers) != 2 || d.Workers[0].DesiredMax == nil || *d.Workers[0].DesiredMax != 4 || d.Workers[0].Runtime == nil {
			t.Errorf("SQL workers = %+v, want telemetry and desired per-name limit", d.Workers)
		}
		for _, worker := range d.Workers {
			if worker.Runtime == nil || worker.Runtime.Version != "v2.13.2" {
				t.Fatalf("SQL worker version lost: %+v", worker.Runtime)
			}
		}
		if len(d.Jobs) != 7 || d.Jobs[0].State != "active" {
			t.Errorf("SQL jobs = %+v", d.Jobs)
		}
		if len(d.Activity) == 0 {
			t.Error("SQL activity is empty")
		}
		if _, err := json.Marshal(d); err != nil {
			t.Errorf("dashboard JSON: %v", err)
		}
	}
	// Existing worker lookups must still work after adding the telemetry column.
	if w, err := repo.GetWorker(ctx, "gpu"); err != nil || w.Name != "gpu" {
		t.Fatalf("GetWorker() after migration = %+v, %v", w, err)
	}
	if err := repo.PingServerUpdate(ctx, "127.0.0.2", model.PingEventType{Event: model.Event{WorkerName: "gpu"}}); err != nil {
		t.Fatal(err)
	}
	d, err := repo.GetDashboard(ctx, 24, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if d.Workers[0].Runtime != nil || !d.Workers[0].Online {
		t.Error("legacy heartbeat did not clear unavailable telemetry while updating presence")
	}
	// The durable desired capacity survives a fresh repository object.
	fresh := &SQLRepository{db: &SQLDatabase{db: db}}
	max, err := fresh.GetWorkerMaxActiveJobs(ctx, "gpu")
	if err != nil || max == nil || *max != 4 {
		t.Errorf("fresh desired limit = %v, %v; want 4", max, err)
	}
	cpuSlots, gpuSlots := 2, 3
	if err := repo.SetWorkerSettings(ctx, "gpu", model.WorkerSettings{CPUJobs: &cpuSlots, GPUJobs: &gpuSlots}); err != nil {
		t.Fatal(err)
	}
	pair, err := fresh.GetWorkerSettings(ctx, "gpu")
	if err != nil || !pair.Independent() || *pair.CPUJobs != 2 || *pair.GPUJobs != 3 || *pair.MaxActiveJobs != 5 {
		t.Fatalf("fresh SQL pair = %+v, %v", pair, err)
	}
	pairedDashboard, err := repo.GetDashboard(ctx, 24, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if pairedDashboard.Workers[0].DesiredCPUJobs == nil || *pairedDashboard.Workers[0].DesiredCPUJobs != 2 || *pairedDashboard.Workers[0].DesiredGPUJobs != 3 {
		t.Errorf("dashboard pair = %+v", pairedDashboard.Workers[0])
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "gpu", 4); err != nil {
		t.Fatal(err)
	}
	pair, err = fresh.GetWorkerSettings(ctx, "gpu")
	if err != nil || pair.Independent() || pair.CPUJobs != nil || pair.GPUJobs != nil {
		t.Errorf("SQL legacy reset = %+v, %v", pair, err)
	}
	// These tables use naive timestamp columns. New writes must be UTC even
	// when this process (or the database session) uses another timezone.
	job := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv"}
	if err := repo.AddJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Second)
	if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: job.Id, EventID: 0, Event: model.Event{WorkerName: "gpu"}, NotificationType: model.JobNotification, Status: model.StartedNotificationStatus}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetJob(ctx, job.Id.String())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastUpdate.Before(before) || loaded.LastUpdate.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("new event time = %v, want current UTC instant", loaded.LastUpdate)
	}
	if err := repo.AddNewTaskEvent(ctx, &model.TaskEventType{JobId: job.Id, EventID: 1, Event: model.Event{WorkerName: "gpu"}, NotificationType: model.FFMPEGSNotification, Status: model.StartedNotificationStatus}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ProgressJob(ctx, &model.TaskProgressType{JobId: job.Id, ProgressID: "phase", NotificationType: model.FFMPEGSNotification, Event: model.Event{WorkerName: "gpu"}, Percent: 25, ETA: time.Minute}); err != nil {
		t.Fatal(err)
	}
	progress, err := repo.GetAllProgressJobs(ctx)
	if err != nil || len(progress) != 1 || progress[0].EventTime.Before(before) || progress[0].EventTime.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("new progress = %+v, %v; want UTC instant", progress, err)
	}
	d, err = repo.GetDashboard(ctx, 24, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if d.Jobs[0].ID != job.Id.String() || len(d.Jobs[0].Progress) != 1 || d.Jobs[0].Progress[0].Percent != 25 || d.Jobs[0].Progress[0].Stale {
		t.Errorf("SQL current progress = %+v", d.Jobs[0])
	}
	// Pause observations persist in JSON and survive a fresh SQL repository reader.
	runtime := &model.WorkerRuntime{CapacityVersion: 2, Paused: true, RAMPaused: true, SuspendedEncodes: 1, ManagedProfile: true, Encoder: "av1_nvenc", Jobs: []model.WorkerJobRuntime{{ID: job.Id.String(), Phase: "FFMPEG", State: "paused", IndividuallyPaused: true}}}
	if err := repo.PingServerUpdate(ctx, "127.0.0.3", model.PingEventType{Event: model.Event{WorkerName: "gpu"}, Runtime: runtime}); err != nil {
		t.Fatal(err)
	}
	observed, err := fresh.GetWorker(ctx, "gpu")
	if err != nil || !reflect.DeepEqual(observed.Runtime, runtime) {
		t.Fatalf("SQL persisted pause telemetry = %+v, %v", observed, err)
	}

}

func TestDashboardPostgresPagination(t *testing.T) {
	dsn := os.Getenv("DASHBOARD_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("DASHBOARD_TEST_POSTGRES is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := "paging_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	scoped, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer scoped.Close()
	repo := &SQLRepository{db: &SQLDatabase{db: scoped}}
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkDashboardJobPages(t, repo)
	checkDashboardWorkerRecency(t, repo)
	checkWorkerControlRepository(t, repo)
	checkDashboardCustomRange(t, repo)
}
