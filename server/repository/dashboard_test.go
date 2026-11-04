package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

func seedDashboard(t *testing.T, repo Repository, now time.Time) {
	t.Helper()
	ctx := context.Background()
	tests := []struct {
		state          model.NotificationStatus
		phase          model.NotificationType
		source, target int64
		age            time.Duration
		worker         string
	}{
		{model.CompletedNotificationStatus, model.JobNotification, 100, 50, time.Hour, "gpu"},
		{model.CompletedNotificationStatus, model.JobNotification, 900, 180, 2 * time.Hour, "cpu"},
		{model.CompletedNotificationStatus, model.JobNotification, 1000, 500, 48 * time.Hour, "gpu"},
		{model.CompletedNotificationStatus, model.FFMPEGSNotification, 3000, 100, time.Minute, "gpu"},
		{model.FailedNotificationStatus, model.JobNotification, 5000, 100, 3 * time.Hour, "cpu"},
		{model.QueuedNotificationStatus, model.JobNotification, 2000, 0, time.Minute, ""},
		{model.CanceledNotificationStatus, model.JobNotification, 2000, 0, time.Minute, "cpu"},
	}
	for _, tt := range tests {
		j := &model.Job{Id: uuid.New(), SourcePath: uuid.NewString() + ".mkv", SourceSize: tt.source, TargetSize: tt.target,
			TargetProbe: &model.MediaProbe{Streams: []model.MediaStream{{CodecType: "audio", CodecName: "aac"}, {CodecType: "video", CodecName: "av1"}}}}
		if err := repo.AddJob(ctx, j); err != nil {
			t.Fatal(err)
		}
		start := &model.TaskEventType{Event: model.Event{EventTime: now.Add(-tt.age - time.Hour), WorkerName: tt.worker}, JobId: j.Id, EventID: 0, NotificationType: model.JobNotification, Status: model.StartedNotificationStatus}
		if err := repo.AddNewTaskEvent(ctx, start); err != nil {
			t.Fatal(err)
		}
		last := &model.TaskEventType{Event: model.Event{EventTime: now.Add(-tt.age), WorkerName: tt.worker}, JobId: j.Id, EventID: 1, NotificationType: tt.phase, Status: tt.state}
		if err := repo.AddNewTaskEvent(ctx, last); err != nil {
			t.Fatal(err)
		}
		if sqlRepo, ok := repo.(*SQLRepository); ok {
			// Production notifications intentionally use server receipt time.
			// Move this fixture's history and projection to the requested period.
			if _, err := sqlRepo.db.ExecContext(ctx, "UPDATE job_events SET event_time=$1 WHERE job_id=$2 AND job_event_id=0", start.EventTime, j.Id); err != nil {
				t.Fatal(err)
			}
			if _, err := sqlRepo.db.ExecContext(ctx, "UPDATE job_events SET event_time=$1 WHERE job_id=$2 AND job_event_id=1", last.EventTime, j.Id); err != nil {
				t.Fatal(err)
			}
			if _, err := sqlRepo.db.ExecContext(ctx, "UPDATE job_status SET event_time=$1 WHERE job_id=$2", last.EventTime, j.Id); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"gpu", "cpu"} {
		if err := repo.PingServerUpdate(ctx, "127.0.0.1", model.PingEventType{Event: model.Event{WorkerName: name}, Runtime: &model.WorkerRuntime{Kind: name, Encoder: "av1_nvenc", MaxActiveJobs: 2, MaxEncodes: 2}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "gpu", 4); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetWorkerMaxActiveJobs(ctx, "offline-only-settings", 0); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardMeasuredOutcomes(t *testing.T) {
	repo := NewInMemoryRepository()
	now := time.Now().UTC()
	seedDashboard(t, repo, now)
	d, err := repo.GetDashboard(t.Context(), 24, now)
	if err != nil {
		t.Fatal(err)
	}
	s := d.Summary
	if s.Total != 7 || s.Completed != 3 || s.Active != 1 || s.Queued != 1 || s.Failed != 1 || s.Canceled != 1 {
		t.Fatalf("summary = %+v, want 7 total, 3 completed, 1 each active/queued/failed/canceled", s)
	}
	if s.SourceBytes != 2000 || s.TargetBytes != 730 || math.Abs(s.CompressionPercent-63.5) > .001 {
		t.Errorf("compression = %d→%d / %f%%, want 2000→730 / 63.5%%", s.SourceBytes, s.TargetBytes, s.CompressionPercent)
	}
	if s.CompletedInRange != 2 || s.FailedInRange != 1 || s.AverageSeconds != 3600 {
		t.Errorf("period results = %+v, want 2 completed, 1 failed, mean 3600 s", s)
	}
	var total int64
	for _, b := range d.Timeline {
		total += b.Completed
	}
	if total != 2 || len(d.Timeline) != 25 {
		t.Errorf("timeline = %d outcomes, %d buckets; want 2,25", total, len(d.Timeline))
	}
	if len(d.Codecs) != 1 || d.Codecs[0].Name != "av1" || d.Codecs[0].SourceBytes != 1000 || d.Codecs[0].TargetBytes != 230 {
		t.Errorf("codecs = %+v, want av1 1000→230 in period", d.Codecs)
	}
	if len(d.Workers) != 3 {
		t.Fatalf("workers = %d, want 3 including offline settings-only worker", len(d.Workers))
	}
	if !d.Workers[0].Online || d.Workers[2].Online {
		t.Errorf("worker freshness = %+v", d.Workers)
	}
	if d.Jobs[0].State != "active" {
		t.Errorf("first job state = %s, want active even after phase completed", d.Jobs[0].State)
	}
}

func TestDashboardEmptyAndProgress(t *testing.T) {
	repo := NewInMemoryRepository()
	now := time.Now().UTC()
	d, err := repo.GetDashboard(t.Context(), 168, now)
	if err != nil || d.Summary.Total != 0 || d.Summary.CompressionPercent != 0 || d.Jobs == nil || d.Workers == nil {
		t.Fatalf("empty dashboard = %+v, %v", d, err)
	}
	if _, err := repo.GetDashboard(t.Context(), 1, now); err == nil {
		t.Fatal("GetDashboard() accepted unsupported period")
	}
	j := &model.Job{Id: uuid.New(), SourcePath: "active.mkv"}
	_ = repo.AddJob(t.Context(), j)
	_ = repo.AddNewTaskEvent(t.Context(), &model.TaskEventType{Event: model.Event{EventTime: now, WorkerName: "gpu"}, JobId: j.Id, NotificationType: model.FFMPEGSNotification, Status: model.StartedNotificationStatus})
	_ = repo.ProgressJob(t.Context(), &model.TaskProgressType{Event: model.Event{EventTime: now.Add(-2 * time.Minute), WorkerName: "gpu"}, JobId: j.Id, ProgressID: "encode", NotificationType: model.FFMPEGSNotification, Percent: 145, ETA: -time.Second})
	d, err = repo.GetDashboard(t.Context(), 24, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Jobs) != 1 || len(d.Jobs[0].Progress) != 1 {
		t.Fatalf("active progress = %+v", d.Jobs)
	}
	p := d.Jobs[0].Progress[0]
	if p.Percent != 100 || p.ETASeconds != 0 || !p.Stale {
		t.Errorf("normalized stale progress = %+v", p)
	}
	_ = repo.AddNewTaskEvent(t.Context(), &model.TaskEventType{Event: model.Event{EventTime: now, WorkerName: "other-worker"}, JobId: j.Id, EventID: 1, NotificationType: model.FFMPEGSNotification, Status: model.StartedNotificationStatus})
	d, _ = repo.GetDashboard(t.Context(), 24, now)
	if len(d.Jobs[0].Progress) != 0 {
		t.Error("dashboard displayed abandoned assignment progress")
	}
}
