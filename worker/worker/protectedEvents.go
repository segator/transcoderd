package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"transcoder/model"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

type protectedEventClient interface {
	PublishEvent(context.Context, model.AssignmentClaim, model.TaskEventType) error
}

type protectedEventPublisher struct {
	mu         sync.Mutex
	client     protectedEventClient
	workerName string
	retryDelay time.Duration
}

func (p *protectedEventPublisher) Reconcile(ctx context.Context, j *job.Context) error {
	if p == nil || p.client == nil || j == nil || j.Assignment == nil || j.Assignment.InstanceID == uuid.Nil {
		return errors.New("protected event publisher is not configured")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if pending := j.PendingProtectedEvent(); pending != nil {
		return p.deliver(ctx, j, *pending)
	}
	return nil
}

// Publish reconciles an unacknowledged event before allocating the next one.
// A lost response is retried with the exact persisted event payload.
func (p *protectedEventPublisher) Publish(ctx context.Context, j *job.Context, phase model.NotificationType, status model.NotificationStatus, message string) error {
	if p == nil || p.client == nil || j == nil || j.Assignment == nil || j.Assignment.InstanceID == uuid.Nil {
		return errors.New("protected event publisher is not configured")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if pending := j.PendingProtectedEvent(); pending != nil {
		if err := p.deliver(ctx, j, *pending); err != nil {
			return err
		}
		if pending.NotificationType == phase && pending.Status == status && pending.Message == message {
			return nil
		}
	}
	event, err := j.NewProtectedEvent(p.workerName, phase, status, message)
	if err != nil {
		return err
	}
	if err := j.StageProtectedEvent(event); err != nil {
		return fmt.Errorf("persist protected event: %w", err)
	}
	return p.deliver(ctx, j, event)
}

func (p *protectedEventPublisher) deliver(ctx context.Context, j *job.Context, event model.TaskEventType) error {
	delay := p.retryDelay
	if delay <= 0 {
		delay = time.Second
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := p.client.PublishEvent(ctx, *j.Assignment, event)
		if err == nil {
			if err := j.AcknowledgeProtectedEvent(event.EventID); err != nil {
				return fmt.Errorf("persist protected event acknowledgement: %w", err)
			}
			return nil
		}
		if errors.Is(err, serverclient.ErrProtectedConflict) || errors.Is(err, serverclient.ErrProtectedUnavailable) {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
