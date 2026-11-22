package scheduler

import (
	"errors"
	"testing"

	"transcoder/model"
)

func TestValidateLegacyNotification(t *testing.T) {
	assigned := &model.TaskEventType{Event: model.Event{WorkerName: "one"}, EventID: 1, NotificationType: model.JobNotification, Status: model.AssignedNotificationStatus}
	started := &model.TaskEventType{Event: model.Event{WorkerName: "one"}, EventID: 2, NotificationType: model.JobNotification, Status: model.StartedNotificationStatus}
	step := &model.TaskEventType{Event: model.Event{WorkerName: "one"}, EventID: 3, NotificationType: model.FFMPEGSNotification, Status: model.CompletedNotificationStatus}
	tests := []struct {
		name   string
		events model.TaskEvents
		event  model.TaskEventType
		replay bool
		err    error
	}{
		{"new", model.TaskEvents{assigned, started}, *step, false, nil},
		{"replay", model.TaskEvents{assigned, started, step}, *step, true, nil},
		{"conflict", model.TaskEvents{assigned, started, step}, model.TaskEventType{Event: model.Event{WorkerName: "one"}, EventID: 3, NotificationType: model.JobVerify, Status: model.StartedNotificationStatus}, false, ErrNotificationConflict},
		{"other worker", model.TaskEvents{assigned, started}, model.TaskEventType{Event: model.Event{WorkerName: "two"}, EventID: 3}, false, ErrAssignmentExpired},
		{"reassigned same worker", model.TaskEvents{assigned, started, step, {Event: model.Event{WorkerName: "one"}, EventID: 5, NotificationType: model.JobNotification, Status: model.AssignedNotificationStatus}}, *step, false, ErrAssignmentExpired},
		{"sequence gap", model.TaskEvents{assigned, started}, model.TaskEventType{Event: model.Event{WorkerName: "one"}, EventID: 4}, false, ErrNotificationConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replay, err := validateLegacyNotification(&model.Job{Events: tt.events}, &tt.event)
			if replay != tt.replay || !errors.Is(err, tt.err) {
				t.Fatalf("validate = %v, %v; want %v, %v", replay, err, tt.replay, tt.err)
			}
		})
	}
}
