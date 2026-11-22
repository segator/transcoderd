package step

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
)

func TestUploadRetriesBeyondBatchAfterServerOutage(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if attempts.Add(1) <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "output.mkv")
	if err := os.WriteFile(file, []byte("encoded"), 0600); err != nil {
		t.Fatal(err)
	}
	j := job.NewContext(uuid.New(), 4, t.TempDir())
	j.Target = &job.VideoData{FilePath: file, Checksum: "checksum"}
	e := &UploadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Millisecond, retryAttempts: 2}
	if err := e.upload(context.Background(), &mockStepTracker{}, j); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 4 {
		t.Fatalf("attempts = %d, want 4", got)
	}
}

func TestChecksumRetriesBeyondBatchAfterServerOutage(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 10 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, "checksum")
	}))
	defer server.Close()
	e := &DownloadStepExecutor{BaseDomainURL: server.URL, retryDelay: time.Millisecond}
	j := job.NewContext(uuid.New(), 0, t.TempDir())
	tracker := &mockStepTracker{}
	result, err := e.getChecksum(context.Background(), j, tracker.Logger())
	if err != nil || result != "checksum" || attempts.Load() != 11 {
		t.Fatalf("result=%q err=%v attempts=%d", result, err, attempts.Load())
	}
}

func TestDownloadRetriesBeyondBatchAfterServerOutage(t *testing.T) {
	const content = "complete"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/checksum" {
			sum := sha256.Sum256([]byte(content))
			_, _ = fmt.Fprint(w, hex.EncodeToString(sum[:]))
			return
		}
		if attempts.Add(1) <= 180 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", "8")
		w.Header().Set("Content-Disposition", `attachment; filename="movie.mkv"`)
		_, _ = fmt.Fprint(w, content)
	}))
	defer server.Close()
	e := &DownloadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Microsecond}
	j := job.NewContext(uuid.New(), 0, t.TempDir())
	result, err := e.downloadSource(context.Background(), &mockStepTracker{}, j)
	if err != nil || result == nil || attempts.Load() != 181 {
		t.Fatalf("result=%v err=%v attempts=%d", result, err, attempts.Load())
	}
}

func TestUploadStopsOnExplicitAssignmentExpiry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		if got := r.Header.Get("X-TranscoderD-Assignment"); got != "4" {
			t.Errorf("assignment header = %q", got)
		}
		w.Header().Set("X-TranscoderD-Error", "assignment-expired")
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "output.mkv")
	if err := os.WriteFile(file, []byte("encoded"), 0600); err != nil {
		t.Fatal(err)
	}
	j := job.NewContext(uuid.New(), 4, t.TempDir())
	j.Target = &job.VideoData{FilePath: file, Checksum: "checksum"}
	e := &UploadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Millisecond, retryAttempts: 2}
	if err := e.upload(context.Background(), &mockStepTracker{}, j); !errors.Is(err, serverclient.ErrAssignmentExpired) {
		t.Fatalf("upload = %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}
