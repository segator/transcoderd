package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"transcoder/model"
)

func TestDashboardIndependentWorkerLimitsAreAtomic(t *testing.T) {
	s := newWorkerSettingsTestServer(t)
	cookie := loginDashboard(t, s)
	for _, name := range []string{"mixed", "other"} {
		body := `{"cpuJobs":2,"gpuJobs":3}`
		if name == "other" {
			body = `{"cpuJobs":1,"gpuJobs":0}`
		}
		if w := dashboardRequest(s, "PUT", "/dashboard/api/workers/"+name+"/settings", body, "", cookie); w.Code != 204 {
			t.Fatalf("pair write = %d %s", w.Code, w.Body.String())
		}
	}
	max := strconv.FormatInt(int64(int(^uint(0)>>1)), 10)
	for _, body := range []string{`{"cpuJobs":2}`, `{"gpuJobs":3}`, `{"cpuJobs":-1,"gpuJobs":3}`, `{"cpuJobs":2,"gpuJobs":-1}`, `{"cpuJobs":1.2,"gpuJobs":3}`, `{"maxActiveJobs":5,"cpuJobs":2,"gpuJobs":3}`, `{"cpuJobs":` + max + `,"gpuJobs":1}`} {
		if w := dashboardRequest(s, "PUT", "/dashboard/api/workers/mixed/settings", body, "", cookie); w.Code != 400 {
			t.Errorf("invalid pair %s = %d, want 400", body, w.Code)
		}
	}
	got, err := s.scheduler.GetWorkerSettings(t.Context(), "mixed")
	if err != nil || !got.Independent() || *got.CPUJobs != 2 || *got.GPUJobs != 3 {
		t.Fatalf("stored pair after invalid updates = %+v, %v", got, err)
	}
	for _, capability := range []string{"", model.IndependentEncoderCapability} {
		r := httptest.NewRequest("GET", "/api/v1/worker/settings", nil)
		r.Header.Set("workerName", "mixed")
		r.Header.Set("Authorization", "Bearer test-token")
		r.Header.Set("workerCapabilities", capability)
		w := httptest.NewRecorder()
		s.srv.Handler.ServeHTTP(w, r)
		var settings model.WorkerSettings
		if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
			t.Fatal(err)
		}
		want := 0
		if capability != "" {
			want = 5
		}
		if w.Code != 200 || settings.MaxActiveJobs == nil || *settings.MaxActiveJobs != want || !settings.Independent() {
			t.Errorf("capability %q settings = %+v / %d, want aggregate %d", capability, settings, w.Code, want)
		}
	}
	// A worker-authenticated pair write also uses the same atomic settings path.
	r := httptest.NewRequest("PUT", "/api/v1/worker/settings", strings.NewReader(`{"cpuJobs":0,"gpuJobs":0}`))
	r.Header.Set("workerName", "mixed")
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("worker pair write = %d %s", w.Code, w.Body.String())
	}
	other, _ := s.scheduler.GetWorkerSettings(t.Context(), "other")
	if *other.CPUJobs != 1 || *other.GPUJobs != 0 {
		t.Error("pair update changed another worker")
	}
	if w := dashboardRequest(s, "PUT", "/dashboard/api/workers/mixed/settings", `{"maxActiveJobs":4}`, "", cookie); w.Code != 204 {
		t.Fatalf("legacy write = %d", w.Code)
	}
	got, _ = s.scheduler.GetWorkerSettings(t.Context(), "mixed")
	if got.Independent() || got.CPUJobs != nil || got.GPUJobs != nil || *got.MaxActiveJobs != 4 {
		t.Errorf("legacy reset = %+v, want cleared pair", got)
	}
}
