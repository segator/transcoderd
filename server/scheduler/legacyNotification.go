package scheduler

import (
	"errors"
	"fmt"

	"transcoder/model"
)

var ErrAssignmentExpired = errors.New("worker assignment expired")
var ErrNotificationConflict = errors.New("notification conflicts with job history")

// validateLegacyNotification accepts an exact replay only while the same
// assignment owns the job. A later assignment supersedes every older event,
// including events previously acknowledged to the same worker name.
func validateLegacyNotification(job *model.Job, event *model.TaskEventType) (bool, error) {
	var assigned *model.TaskEventType
	var matching *model.TaskEventType
	for _, old := range job.Events {
		if old.NotificationType == model.JobNotification && old.Status == model.AssignedNotificationStatus &&
			(assigned == nil || old.EventID > assigned.EventID) {
			assigned = old
		}
		if old.EventID == event.EventID {
			matching = old
		}
	}
	state := job.Events.GetLatestPerNotificationType(model.JobNotification)
	if assigned == nil || state == nil || assigned.WorkerName != event.WorkerName ||
		event.EventID <= assigned.EventID {
		return false, ErrAssignmentExpired
	}
	if event.AssignmentEventID != nil && *event.AssignmentEventID != assigned.EventID {
		return false, ErrAssignmentExpired
	}
	if state.Status != model.AssignedNotificationStatus && state.Status != model.StartedNotificationStatus {
		if matching == nil || !matching.IsCompleted() || state.EventID != matching.EventID {
			return false, ErrAssignmentExpired
		}
	}
	if matching != nil {
		if matching.DeliveryDigest != "" {
			digest, err := model.NotificationDigest(event)
			if err != nil {
				return false, err
			}
			if digest == matching.DeliveryDigest {
				return true, nil
			}
			return false, ErrNotificationConflict
		}
		// Older rows have no digest; compare the fields they retained.
		if matching.WorkerName == event.WorkerName && matching.NotificationType == event.NotificationType &&
			matching.Status == event.Status && matching.Message == event.Message {
			return true, nil
		}
		return false, ErrNotificationConflict
	}
	latest := job.Events.GetLatest()
	if latest == nil || event.EventID != latest.EventID+1 {
		return false, fmt.Errorf("%w: unexpected sequence", ErrNotificationConflict)
	}
	return false, nil
}
