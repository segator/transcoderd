package job

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

func TestProtectedEventOutboxSurvivesLostAcknowledgement(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "job")
	jobContext := NewContext(uuid.New(), 4, workspace)
	if err := jobContext.Init(); err != nil {
		t.Fatal(err)
	}
	event := model.TaskEventType{Event: model.Event{WorkerName: "node", EventTime: time.Now().UTC()}, JobId: jobContext.JobId, EventID: 5, NotificationType: model.DownloadNotification, Status: model.StartedNotificationStatus}
	if err := jobContext.StageProtectedEvent(event); err != nil {
		t.Fatal(err)
	}
	if err := jobContext.StageProtectedEvent(event); err != nil {
		t.Fatalf("retry staged event = %v", err)
	}
	other := event
	other.EventID++
	if err := jobContext.StageProtectedEvent(other); !errors.Is(err, ErrPendingEvent) {
		t.Fatalf("stage next event = %v", err)
	}
	reloaded, err := ReadContextFromDiskByPath(filepath.Join(workspace, jobContext.JobId.String()+".json"))
	if err != nil || reloaded.PendingProtectedEvent() == nil || reloaded.PendingProtectedEvent().EventID != event.EventID {
		t.Fatalf("pending event after restart = %+v, %v", reloaded, err)
	}
	// An acknowledged event must remain retryable if its local acknowledgement
	// cannot be saved, such as when the workspace temporarily disappears.
	if err := os.Rename(workspace, workspace+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.AcknowledgeProtectedEvent(event.EventID); err == nil || reloaded.PendingProtectedEvent() == nil {
		t.Fatal("failed local acknowledgement lost the pending event")
	}
	if err := os.Rename(workspace+"-unavailable", workspace); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.AcknowledgeProtectedEvent(event.EventID); err != nil {
		t.Fatal(err)
	}
	reloaded, err = ReadContextFromDiskByPath(filepath.Join(workspace, jobContext.JobId.String()+".json"))
	if err != nil || reloaded.PendingProtectedEvent() != nil || reloaded.LastEvent == nil || reloaded.LastEvent.EventId != event.EventID {
		t.Fatalf("acknowledged event after restart = %+v, %v", reloaded, err)
	}
	if err := reloaded.StageProtectedEvent(other); err != nil {
		t.Fatalf("stage next event after acknowledgement = %v", err)
	}
}
