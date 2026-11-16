package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

func (s *SQLRepository) BeginProtectedPublication(ctx context.Context, worker string, claim model.AssignmentClaim, event model.TaskEventType, deleteSource bool) (*model.ProtectedPublication, error) {
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || event.JobId != claim.JobID || event.WorkerName != worker || event.NotificationType != model.JobNotification || event.Status != model.CompletedNotificationStatus || event.EventID < 0 {
		return nil, ErrStaleAssignment
	}
	payloadHash, err := protectedEventHash(event)
	if err != nil {
		return nil, err
	}
	var publication *model.ProtectedPublication
	err = s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
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
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || state == "revoked" || state == "parked" {
			return ErrStaleAssignment
		}
		publication = &model.ProtectedPublication{JobID: claim.JobID, Generation: claim.Generation, WorkerName: worker}
		var completionID sql.NullInt64
		var message, finalPath sql.NullString
		var savedHash []byte
		err = conn.QueryRowContext(ctx, `SELECT staging_path,checksum,size_bytes,state,completion_event_id,completion_message,final_path,delete_source_pending,cleanup_pending,completion_hash
FROM protected_publications WHERE job_id=$1 AND generation=$2`, claim.JobID.String(), generation).
			Scan(&publication.StagingPath, &publication.Checksum, &publication.SizeBytes, &publication.State, &completionID, &message, &finalPath, &publication.DeleteSource, &publication.CleanupPending, &savedHash)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEventConflict
		}
		if err != nil {
			return err
		}
		publication.EventID, publication.Message, publication.FinalPath = int(completionID.Int64), message.String, finalPath.String
		if state == "publishing" || state == "completed" {
			if !completionID.Valid || publication.EventID != event.EventID || publication.Message != event.Message || publication.State != state || subtle.ConstantTimeCompare(savedHash, payloadHash[:]) != 1 {
				return ErrEventConflict
			}
			return nil
		}
		if err := protectedLeaseValid(ctx, conn, claim, false); err != nil {
			return err
		}
		lastJob := job.Events.GetLatestPerNotificationType(model.JobNotification)
		lastEvent := job.Events.GetLatest()
		if (state != "assigned" && state != "running") || publication.State != "staged" || lastJob == nil || lastJob.WorkerName != worker || lastJob.Status != model.StartedNotificationStatus || lastEvent == nil || lastEvent.EventID+1 != event.EventID {
			return ErrEventConflict
		}
		if _, err = conn.ExecContext(ctx, `UPDATE protected_publications SET state='publishing',completion_event_id=$3,completion_message=$4,delete_source_pending=$5,completion_hash=$6 WHERE job_id=$1 AND generation=$2`, claim.JobID.String(), generation, event.EventID, event.Message, deleteSource, payloadHash[:]); err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='publishing',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, claim.JobID.String()); err != nil {
			return err
		}
		publication.State, publication.EventID, publication.Message, publication.DeleteSource = "publishing", event.EventID, event.Message, deleteSource
		return nil
	})
	return publication, err
}

func (s *SQLRepository) FinishProtectedPublication(ctx context.Context, publication model.ProtectedPublication, finalPath string, size int64, probe *model.MediaProbe) error {
	if publication.JobID == uuid.Nil || publication.Generation == 0 || !filepath.IsLocal(finalPath) || size < 0 || probe == nil {
		return errors.New("invalid protected publication result")
	}
	return s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, publication.JobID.String())
		if err != nil {
			return err
		}
		var state, owner string
		var generation int64
		err = conn.QueryRowContext(ctx, `SELECT state,worker_name,generation FROM protected_assignments WHERE job_id=$1`, publication.JobID.String()).Scan(&state, &owner, &generation)
		if err != nil {
			return err
		}
		if generation < 1 || uint64(generation) != publication.Generation {
			return ErrStaleAssignment
		}
		var pubState, message, stagingPath, checksum string
		var eventID int
		var stagedSize int64
		var savedFinal sql.NullString
		err = conn.QueryRowContext(ctx, `SELECT state,completion_event_id,completion_message,final_path,staging_path,checksum,size_bytes FROM protected_publications WHERE job_id=$1 AND generation=$2`, publication.JobID.String(), generation).Scan(&pubState, &eventID, &message, &savedFinal, &stagingPath, &checksum, &stagedSize)
		if err != nil {
			return err
		}
		if eventID != publication.EventID || message != publication.Message || owner != publication.WorkerName || stagingPath != publication.StagingPath || checksum != publication.Checksum || stagedSize != publication.SizeBytes {
			return ErrEventConflict
		}
		if state == "completed" && pubState == "completed" {
			if !savedFinal.Valid || savedFinal.String != finalPath {
				return ErrEventConflict
			}
			return nil
		}
		if state != "publishing" || pubState != "publishing" {
			return ErrStaleAssignment
		}
		last := job.Events.GetLatest()
		if last == nil || last.EventID+1 != eventID {
			return ErrEventConflict
		}
		job.TargetPath, job.TargetSize, job.TargetProbe = finalPath, size, probe
		if err := tx.UpdateJob(ctx, job); err != nil {
			return err
		}
		event := &model.TaskEventType{Event: model.Event{WorkerName: owner, EventTime: time.Now().UTC()}, JobId: publication.JobID, EventID: eventID, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus, Message: message}
		if err := tx.AddNewTaskEvent(ctx, event); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE protected_publications SET state='completed',final_path=$3 WHERE job_id=$1 AND generation=$2`, publication.JobID.String(), generation, finalPath); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE protected_assignments SET state='completed',updated_at=CURRENT_TIMESTAMP WHERE job_id=$1`, publication.JobID.String())
		return err
	})
}

func (s *SQLRepository) ListPendingProtectedPublications(ctx context.Context) ([]model.ProtectedPublication, error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT p.job_id,p.generation,a.worker_name,p.staging_path,p.checksum,p.size_bytes,COALESCE(p.completion_event_id,-1),COALESCE(p.completion_message,''),p.state,COALESCE(p.final_path,''),p.delete_source_pending,p.cleanup_pending
FROM protected_publications p JOIN protected_assignments a ON a.job_id=p.job_id AND a.generation=p.generation
WHERE p.state='publishing' OR (p.state IN ('completed','abandoned') AND (p.delete_source_pending OR p.cleanup_pending))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var publications []model.ProtectedPublication
	for rows.Next() {
		var p model.ProtectedPublication
		var generation int64
		if err := rows.Scan(&p.JobID, &generation, &p.WorkerName, &p.StagingPath, &p.Checksum, &p.SizeBytes, &p.EventID, &p.Message, &p.State, &p.FinalPath, &p.DeleteSource, &p.CleanupPending); err != nil {
			return nil, err
		}
		if generation <= 0 {
			return nil, fmt.Errorf("invalid publication generation for %s", p.JobID)
		}
		p.Generation = uint64(generation)
		publications = append(publications, p)
	}
	return publications, rows.Err()
}

func (s *SQLRepository) ClearProtectedSourceDeletion(ctx context.Context, id string, generation uint64) error {
	conn, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `UPDATE protected_publications SET delete_source_pending=false,cleanup_pending=false WHERE job_id=$1 AND generation=$2 AND state IN ('completed','abandoned')`, id, generation)
	return err
}
