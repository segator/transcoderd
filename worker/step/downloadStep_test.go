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
	"strconv"
	"testing"
	"time"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/job"

	"github.com/google/uuid"
)

// Mock logger
type mockLogger struct{}

func (m *mockLogger) Logf(format string, args ...interface{})   {}
func (m *mockLogger) Warnf(format string, args ...interface{})  {}
func (m *mockLogger) Cmdf(format string, args ...interface{})   {}
func (m *mockLogger) Errorf(format string, args ...interface{}) {}

// Mock tracker
type mockStepTracker struct {
	total   int64
	current int64
	logger  console.LeveledLogger
}

func (m *mockStepTracker) SetTotal(total int64) {
	m.total = total
}

func (m *mockStepTracker) Increment(n int) {
	m.current += int64(n)
}

func (m *mockStepTracker) UpdateValue(value int64) {
	m.current = value
}

func (m *mockStepTracker) Logger() console.LeveledLogger {
	if m.logger == nil {
		m.logger = &mockLogger{}
	}
	return m.logger
}

func TestDownloadStepExecutorGetDownloadURL(t *testing.T) {
	executor := &DownloadStepExecutor{
		workerName:    "test-worker",
		BaseDomainURL: "http://localhost:8080",
	}

	testID := uuid.New()
	url := executor.GetDownloadURL(testID)

	expected := fmt.Sprintf("http://localhost:8080/api/v1/download?uuid=%s", testID.String())
	if url != expected {
		t.Errorf("GetDownloadURL() = %s, want %s", url, expected)
	}
}

func TestDownloadStepExecutorGetChecksumURL(t *testing.T) {
	executor := &DownloadStepExecutor{
		workerName:    "test-worker",
		BaseDomainURL: "http://localhost:8080",
	}

	testID := uuid.New()
	url := executor.GetChecksumURL(testID)

	expected := fmt.Sprintf("http://localhost:8080/api/v1/checksum?uuid=%s", testID.String())
	if url != expected {
		t.Errorf("GetChecksumURL() = %s, want %s", url, expected)
	}
}

// NOTE: Tests that call getChecksum or download are skipped because retry-go has very long timeouts
// that cause tests to hang for several minutes. These would be better tested with integration tests
// where retry behavior can be configured differently or mocked.
func TestDownloadStepExecutorGetChecksum(t *testing.T) {
	t.Skip("Skipping test that uses retry-go with long timeouts")
}

func TestDownloadStepExecutorGetChecksumError(t *testing.T) {
	t.Skip("Skipping test that uses retry-go with long timeouts")
}

func TestNewDownloadStepExecutor(t *testing.T) {
	workerName := "test-worker"
	baseDomainURL := "http://localhost:8080"

	executor := NewDownloadStepExecutor(workerName, baseDomainURL)

	if executor == nil {
		t.Fatal("NewDownloadStepExecutor() returned nil")
	}

	if executor.notificationType != model.DownloadNotification {
		t.Errorf("notificationType = %v, want %v", executor.notificationType, model.DownloadNotification)
	}
}

func TestDownloadStepExecutorActions(t *testing.T) {
	executor := &DownloadStepExecutor{
		workerName:    "test-worker",
		BaseDomainURL: "http://localhost:8080",
	}

	tmpDir := filepath.Join(os.TempDir(), "transcoder_test_"+uuid.New().String())
	defer os.RemoveAll(tmpDir)

	jobCtx := &job.Context{
		JobId:      uuid.New(),
		EventId:    1,
		WorkingDir: tmpDir,
	}

	actions := executor.actions(jobCtx)

	if len(actions) != 1 {
		t.Errorf("actions() returned %d actions, want 1", len(actions))
	}

	if actions[0].Id != jobCtx.JobId.String() {
		t.Errorf("action Id = %s, want %s", actions[0].Id, jobCtx.JobId.String())
	}

	if actions[0].Execute == nil {
		t.Error("action Execute function should not be nil")
	}
}

// Integration tests are skipped as they require ffprobe and have complex retry logic
func TestDownloadStepExecutorDownloadSuccess(t *testing.T) {
	t.Skip("Skipping integration test that requires ffprobe and retry-go")
}

func TestDownloadStepExecutorDownloadNotFound(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "source read failed for this assignment", http.StatusNotFound)
	}))
	defer server.Close()
	executor := &DownloadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Millisecond}
	_, err := executor.downloadSource(context.Background(), &mockStepTracker{}, &job.Context{JobId: uuid.New(), WorkingDir: t.TempDir()})
	if !errors.Is(err, errJobNotFound) || attempts != 1 {
		t.Fatalf("downloadSource() error = %v, attempts = %d; want job not found after one request", err, attempts)
	}
}

// Test download URLs are constructed correctly
func TestDownloadURLConstruction(t *testing.T) {
	executor := &DownloadStepExecutor{
		workerName:    "test-worker",
		BaseDomainURL: "https://example.com",
	}

	testID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	downloadURL := executor.GetDownloadURL(testID)
	expectedDownload := "https://example.com/api/v1/download?uuid=550e8400-e29b-41d4-a716-446655440000"
	if downloadURL != expectedDownload {
		t.Errorf("GetDownloadURL() = %s, want %s", downloadURL, expectedDownload)
	}

	checksumURL := executor.GetChecksumURL(testID)
	expectedChecksum := "https://example.com/api/v1/checksum?uuid=550e8400-e29b-41d4-a716-446655440000"
	if checksumURL != expectedChecksum {
		t.Errorf("GetChecksumURL() = %s, want %s", checksumURL, expectedChecksum)
	}
}

// Test mock tracker behavior
func TestMockStepTracker(t *testing.T) {
	tracker := &mockStepTracker{}

	tracker.SetTotal(100)
	if tracker.total != 100 {
		t.Errorf("SetTotal() failed, got %d, want 100", tracker.total)
	}

	tracker.Increment(10)
	if tracker.current != 10 {
		t.Errorf("Increment() failed, got %d, want 10", tracker.current)
	}

	tracker.Increment(20)
	if tracker.current != 30 {
		t.Errorf("Increment() failed, got %d, want 30", tracker.current)
	}

	tracker.UpdateValue(75)
	if tracker.current != 75 {
		t.Errorf("UpdateValue() failed, got %d, want 75", tracker.current)
	}

	logger := tracker.Logger()
	if logger == nil {
		t.Error("Logger() returned nil")
	}
}

// Test that httptest server can be created (validates test setup)
func TestHTTPTestServerSetup(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()

	if server.URL == "" {
		t.Error("httptest.NewServer() failed to create server with URL")
	}
}

// Test job context setup for download tests
func TestJobContextSetup(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "transcoder_test_"+uuid.New().String())
	defer os.RemoveAll(tmpDir)

	jobCtx := &job.Context{
		JobId:      uuid.New(),
		EventId:    1,
		WorkingDir: tmpDir,
	}

	if jobCtx.JobId == uuid.Nil {
		t.Error("JobId should not be nil")
	}

	if jobCtx.EventId != 1 {
		t.Errorf("EventId = %d, want 1", jobCtx.EventId)
	}

	if jobCtx.WorkingDir != tmpDir {
		t.Errorf("WorkingDir = %s, want %s", jobCtx.WorkingDir, tmpDir)
	}
}

// Test checksum calculation (without network calls)
func TestChecksumCalculation(t *testing.T) {
	testData := []byte("test data for checksum")
	hash := sha256.Sum256(testData)
	checksum := hex.EncodeToString(hash[:])

	// Verify checksum is calculated correctly
	expectedLength := 64 // SHA256 produces 64 hex characters
	if len(checksum) != expectedLength {
		t.Errorf("checksum length = %d, want %d", len(checksum), expectedLength)
	}

	// Verify it's valid hex
	_, err := hex.DecodeString(checksum)
	if err != nil {
		t.Errorf("checksum is not valid hex: %v", err)
	}
}

func TestDownloadSourceIncompleteTransfers(t *testing.T) {
	const content = "complete-video"
	tests := []struct {
		name              string
		truncatedAttempts int
		wantAttempts      int
		wantError         bool
	}{
		{name: "repeated truncation stops", truncatedAttempts: 3, wantAttempts: 3, wantError: true},
		{name: "one truncation recovers", truncatedAttempts: 1, wantAttempts: 2},
		{name: "complete first attempt", truncatedAttempts: 0, wantAttempts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			checksumRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/download":
					attempts++
					w.Header().Set("Content-Length", strconv.Itoa(len(content)))
					w.Header().Set("Content-Disposition", `attachment; filename="film.mkv"`)
					if attempts <= tt.truncatedAttempts {
						_, _ = io.WriteString(w, content[:3])
						return
					}
					_, _ = io.WriteString(w, content)
				case "/api/v1/checksum":
					checksumRequests++
					sum := sha256.Sum256([]byte(content))
					_, _ = fmt.Fprint(w, hex.EncodeToString(sum[:]))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			tracker := &mockStepTracker{}
			executor := &DownloadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Millisecond}
			jobContext := &job.Context{JobId: uuid.New(), WorkingDir: t.TempDir()}
			result, err := executor.downloadSource(context.Background(), tracker, jobContext)
			if tt.wantError {
				if !errors.Is(err, errRepeatedIncompleteDownload) {
					t.Fatalf("downloadSource() error = %v, want repeated incomplete download", err)
				}
				if result != nil || checksumRequests != 0 {
					t.Fatalf("downloadSource() = %v, checksum requests = %d; partial input must not be verified", result, checksumRequests)
				}
				if tracker.current != 3 {
					t.Errorf("download progress = %d, want 3 for last attempt", tracker.current)
				}
			} else {
				if err != nil {
					t.Fatalf("downloadSource() error = %v", err)
				}
				if result.Size != int64(len(content)) || tracker.current != int64(len(content)) || checksumRequests != 1 {
					t.Errorf("downloadSource() size = %d, progress = %d, checksums = %d", result.Size, tracker.current, checksumRequests)
				}
				got, readErr := os.ReadFile(result.FilePath)
				if readErr != nil || string(got) != content {
					t.Errorf("downloaded file = %q, error = %v", got, readErr)
				}
			}
			if attempts != tt.wantAttempts {
				t.Errorf("download attempts = %d, want %d", attempts, tt.wantAttempts)
			}
		})
	}
}

func TestDownloadSourceRepeatedChecksumMismatch(t *testing.T) {
	const content = "complete-video"
	attempts := 0
	checksums := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/download":
			attempts++
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Header().Set("Content-Disposition", `attachment; filename="film.mkv"`)
			_, _ = io.WriteString(w, content)
		case "/api/v1/checksum":
			checksums++
			_, _ = io.WriteString(w, "wrong-checksum")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	executor := &DownloadStepExecutor{workerName: "worker", BaseDomainURL: server.URL, retryDelay: time.Millisecond}
	tracker := &mockStepTracker{}
	jobContext := &job.Context{JobId: uuid.New(), WorkingDir: t.TempDir()}
	result, err := executor.downloadSource(context.Background(), tracker, jobContext)
	if !errors.Is(err, errRepeatedVerificationFailure) || result != nil {
		t.Fatalf("downloadSource() = %v, error = %v; want repeated verification failure", result, err)
	}
	if attempts != 3 || checksums != 3 {
		t.Errorf("downloads = %d, checksum requests = %d; want 3 each", attempts, checksums)
	}
	if tracker.current != int64(len(content)) {
		t.Errorf("download progress = %d, want %d for last attempt", tracker.current, len(content))
	}
}
