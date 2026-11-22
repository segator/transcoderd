package job

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"transcoder/model"
)

func TestLegacyPendingEventSurvivesRestart(t *testing.T) {
	id := uuid.New()
	dir := t.TempDir()
	j := NewContext(id, 3, dir)
	event, err := j.NewLegacyEvent("worker", model.FFMPEGSNotification, model.CompletedNotificationStatus, "done")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.StageLegacyEvent(event); err != nil {
		t.Fatal(err)
	}
	restored, err := ReadContextFromDiskByPath(filepath.Join(dir, id.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	pending := restored.PendingLegacyEvent()
	if pending == nil || pending.EventID != event.EventID || !pending.EventTime.Equal(event.EventTime) || pending.Message != event.Message {
		t.Fatalf("pending = %+v, want %+v", pending, event)
	}
	if err := restored.AcknowledgeLegacyEvent(event.EventID); err != nil {
		t.Fatal(err)
	}
	restored, err = ReadContextFromDiskByPath(filepath.Join(dir, id.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if restored.PendingLegacyEvent() != nil || restored.LastEvent == nil || restored.LastEvent.EventId != event.EventID {
		t.Fatalf("restored acknowledgement = %+v", restored.LastEvent)
	}
}
