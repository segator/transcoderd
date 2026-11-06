package repository

import (
	"context"
	"fmt"
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
		if err := repo.PingServerUpdate(ctx, "127.0.0.1", model.PingEventType{Event: model.Event{WorkerName: name}, Runtime: &model.WorkerRuntime{Version: "v2.13.2", Kind: name, Encoder: "av1_nvenc", MaxActiveJobs: 2, MaxEncodes: 2}}); err != nil {
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
	if len(d.Workers) != 2 {
		t.Fatalf("workers = %d, want 2 recently seen workers", len(d.Workers))
	}
	if !d.Workers[0].Online || !d.Workers[1].Online {
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

func checkDashboardJobPages(t *testing.T, repo Repository) {
	t.Helper()
	for i := 0; i < 130; i++ {
		j := &model.Job{Id: uuid.New(), SourcePath: fmt.Sprintf("queue-%03d.mkv", i)}
		if err := repo.AddJob(t.Context(), j); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddNewTaskEvent(t.Context(), j.AddEvent(model.JobNotification, model.QueuedNotificationStatus)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 28; i++ {
		j := &model.Job{Id: uuid.New(), SourcePath: fmt.Sprintf("Finished-Movie-%03d.mkv", i)}
		if err := repo.AddJob(t.Context(), j); err != nil {
			t.Fatal(err)
		}
		event := j.AddEvent(model.JobNotification, model.CompletedNotificationStatus)
		event.WorkerName = "Test-CPU"
		if err := repo.AddNewTaskEvent(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for _, offset := range []int{0, 25} {
		page, err := repo.GetDashboardJobs(t.Context(), model.DashboardJobsQuery{State: "completed", Search: "finished-movie", Limit: 25, Offset: offset})
		if err != nil || page.Total != 28 || page.Offset != offset || len(page.Jobs) != min(25, 28-offset) {
			t.Fatalf("GetDashboardJobs() = %+v, %v", page, err)
		}
		for _, j := range page.Jobs {
			if seen[j.ID] || j.State != "completed" {
				t.Fatalf("GetDashboardJobs() duplicate or wrong state = %+v", j)
			}
			seen[j.ID] = true
		}
	}
	for _, query := range []model.DashboardJobsQuery{
		{State: "completed", Search: "TEST-cpu", Limit: 25},
		{State: "completed", Search: "%", Limit: 25},
		{State: "completed", Search: "finished-movie", Limit: 25, Offset: 1000},
	} {
		page, err := repo.GetDashboardJobs(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		if query.Search == "TEST-cpu" {
			if page.Total != 28 {
				t.Fatalf("worker search total = %d, want 28", page.Total)
			}
		} else if len(page.Jobs) != 0 {
			t.Fatalf("literal search/empty page = %+v", page)
		}
	}
}
func TestDashboardJobPages(t *testing.T) { checkDashboardJobPages(t, NewInMemoryRepository()) }

func checkDashboardWorkerRecency(t *testing.T, repo Repository) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	tests := []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"recency-online", time.Second, true}, {"recency-offline", time.Hour, true},
		{"recency-boundary", 30 * 24 * time.Hour, true}, {"recency-old", 30*24*time.Hour + time.Second, false},
	}
	for _, tt := range tests {
		if err := repo.PingServerUpdate(t.Context(), "127.0.0.1", model.PingEventType{Event: model.Event{WorkerName: tt.name}}); err != nil {
			t.Fatal(err)
		}
		at := now.Add(-tt.age)
		switch r := repo.(type) {
		case *InMemoryRepository:
			r.mu.Lock()
			r.workers[tt.name].LastSeen = at
			r.mu.Unlock()
		case *SQLRepository:
			if _, err := r.db.ExecContext(t.Context(), "UPDATE workers SET last_seen=$1 WHERE name=$2", at, tt.name); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, hours := range []int{24, 168, 720} {
		d, err := repo.GetDashboard(t.Context(), hours, now)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, w := range d.Workers {
			found[w.Name] = true
		}
		for _, tt := range tests {
			if found[tt.name] != tt.want {
				t.Errorf("GetDashboard(%d) worker %s present=%v, want %v", hours, tt.name, found[tt.name], tt.want)
			}
		}
	}
}
func TestDashboardWorkerRecency(t *testing.T) {
	checkDashboardWorkerRecency(t, NewInMemoryRepository())
}

func checkDashboardCustomRange(t *testing.T, repo Repository) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	period := model.DashboardRange{Start: now.Add(-72 * time.Hour), End: now.Add(-48 * time.Hour)}
	for i, at := range []time.Time{period.Start.Add(-time.Second), period.Start, period.End.Add(-time.Second), period.End, now.Add(time.Hour)} {
		j := &model.Job{Id: uuid.New(), SourcePath: fmt.Sprintf("range-%d.mkv", i), SourceSize: 100, TargetSize: 40, TargetProbe: &model.MediaProbe{Streams: []model.MediaStream{{CodecType: "video", CodecName: "av1"}}}}
		if err := repo.AddJob(t.Context(), j); err != nil {
			t.Fatal(err)
		}
		e := &model.TaskEventType{Event: model.Event{EventTime: at, WorkerName: "range-worker"}, JobId: j.Id, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
		if err := repo.AddNewTaskEvent(t.Context(), e); err != nil {
			t.Fatal(err)
		}
		if r, ok := repo.(*SQLRepository); ok {
			for _, table := range []string{"job_events", "job_status"} {
				if _, err := r.db.ExecContext(t.Context(), "UPDATE "+table+" SET event_time=$1 WHERE job_id=$2", at, j.Id); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// Baseline may contain other fixtures; compare global totals separately from period results.
	d, err := repo.GetDashboard(t.Context(), 0, now, period)
	if err != nil {
		t.Fatal(err)
	}
	if d.Period.Completed != 2 || d.Period.SourceBytes != 200 || d.Period.TargetBytes != 80 || d.Period.SavedBytes != 120 || d.Period.CompressionPercent != 60 {
		t.Fatalf("range summary = %+v, want 2 completions, 200→80, 60%%", d.Period)
	}
	if d.Summary.Completed < 5 || d.Summary.SourceBytes < 500 {
		t.Fatalf("global totals changed: %+v", d.Summary)
	}
	var count, source int64
	for _, b := range d.Timeline {
		count += b.Completed
		source += b.SourceBytes
	}
	if count != 2 || source != 200 {
		t.Fatalf("range timeline=%d, %d, want 2,200", count, source)
	}
	if len(d.Codecs) != 1 || d.Codecs[0].Jobs != 2 {
		t.Fatalf("range codecs=%+v", d.Codecs)
	}
	for _, a := range d.Activity {
		if !period.Contains(a.Time) {
			t.Fatalf("out-of-range activity: %+v", a)
		}
	}
}
func TestDashboardCustomRange(t *testing.T) { checkDashboardCustomRange(t, NewInMemoryRepository()) }
