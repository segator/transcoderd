package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"transcoder/model"
)

var ErrPendingEvent = errors.New("another protected event awaits acknowledgement")

// StageProtectedEvent persists exactly one immutable event before transmission.
// Until it is acknowledged, later events cannot consume its sequence number.
func (e *Context) StageProtectedEvent(event model.TaskEventType) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if event.JobId != e.JobId || event.WorkerName == "" {
		return errors.New("protected event does not match job or worker")
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
		return fmt.Errorf("protected event sequence %d, expected %d", event.EventID, next)
	}
	stored := cloneProtectedEvent(event)
	e.PendingEvent = &stored
	if err := e.persistJobContextLocked(); err != nil {
		e.PendingEvent = nil
		return err
	}
	return nil
}

func (e *Context) PendingProtectedEvent() *model.TaskEventType {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PendingEvent == nil {
		return nil
	}
	event := cloneProtectedEvent(*e.PendingEvent)
	return &event
}

func cloneProtectedEvent(event model.TaskEventType) model.TaskEventType {
	if event.TargetProbe != nil {
		probe := *event.TargetProbe
		probe.Streams = append([]model.MediaStream(nil), probe.Streams...)
		event.TargetProbe = &probe
	}
	return event
}

// AcknowledgeProtectedEvent advances the durable sequence only after the
// server confirms the exact staged event. A failed write leaves it pending.
func (e *Context) AcknowledgeProtectedEvent(eventID int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.PendingEvent == nil {
		if e.LastEvent != nil && e.LastEvent.EventId == eventID {
			return nil
		}
		return errors.New("no matching protected event is pending")
	}
	if e.PendingEvent.EventID != eventID {
		return errors.New("protected event acknowledgement identity mismatch")
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
