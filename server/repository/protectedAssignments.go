package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"transcoder/model"
	"transcoder/server/assignment"

	"github.com/google/uuid"
)

// RequestProtectedJob binds a request identity to exactly one response. Its
// advisory lock serializes retries across server replicas, including empty
// responses; the job row lock serializes assignment against other requesters.
func (s *SQLRepository) RequestProtectedJob(ctx context.Context, worker, requestID string, keys *assignment.Keyring) (*model.ProtectedAssignment, error) {
	if strings.TrimSpace(worker) == "" || strings.TrimSpace(worker) != worker || len(worker) > 100 || keys == nil {
		return nil, errors.New("invalid protected job request")
	}
	parsedRequestID, err := uuid.Parse(requestID)
	if err != nil {
		return nil, fmt.Errorf("invalid request identity: %w", err)
	}
	requestID = parsedRequestID.String()
	var result *model.ProtectedAssignment
	err = s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, err := tx.getConnection()
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, worker, requestID); err != nil {
			return err
		}
		var jobID, keyID, sourceName sql.NullString
		var generation, eventID sql.NullInt64
		err = conn.QueryRowContext(ctx, `SELECT job_id, generation, key_id, event_id, source_name
FROM protected_job_requests WHERE worker_name=$1 AND request_id=$2`, worker, requestID).
			Scan(&jobID, &generation, &keyID, &eventID, &sourceName)
		if err == nil {
			if !jobID.Valid {
				return nil
			}
			id, err := uuid.Parse(jobID.String)
			if err != nil {
				return err
			}
			capability, err := keys.Capability(keyID.String, worker, jobID.String, uint64(generation.Int64))
			if err != nil {
				return err
			}
			result = &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: id, EventID: int(eventID.Int64), Name: sourceName.String}, Generation: uint64(generation.Int64), KeyID: keyID.String, Capability: capability}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var nextJob string
		err = conn.QueryRowContext(ctx, `SELECT j.id FROM jobs j JOIN job_status s ON s.job_id=j.id
LEFT JOIN protected_assignments p ON p.job_id=j.id
WHERE s.notification_type='Job' AND s.status='queued' AND (p.job_id IS NULL OR
(p.state IN ('revoked','completed') AND NOT EXISTS (SELECT 1 FROM protected_publications x WHERE x.job_id=p.job_id AND x.state IN ('completed','abandoned') AND (x.delete_source_pending OR x.cleanup_pending)))) ORDER BY s.event_time ASC, j.id ASC
FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&nextJob)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = conn.ExecContext(ctx, `INSERT INTO protected_job_requests(worker_name,request_id) VALUES($1,$2)`, worker, requestID)
			return err
		}
		if err != nil {
			return err
		}
		job, err := tx.GetJob(ctx, nextJob)
		if err != nil {
			return err
		}
		// A legacy scheduler may have updated status between selection and the
		// row lock. Never hand an already assigned job to another requester.
		if job.Events.GetStatus() != model.QueuedNotificationStatus {
			return errors.New("queued job changed during protected assignment")
		}
		key := keys.ActiveKeyID()
		var nextGeneration int64
		err = conn.QueryRowContext(ctx, `INSERT INTO protected_assignments(job_id,generation,worker_name,key_id,capability_hash,state)
VALUES($1,1,$2,$3,decode(repeat('00',32),'hex'),'assigned')
ON CONFLICT(job_id) DO UPDATE SET generation=protected_assignments.generation+1, worker_name=EXCLUDED.worker_name,
key_id=EXCLUDED.key_id, capability_hash=EXCLUDED.capability_hash, state='assigned', lease_instance=NULL, lease_expires_at=NULL, updated_at=CURRENT_TIMESTAMP
WHERE protected_assignments.state IN ('revoked','completed') RETURNING generation`, nextJob, worker, key).Scan(&nextGeneration)
		if err != nil {
			return fmt.Errorf("reserve protected assignment: %w", err)
		}
		capability, err := keys.Capability(key, worker, nextJob, uint64(nextGeneration))
		if err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(capability))
		if _, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET capability_hash=$2 WHERE job_id=$1`, nextJob, hash[:]); err != nil {
			return err
		}
		event := job.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
		event.WorkerName, event.EventTime = worker, time.Now().UTC()
		if err := tx.AddNewTaskEvent(ctx, event); err != nil {
			return err
		}
		name := filepath.Base(job.SourcePath)
		if _, err = conn.ExecContext(ctx, `INSERT INTO protected_job_requests(worker_name,request_id,job_id,generation,key_id,event_id,source_name)
VALUES($1,$2,$3,$4,$5,$6,$7)`, worker, requestID, nextJob, nextGeneration, key, event.EventID, name); err != nil {
			return err
		}
		result = &model.ProtectedAssignment{RequestJobResponse: model.RequestJobResponse{Id: job.Id, EventID: event.EventID, Name: name}, Generation: uint64(nextGeneration), KeyID: key, Capability: capability}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func lockJob(ctx context.Context, tx Repository, id string) (SQLDBOperations, *model.Job, error) {
	conn, err := tx.getConnection()
	if err != nil {
		return nil, nil, err
	}
	var locked string
	if err := conn.QueryRowContext(ctx, `SELECT id FROM jobs WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrElementNotFound
		}
		return nil, nil, err
	}
	j, err := tx.GetJob(ctx, locked)
	return conn, j, err
}

func (s *SQLRepository) GetJobForMutation(ctx context.Context, id string) (*model.Job, string, error) {
	if s.tx == nil {
		return nil, "", errors.New("job mutation requires a transaction")
	}
	conn, job, err := lockJob(ctx, s, id)
	if err != nil {
		return nil, "", err
	}
	var state string
	err = conn.QueryRowContext(ctx, `SELECT state FROM protected_assignments WHERE job_id=$1`, id).Scan(&state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	if state == "completed" || state == "revoked" {
		var pending bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM protected_publications WHERE job_id=$1 AND state IN ('completed','abandoned') AND (delete_source_pending OR cleanup_pending))`, id).Scan(&pending); err != nil {
			return nil, "", err
		}
		if pending {
			return nil, "", ErrJobPublishing
		}
	}
	return job, state, nil
}

func (s *SQLRepository) IsProtectedJob(ctx context.Context, id string) (bool, error) {
	conn, err := s.getConnection()
	if err != nil {
		return false, err
	}
	var protected bool
	err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM protected_assignments WHERE job_id=$1)`, id).Scan(&protected)
	return protected, err
}

func (s *SQLRepository) WithLegacyJob(ctx context.Context, id string, fn func(Repository, *model.Job) error) error {
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		_, job, err := lockJob(ctx, tx, id)
		if err != nil {
			return err
		}
		protected, err := tx.IsProtectedJob(ctx, id)
		if err != nil {
			return err
		}
		if protected {
			return ErrStaleAssignment
		}
		return fn(tx, job)
	})
}

func validateProtectedEvent(worker string, claim model.AssignmentClaim, event model.TaskEventType) error {
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || event.JobId != claim.JobID || event.EventID < 0 || event.WorkerName != worker {
		return ErrStaleAssignment
	}
	switch event.NotificationType {
	case model.JobNotification:
		if event.Status != model.StartedNotificationStatus && event.Status != model.FailedNotificationStatus {
			return ErrEventConflict
		}
	case model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification, model.FFMPEGSNotification, model.JobVerify, model.UploadNotification:
		if event.Status != model.StartedNotificationStatus && event.Status != model.CompletedNotificationStatus && event.Status != model.FailedNotificationStatus {
			return ErrEventConflict
		}
	default:
		return ErrEventConflict
	}
	return nil
}

func protectedEventHash(event model.TaskEventType) ([sha256.Size]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// ApplyProtectedEvent fences and appends an event under the same row lock used
// by cancellation and parking. Identical committed retries are acknowledged.
// Completion is handled separately by the recoverable publication protocol.
func (s *SQLRepository) ApplyProtectedEvent(ctx context.Context, worker string, claim model.AssignmentClaim, event model.TaskEventType) error {
	if err := validateProtectedEvent(worker, claim, event); err != nil {
		return err
	}
	payloadHash, err := protectedEventHash(event)
	if err != nil {
		return err
	}
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, claim.JobID.String())
		if err != nil {
			return err
		}
		var owner, state string
		var generation int64
		var expected []byte
		err = conn.QueryRowContext(ctx, `SELECT worker_name,generation,state,capability_hash FROM protected_assignments WHERE job_id=$1`, claim.JobID.String()).Scan(&owner, &generation, &state, &expected)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAssignment
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(claim.Capability))
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || state == "revoked" {
			return ErrStaleAssignment
		}
		leaseErr := protectedLeaseValid(ctx, conn, claim, false)
		if leaseErr != nil && !errors.Is(leaseErr, ErrStaleAssignment) {
			return leaseErr
		}
		for _, existing := range job.Events {
			if existing.EventID == event.EventID {
				var saved []byte
				err := conn.QueryRowContext(ctx, `SELECT payload_hash FROM protected_events WHERE job_id=$1 AND generation=$2 AND event_id=$3`, claim.JobID.String(), generation, event.EventID).Scan(&saved)
				if err == nil && subtle.ConstantTimeCompare(saved, payloadHash[:]) == 1 {
					// A committed failure is terminal. Its exact acknowledgement can
					// be replayed after the process lease expires, but never for a
					// revoked or superseded assignment.
					if state == "completed" && event.NotificationType == model.JobNotification && event.Status == model.FailedNotificationStatus {
						return nil
					}
					return leaseErr
				}
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if leaseErr != nil {
					return leaseErr
				}
				return ErrEventConflict
			}
		}
		if leaseErr != nil {
			return leaseErr
		}
		if state != "assigned" && state != "running" {
			return ErrStaleAssignment
		}
		last := job.Events.GetLatest()
		if last == nil || event.EventID != last.EventID+1 {
			return ErrEventConflict
		}
		if err := tx.AddNewTaskEvent(ctx, &event); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO protected_events(job_id,generation,event_id,payload_hash) VALUES($1,$2,$3,$4)`, claim.JobID.String(), generation, event.EventID, payloadHash[:]); err != nil {
			return err
		}
		if event.NotificationType == model.JobNotification && event.Status == model.FailedNotificationStatus {
			if _, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='completed',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, claim.JobID.String()); err != nil {
				return err
			}
			_, err = conn.ExecContext(ctx, `UPDATE protected_publications SET state='abandoned' WHERE job_id=$1 AND generation=$2 AND state='staged'`, claim.JobID.String(), generation)
		} else if event.NotificationType == model.FFMPEGSNotification && event.Status == model.StartedNotificationStatus {
			_, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='running',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, claim.JobID.String())
		}
		return err
	})
}

func validateUploadMetadata(path, checksum string, size int64, stage func() error) error {
	decoded, err := hex.DecodeString(checksum)
	if !filepath.IsLocal(path) || size < 0 || err != nil || len(decoded) != sha256.Size || stage == nil {
		return errors.New("invalid protected upload metadata")
	}
	return nil
}

func isProtectedParkBoundary(events model.TaskEvents) bool {
	last := events.GetLatest()
	if last == nil || last.Status != model.CompletedNotificationStatus {
		return false
	}
	switch last.NotificationType {
	case model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification:
		return true
	default:
		return false
	}
}

// StageProtectedUpload rechecks ownership under the same row lock as
// cancellation, then publishes only an isolated generation-specific staging
// file. A crash before the database commit can leave an orphan, never a final
// output or an accepted object belonging to another generation.
func (s *SQLRepository) StageProtectedUpload(ctx context.Context, worker string, claim model.AssignmentClaim, path, checksum string, size int64, stage func() error) error {
	if err := validateUploadMetadata(path, checksum, size, stage); err != nil {
		return err
	}
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" {
		return ErrStaleAssignment
	}
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, claim.JobID.String())
		if err != nil {
			return err
		}
		var owner, state string
		var generation int64
		var expected []byte
		err = conn.QueryRowContext(ctx, `SELECT worker_name,generation,state,capability_hash FROM protected_assignments WHERE job_id=$1`, claim.JobID.String()).Scan(&owner, &generation, &state, &expected)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAssignment
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(claim.Capability))
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || (state != "assigned" && state != "running") || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
			return ErrStaleAssignment
		}
		if err := protectedLeaseValid(ctx, conn, claim, false); err != nil {
			return err
		}
		var savedPath, savedChecksum, savedState string
		var savedSize int64
		err = conn.QueryRowContext(ctx, `SELECT staging_path,checksum,size_bytes,state FROM protected_publications WHERE job_id=$1 AND generation=$2`, claim.JobID.String(), generation).Scan(&savedPath, &savedChecksum, &savedSize, &savedState)
		if err == nil && (savedPath != path || savedChecksum != checksum || savedSize != size || savedState != "staged") {
			return ErrEventConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := stage(); err != nil {
			return fmt.Errorf("stage protected upload: %w", err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			_, err = conn.ExecContext(ctx, `INSERT INTO protected_publications(job_id,generation,staging_path,checksum,size_bytes,state) VALUES($1,$2,$3,$4,$5,'staged')`, claim.JobID.String(), generation, path, checksum, size)
			return err
		}
		return nil
	})
}

// ParkProtectedJob is idempotent for a current generation. The caller must
// have durably saved and checked the prefetched workspace before parking.
func (s *SQLRepository) ParkProtectedJob(ctx context.Context, worker string, claim model.AssignmentClaim) error {
	if claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || worker == "" {
		return ErrStaleAssignment
	}
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, claim.JobID.String())
		if err != nil {
			return err
		}
		var owner, state string
		var generation int64
		var expected []byte
		err = conn.QueryRowContext(ctx, `SELECT worker_name,generation,state,capability_hash FROM protected_assignments WHERE job_id=$1`, claim.JobID.String()).Scan(&owner, &generation, &state, &expected)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAssignment
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(claim.Capability))
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || (state != "assigned" && state != "parked") || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
			return ErrStaleAssignment
		}
		if err := protectedLeaseValid(ctx, conn, claim, true); err != nil {
			return err
		}
		if !isProtectedParkBoundary(job.Events) {
			return ErrEventConflict
		}
		var staged bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM protected_publications WHERE job_id=$1 AND generation=$2 AND state='staged')`, claim.JobID.String(), generation).Scan(&staged); err != nil {
			return err
		}
		if staged {
			return ErrEventConflict
		}
		_, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='parked',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, claim.JobID.String())
		return err
	})
}

func (s *SQLRepository) GetProtectedJob(ctx context.Context, worker string, claim model.AssignmentClaim) (*model.Job, error) {
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" {
		return nil, ErrStaleAssignment
	}
	var result *model.Job
	err := s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, claim.JobID.String())
		if err != nil {
			return err
		}
		var owner, state string
		var generation int64
		var expected []byte
		err = conn.QueryRowContext(ctx, `SELECT worker_name,generation,state,capability_hash FROM protected_assignments WHERE job_id=$1`, claim.JobID.String()).Scan(&owner, &generation, &state, &expected)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAssignment
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(claim.Capability))
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || (state != "assigned" && state != "running") || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
			return ErrStaleAssignment
		}
		if err := protectedLeaseValid(ctx, conn, claim, false); err != nil {
			return err
		}
		result = job
		return nil
	})
	return result, err
}

// CancelJobAssignment serializes cancellation with parking and timeout. A
// publishing intent must be resolved before its generation can be revoked.
func (s *SQLRepository) CancelJobAssignment(ctx context.Context, id string) error {
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, id)
		if err != nil {
			return err
		}
		status := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if status == nil || (status.Status != model.AssignedNotificationStatus && status.Status != model.StartedNotificationStatus) {
			return fmt.Errorf("job %s cannot be canceled in its current state", id)
		}
		var protectedState string
		err = conn.QueryRowContext(ctx, `SELECT state FROM protected_assignments WHERE job_id=$1`, id).Scan(&protectedState)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if protectedState == "publishing" {
			return ErrJobPublishing
		}
		if protectedState != "" {
			if _, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='revoked',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, id); err != nil {
				return err
			}
			if _, err = conn.ExecContext(ctx, `UPDATE protected_publications SET state='abandoned' WHERE job_id=$1 AND state='staged'`, id); err != nil {
				return err
			}
		}
		return tx.AddNewTaskEvent(ctx, job.AddEventComplete(model.JobNotification, model.CanceledNotificationStatus, "Job canceled by user"))
	})
}

// RequeueTimedOutJob checks the exact event under the job lock. A stale
// maintenance scan must not revoke an assignment parked in the meantime.
func (s *SQLRepository) RequeueTimedOutJob(ctx context.Context, id string, eventID int, cutoff time.Time) (bool, error) {
	requeued := false
	err := s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, id)
		if err != nil {
			return err
		}
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if last == nil || last.EventID != eventID || !last.IsAssigned() || !last.EventTime.Before(cutoff) {
			return nil
		}
		var state, owner string
		var leaseValid bool
		err = conn.QueryRowContext(ctx, `SELECT state,worker_name,COALESCE(lease_expires_at > clock_timestamp(),false) FROM protected_assignments WHERE job_id=$1`, id).Scan(&state, &owner, &leaseValid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state == "publishing" || leaseValid || (state == "parked" && owner == last.WorkerName) {
			return nil
		}
		if state != "" {
			if _, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='revoked',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, id); err != nil {
				return err
			}
			if _, err = conn.ExecContext(ctx, `UPDATE protected_publications SET state='abandoned' WHERE job_id=$1 AND state='staged'`, id); err != nil {
				return err
			}
		}
		for _, event := range []*model.TaskEventType{job.AddEvent(model.JobNotification, model.CanceledNotificationStatus), job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)} {
			if err := tx.AddNewTaskEvent(ctx, event); err != nil {
				return err
			}
		}
		requeued = true
		return nil
	})
	return requeued, err
}
