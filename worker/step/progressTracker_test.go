package step

import (
	"bytes"
	"crypto/sha256"
	"io"
	"testing"
	"transcoder/worker/console"
)

// Mock logger
type mockTestLogger struct{}

func (m *mockTestLogger) Logf(format string, args ...interface{})   {}
func (m *mockTestLogger) Warnf(format string, args ...interface{})  {}
func (m *mockTestLogger) Cmdf(format string, args ...interface{})   {}
func (m *mockTestLogger) Errorf(format string, args ...interface{}) {}

type mockTracker struct {
	total      int64
	current    int64
	increments []int
}

func (m *mockTracker) SetTotal(total int64) {
	m.total = total
}

func (m *mockTracker) Increment(n int) {
	m.current += int64(n)
	m.increments = append(m.increments, n)
}

func (m *mockTracker) UpdateValue(value int64) {
	m.current = value
}

func (m *mockTracker) Logger() console.LeveledLogger {
	return &mockTestLogger{}
}

type mockReadCloser struct {
	io.Reader
	closed bool
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestNewProgressTrackStream(t *testing.T) {
	tracker := &mockTracker{}
	data := []byte("test data")
	reader := &mockReadCloser{Reader: bytes.NewReader(data)}

	progressReader := NewProgressTrackStream(tracker, reader)

	if progressReader == nil {
		t.Fatal("NewProgressTrackStream() returned nil")
	}
	if progressReader.sha == nil {
		t.Error("sha hash not initialized")
	}
}

func TestProgressTrackReaderRead(t *testing.T) {
	tracker := &mockTracker{}
	testData := []byte("Hello, World! This is test data.")
	reader := &mockReadCloser{Reader: bytes.NewReader(testData)}

	progressReader := NewProgressTrackStream(tracker, reader)

	buf := make([]byte, len(testData))
	n, err := progressReader.Read(buf)

	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() n = %d, want %d", n, len(testData))
	}
	if !bytes.Equal(buf, testData) {
		t.Errorf("Read() data = %q, want %q", buf, testData)
	}
	if tracker.current != int64(len(testData)) {
		t.Errorf("tracker.current = %d, want %d", tracker.current, len(testData))
	}
}

func TestProgressTrackReaderSumSha(t *testing.T) {
	tracker := &mockTracker{}
	testData := []byte("Hello, World!")
	reader := &mockReadCloser{Reader: bytes.NewReader(testData)}

	progressReader := NewProgressTrackStream(tracker, reader)

	// Read all data
	buf := make([]byte, len(testData))
	_, err := progressReader.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Calculate expected checksum
	expectedHash := sha256.Sum256(testData)

	// Get checksum from progress reader
	actualHash := progressReader.SumSha()

	if !bytes.Equal(actualHash, expectedHash[:]) {
		t.Errorf("SumSha() = %x, want %x", actualHash, expectedHash)
	}
}

func TestProgressTrackReaderMultipleReads(t *testing.T) {
	tracker := &mockTracker{}
	testData := []byte("This is a longer test string that will be read in multiple chunks")
	reader := &mockReadCloser{Reader: bytes.NewReader(testData)}

	progressReader := NewProgressTrackStream(tracker, reader)

	// Read in small chunks
	chunkSize := 10
	var allData []byte
	buf := make([]byte, chunkSize)

	for {
		n, err := progressReader.Read(buf)
		if n > 0 {
			allData = append(allData, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
	}

	// Verify all data was read
	if !bytes.Equal(allData, testData) {
		t.Errorf("Read data = %q, want %q", allData, testData)
	}

	// Verify tracker was incremented correctly
	if tracker.current != int64(len(testData)) {
		t.Errorf("tracker.current = %d, want %d", tracker.current, len(testData))
	}

	// Verify number of increments
	if len(tracker.increments) == 0 {
		t.Error("tracker.increments should not be empty")
	}
}
