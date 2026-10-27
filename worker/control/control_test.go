package control

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdmissionStatusJSONIncludesAcceptedJobCeiling(t *testing.T) {
	status := AdmissionStatus{MaxActiveJobs: 2, MaxAcceptedJobs: 4}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"maxAcceptedJobs":4`) {
		t.Fatalf("serialized admission status = %s, want maxAcceptedJobs 4", encoded)
	}
}

type fakeController struct {
	mu        sync.Mutex
	status    AdmissionStatus
	persisted int
}

func (f *fakeController) AdmissionStatus() AdmissionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeController) PersistMaxActiveJobs(max int) error {
	if max < 0 {
		return os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.persisted++
	f.status.MaxActiveJobs = max
	f.status.MaxAcceptedJobs = max
	if max > 0 {
		f.status.MaxAcceptedJobs += 2
	}
	f.status.Accepting = max > 0
	if max > 0 {
		f.status.MaxEncodes = max
	}
	return nil
}

func (f *fakeController) SetMaxActiveJobs(max int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.MaxActiveJobs = max
	f.status.Accepting = max > 0
	return nil
}

func TestTransientDrainAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.sock")
	state := &fakeController{status: AdmissionStatus{Accepting: true, MaxActiveJobs: 3, ActiveJobs: 1, MaxEncodes: 3}}
	shutdown := make(chan struct{}, 1)
	server, err := Start(path, state, func() { shutdown <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	got, err := Call(path, Request{Command: "drain"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Accepting || got.Status.ActiveJobs != 1 || got.Status.MaxEncodes != 3 || state.persisted != 0 {
		t.Fatalf("drain status = %+v, persisted = %d", got.Status, state.persisted)
	}
	if _, err := Call(path, Request{Command: "shutdown"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-shutdown:
	case <-time.After(time.Second):
		t.Fatal("shutdown was not requested")
	}
}

func TestControlCommandsUpdateRunningWorker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.sock")
	state := &fakeController{status: AdmissionStatus{Accepting: true, ActiveJobs: 4, MaxActiveJobs: 4, MaxAcceptedJobs: 6, ActiveEncodes: 2, MaxEncodes: 2}}
	server, err := Start(path, state)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("socket permissions = %o, want 600", got)
		}
	}

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"set-max-jobs", "0"}, want: "max jobs set to 0; admitted_limit=0; accepting=false encodes=2/2"},
		{args: []string{"set-max-jobs", "1"}, want: "max jobs set to 1; admitted_limit=3; accepting=true encodes=2/1"},
		{args: []string{"status"}, want: "accepting=true active=4 reserved=0 max_jobs=1 admitted_limit=3 encodes=2/1"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, "-"), func(t *testing.T) {
			var output bytes.Buffer
			if err := runCLI(tt.args, &output, path); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tt.want) {
				t.Fatalf("command output = %q, want it to contain %q", output.String(), tt.want)
			}
		})
	}
}

func TestControlRejectsInvalidMaximumWithoutChangingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.sock")
	state := &fakeController{status: AdmissionStatus{Accepting: true, MaxActiveJobs: 2}}
	server, err := Start(path, state)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	for _, value := range []string{"-1", "x"} {
		if err := runCLI([]string{"set-max-jobs", value}, &bytes.Buffer{}, path); err == nil {
			t.Errorf("set-max-jobs %q returned no error", value)
		}
	}
	if got := state.AdmissionStatus().MaxActiveJobs; got != 2 {
		t.Fatalf("maximum after invalid commands = %d, want 2", got)
	}
}

func TestStartRejectsActiveSocketAndRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.sock")
	listener, err := listenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	state := &fakeController{}
	if _, err := Start(path, state); err == nil {
		t.Fatal("Start() succeeded with an active socket")
	}
	_ = listener.Close()
	deadline := time.Now().Add(time.Second)
	for runtime.GOOS != "windows" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test listener did not remove its socket")
		}
		time.Sleep(10 * time.Millisecond)
	}

	server, err := Start(path, state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server.Serve(ctx)
	_ = server.Close()
}

func (f *fakeController) HoldAdmission() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.Accepting = false
}

func TestMandatoryHoldPreservesLimitsAndAcceptedWork(t *testing.T) {
	state := &fakeController{status: AdmissionStatus{Accepting: true, MaxActiveJobs: 4, MaxEncodes: 4, ActiveJobs: 3, ReservedJobs: 1}}
	path := filepath.Join(t.TempDir(), "worker.sock")
	server, err := Start(path, state)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	got, err := Call(path, Request{Command: "hold-admission"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Accepting || got.Status.MaxActiveJobs != 4 || got.Status.ActiveJobs != 3 || got.Status.ReservedJobs != 1 || state.persisted != 0 {
		t.Fatalf("mandatory hold changed accepted work/limits: %+v", got.Status)
	}
}
