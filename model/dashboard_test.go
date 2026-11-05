package model

import (
	"testing"
	"time"
)

func TestDashboardRecentWorkersAndOrdering(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	d := NewDashboard(now, 24)
	d.Workers = []DashboardWorker{
		{Name: "old", LastSeen: ago(30*24*time.Hour + time.Second)},
		{Name: "unknown"},
		{Name: "offline", LastSeen: ago(time.Hour)},
		{Name: "idle", LastSeen: ago(time.Second)},
		{Name: "busy", LastSeen: ago(time.Second), ActiveJobs: 2},
		{Name: "boundary", LastSeen: ago(30 * 24 * time.Hour)},
	}
	d.Finalize()
	want := []string{"busy", "idle", "boundary", "offline"}
	if len(d.Workers) != len(want) {
		t.Fatalf("Finalize() = %+v", d.Workers)
	}
	for i, name := range want {
		if d.Workers[i].Name != name || d.Workers[i].Online != (i < 2) {
			t.Errorf("Finalize() worker %d = %+v, want %s", i, d.Workers[i], name)
		}
	}
}
