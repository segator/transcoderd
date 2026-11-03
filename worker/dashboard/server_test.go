package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"transcoder/worker/control"
)

type testWorker struct {
	status control.AdmissionStatus
	limit  int
}

func (w *testWorker) AdmissionStatus() control.AdmissionStatus { return w.status }
func (w *testWorker) PersistMaxActiveJobs(limit int) error {
	w.limit = limit
	w.status.MaxActiveJobs = limit
	w.status.Accepting = limit > 0
	return nil
}

func TestDashboardHTTPAuthStatusAndControl(t *testing.T) {
	worker := &testWorker{status: control.AdmissionStatus{Accepting: true, MaxActiveJobs: 2, MaxEncodes: 2, Jobs: []control.JobState{{JobID: "job-1", Codec: "libsvtav1", Profile: "cpu"}}}}
	server, err := Start("127.0.0.1:0", "secret-token", worker)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Shutdown(); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	}()
	go func() {
		if err := server.Serve(); err != nil {
			t.Errorf("Serve() error = %v", err)
		}
	}()
	baseURL := "http://" + server.listener.Addr().String()

	unauthorized, err := http.Get(baseURL + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}
	_ = unauthorized.Body.Close()

	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var status statusResponse
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !status.Configured || status.Version == "" || len(status.Worker.Jobs) != 1 || status.Worker.Jobs[0].Codec != "libsvtav1" {
		t.Fatalf("authorized status = %+v, HTTP %d", status, response.StatusCode)
	}

	controlRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/worker/max-jobs", strings.NewReader(`{"maxJobs":0}`))
	if err != nil {
		t.Fatal(err)
	}
	controlRequest.Header.Set("Authorization", "Bearer secret-token")
	controlRequest.Header.Set("Content-Type", "application/json")
	controlResponse, err := http.DefaultClient.Do(controlRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, controlResponse.Body)
	_ = controlResponse.Body.Close()
	if controlResponse.StatusCode != http.StatusOK || worker.limit != 0 {
		t.Fatalf("control response = %d, worker limit = %d", controlResponse.StatusCode, worker.limit)
	}

	assetsResponse, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(assetsResponse.Body)
	_ = assetsResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if assetsResponse.StatusCode != http.StatusOK || !strings.Contains(string(page), "TranscoderD") || !strings.Contains(string(page), "activity-filters") {
		t.Fatalf("dashboard page missing expected UI: HTTP %d", assetsResponse.StatusCode)
	}
}

func TestDashboardRequiresTokenWhenEnabled(t *testing.T) {
	if server, err := Start("", "", nil); err != nil || server != nil {
		t.Fatalf("disabled dashboard = %v, %v; want nil, nil", server, err)
	}
	if _, err := Start("127.0.0.1:0", "", &testWorker{}); err == nil {
		t.Fatal("dashboard started without a token")
	}
}

func (w *testWorker) SetJobPaused(jobID string, paused bool) error {
	for i := range w.status.Jobs {
		j := &w.status.Jobs[i]
		if j.JobID == jobID && j.CanPause {
			j.IndividuallyPaused = paused
			j.State = "running"
			if paused {
				j.State = "paused"
			}
			return nil
		}
	}
	return control.ErrJobNotPausable
}

func TestIndividualEncodingHTTPControls(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	worker := &testWorker{status: control.AdmissionStatus{MaxActiveJobs: 2, Jobs: []control.JobState{{JobID: id, Phase: "FFMPEG", State: "running", CanPause: true}}}}
	server, err := Start("127.0.0.1:0", "secret", worker)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Shutdown(); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		if err := server.Serve(); err != nil {
			t.Error(err)
		}
	}()
	for _, tt := range []struct {
		suffix, token string
		want          int
		paused        bool
	}{
		{id + "/pause", "", 401, false}, {id + "/pause", "secret", 200, true}, {id + "/resume", "secret", 200, false},
		{"00000000-0000-4000-8000-000000000002/pause", "secret", 404, false}, {"invalid/pause", "secret", 400, false},
	} {
		request, err := http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/api/worker/jobs/"+tt.suffix, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+tt.token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != tt.want || worker.status.Jobs[0].IndividuallyPaused != tt.paused || worker.status.MaxActiveJobs != 2 {
			t.Fatalf("%s=%d state=%+v,want %d paused %t", tt.suffix, response.StatusCode, worker.status, tt.want, tt.paused)
		}
	}
}

func (w *testWorker) SetRAMPaused(paused bool) error      { w.status.RAMPaused = paused; return nil }
func (w *testWorker) PersistAdmissionLimit(max int) error { return w.PersistMaxActiveJobs(max) }

func TestSeparateEncodingHTTPControlAuthAndLimits(t *testing.T) {
	worker := &testWorker{status: control.AdmissionStatus{MaxActiveJobs: 3}}
	server, err := Start("127.0.0.1:0", "secret", worker)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := server.Shutdown(); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		if err := server.Serve(); err != nil {
			t.Error(err)
		}
	}()
	for _, tt := range []struct {
		action, token string
		want          int
		paused        bool
	}{{"pause", "", 401, false}, {"pause", "secret", 200, true}, {"resume", "secret", 200, false}} {
		req, err := http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/api/worker/encodes/"+tt.action, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+tt.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode != tt.want || worker.status.RAMPaused != tt.paused || worker.status.MaxActiveJobs != 3 {
			t.Fatalf("%s status=%d worker=%+v", tt.action, res.StatusCode, worker.status)
		}
	}
}
