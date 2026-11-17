package scheduler

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type failingSourceReader struct{}

func (f failingSourceReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated disk I/O error")
}

func TestSourceReadGuardAssignmentAndExpiry(t *testing.T) {
	var guard sourceReadGuard
	now := time.Now()
	first := sourceReadAssignment{jobID: "job", eventID: 2}
	next := sourceReadAssignment{jobID: "job", eventID: 3}
	guard.block(first, now)
	if !guard.isBlocked(first, now) {
		t.Error("failed assignment was not blocked")
	}
	if guard.isBlocked(next, now) {
		t.Error("new assignment was blocked")
	}
	if guard.isBlocked(first, now.Add(sourceReadGuardDuration)) {
		t.Error("expired assignment remains blocked")
	}
}

func TestSourceReadGuardConcurrentAccess(t *testing.T) {
	var guard sourceReadGuard
	assignment := sourceReadAssignment{jobID: "job", eventID: 1}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			guard.block(assignment, time.Now())
			_ = guard.isBlocked(assignment, time.Now())
		}()
	}
	wg.Wait()
	if !guard.isBlocked(assignment, time.Now()) {
		t.Error("assignment should remain blocked")
	}
}

func TestDownloadStreamRecordsSourceError(t *testing.T) {
	var got error
	stream := &DownloadJobStream{sourceReader: failingSourceReader{}, onReadError: func(err error) { got = err }}
	_, err := stream.Read(make([]byte, 10))
	if err == nil || got == nil || got.Error() != err.Error() {
		t.Fatalf("Read() error = %v, recorded = %v", err, got)
	}
	got = nil
	stream.sourceReader = emptySourceReader{}
	if _, err = stream.Read(make([]byte, 10)); !errors.Is(err, io.EOF) || got != nil {
		t.Fatalf("EOF error = %v, recorded = %v", err, got)
	}
}

type emptySourceReader struct{}

func (emptySourceReader) Read([]byte) (int, error) { return 0, io.EOF }
