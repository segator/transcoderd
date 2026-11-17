package job

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"transcoder/model"

	"github.com/google/uuid"
)

// PreparePark records an intent only after validating the completed prefetch.
// The server must acknowledge that intent before ConfirmPark is called.
func (e *Context) PreparePark() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Assignment == nil || e.Assignment.JobID != e.JobId || e.Assignment.Generation == 0 || e.Assignment.Capability == "" || e.Assignment.InstanceID == uuid.Nil || e.PendingEvent != nil || e.NextPhase != model.FFMPEGSNotification || e.LastEvent == nil || e.LastEvent.Status != model.CompletedNotificationStatus {
		return errors.New("job is not at a recoverable pre-encode boundary")
	}
	switch e.LastEvent.NotificationType {
	case model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification:
	default:
		return errors.New("job has not completed pre-encode preparation")
	}
	if err := e.ValidatePrefetchedSource(); err != nil {
		return err
	}
	if e.ParkIntent || e.Parked {
		return nil
	}
	root, err := os.OpenRoot(e.WorkingDir)
	if err != nil {
		return fmt.Errorf("open prefetch workspace: %w", err)
	}
	checksums, err := e.prefetchedArtifactChecksums(root, true)
	_ = root.Close()
	if err != nil {
		return err
	}
	previousArtifacts := e.PrefetchedArtifacts
	if previousArtifacts != nil {
		if len(previousArtifacts) != len(checksums) {
			return fmt.Errorf("%w: subtitle manifest changed while parking", ErrInvalidPrefetchedSource)
		}
		for name, checksum := range checksums {
			if !strings.EqualFold(previousArtifacts[name], checksum) {
				return fmt.Errorf("%w: subtitle %s changed while parking", ErrInvalidPrefetchedSource, name)
			}
		}
	}
	if len(checksums) != 0 {
		e.PrefetchedArtifacts = checksums
	}
	e.ParkIntent = true
	if err := e.persistJobContextLocked(); err != nil {
		e.ParkIntent = false
		e.PrefetchedArtifacts = previousArtifacts
		return fmt.Errorf("persist protected park intent: %w", err)
	}
	return nil
}

func (e *Context) ConfirmPark() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Parked && !e.ParkIntent {
		return nil
	}
	if !e.ParkIntent || e.Assignment == nil || e.PendingEvent != nil {
		return errors.New("protected park intent is missing")
	}
	e.ParkIntent, e.Parked = false, true
	if err := e.persistJobContextLocked(); err != nil {
		e.ParkIntent, e.Parked = true, false
		return fmt.Errorf("persist protected park acknowledgement: %w", err)
	}
	return nil
}

// RecordAcquiredLease persists the process identity after the server grants a
// lease, while retaining any pending event or unresolved park intent.
func (e *Context) RecordAcquiredLease(instance uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if instance == uuid.Nil || e.Assignment == nil {
		return errors.New("cannot record a missing protected lease")
	}
	old := e.Assignment.InstanceID
	e.Assignment.InstanceID = instance
	if err := e.persistJobContextLocked(); err != nil {
		e.Assignment.InstanceID = old
		return fmt.Errorf("persist protected lease identity: %w", err)
	}
	return nil
}

// ResumeWithLease records a freshly acquired process lease before starting
// parked work. It must only be called after the server grants that lease.
func (e *Context) ResumeWithLease(instance uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if instance == uuid.Nil || e.Assignment == nil || e.Assignment.InstanceID != instance || e.PendingEvent != nil {
		return errors.New("cannot resume protected assignment without a lease")
	}
	if e.Parked || e.ParkIntent {
		if e.NextPhase != model.FFMPEGSNotification {
			return errors.New("parked assignment does not have a recoverable encode phase")
		}
		if err := e.ValidatePrefetchedSource(); err != nil {
			return err
		}
	}
	oldInstance, wasParked, hadIntent := e.Assignment.InstanceID, e.Parked, e.ParkIntent
	e.Assignment.InstanceID = instance
	e.Parked, e.ParkIntent = false, false
	if err := e.persistJobContextLocked(); err != nil {
		e.Assignment.InstanceID, e.Parked, e.ParkIntent = oldInstance, wasParked, hadIntent
		return fmt.Errorf("persist protected lease: %w", err)
	}
	return nil
}

func (e *Context) ReadyForProtectedEncode(instance uuid.UUID) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return instance != uuid.Nil && e.Assignment != nil && e.Assignment.InstanceID == instance && !e.Parked && !e.ParkIntent && e.PendingEvent == nil && e.NextPhase == model.FFMPEGSNotification
}
