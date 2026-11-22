package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"transcoder/model"
)

// NewLegacyEvent allocates the next sequence from the last acknowledged
// snapshot. The caller must still stage it before transmission.
func (e *Context) NewLegacyEvent(worker string, phase model.NotificationType, status model.NotificationStatus, message string) (model.TaskEventType, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PendingEvent != nil {
		return model.TaskEventType{}, ErrPendingEvent
	}
	if worker == "" {
		return model.TaskEventType{}, errors.New("worker is missing")
	}
	id := e.EventId + 1
	if e.LastEvent != nil {
		id = e.LastEvent.EventId + 1
	}
	event := model.TaskEventType{Event: model.Event{WorkerName: worker, EventTime: time.Now().UTC()}, JobId: e.JobId, EventID: id, NotificationType: phase, Status: status, Message: message}
	assignmentID := e.EventId
	event.AssignmentEventID = &assignmentID
	if phase == model.JobNotification && status == model.CompletedNotificationStatus {
		event.TargetProbe = e.TargetMediaProbe
	}
	return cloneLegacyEvent(event), nil
}

var ErrPendingEvent = errors.New("another legacy event awaits acknowledgement")

// StageLegacyEvent persists exactly one immutable event before transmission.
// Until it is acknowledged, later events cannot consume its sequence number.
func (e *Context) StageLegacyEvent(event model.TaskEventType) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if event.JobId != e.JobId || event.WorkerName == "" {
		return errors.New("legacy event does not match job or worker")
	}
	if e.PendingEvent != nil {
		oldData, oldErr := json.Marshal(e.PendingEvent)
		newData, newErr := json.Marshal(event)
		if oldErr == nil && newErr == nil && bytes.Equal(oldData, newData) {
			return nil
		}
		return ErrPendingEvent
	}
	next := e.EventId + 1
	if e.LastEvent != nil {
		next = e.LastEvent.EventId + 1
	}
	if event.EventID != next {
		return fmt.Errorf("legacy event sequence %d, expected %d", event.EventID, next)
	}
	stored := cloneLegacyEvent(event)
	e.PendingEvent = &stored
	if err := e.persistJobContextLocked(); err != nil {
		e.PendingEvent = nil
		return err
	}
	return nil
}

func (e *Context) PendingLegacyEvent() *model.TaskEventType {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PendingEvent == nil {
		return nil
	}
	event := cloneLegacyEvent(*e.PendingEvent)
	return &event
}

func cloneLegacyEvent(event model.TaskEventType) model.TaskEventType {
	if event.AssignmentEventID != nil {
		id := *event.AssignmentEventID
		event.AssignmentEventID = &id
	}
	if event.TargetProbe != nil {
		probe := *event.TargetProbe
		probe.Streams = append([]model.MediaStream(nil), probe.Streams...)
		event.TargetProbe = &probe
	}
	return event
}

// AcknowledgeLegacyEvent advances the durable sequence only after the
// server confirms the exact staged event. A failed write leaves it pending.
func (e *Context) AcknowledgeLegacyEvent(eventID int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PendingEvent == nil {
		if e.LastEvent != nil && e.LastEvent.EventId == eventID {
			return nil
		}
		return errors.New("no matching legacy event is pending")
	}
	if e.PendingEvent.EventID != eventID {
		return errors.New("legacy event acknowledgement identity mismatch")
	}
	pending, last := e.PendingEvent, e.LastEvent
	e.LastEvent = &ContextEvent{EventId: pending.EventID, NotificationType: pending.NotificationType, Status: pending.Status, Message: pending.Message}
	e.PendingEvent = nil
	if err := e.persistJobContextLocked(); err != nil {
		e.PendingEvent, e.LastEvent = pending, last
		return err
	}
	return nil
}
