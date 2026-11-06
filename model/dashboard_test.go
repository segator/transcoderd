package model

import (
	"encoding/json"
	"strings"
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

func TestWorkerRuntimeVersionWireCompatibility(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{`{"controlVersion":1}`, ""},
		{`{"version":"v2.13.2","controlVersion":2}`, "v2.13.2"},
	} {
		var runtime WorkerRuntime
		if err := json.Unmarshal([]byte(tt.raw), &runtime); err != nil {
			t.Fatal(err)
		}
		if runtime.Version != tt.want {
			t.Errorf("runtime version=%q,want %q", runtime.Version, tt.want)
		}
		encoded, err := json.Marshal(runtime)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), `"version"`) != (tt.want != "") {
			t.Fatalf("optional version wire=%s", encoded)
		}
	}
}
