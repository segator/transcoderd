package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"transcoder/model"

	"github.com/google/uuid"
)

func protectedProgressHash(worker string, claim model.AssignmentClaim, progress model.TaskProgressType, sequence uint64) ([sha256.Size]byte, error) {
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || claim.InstanceID == uuid.Nil || progress.JobId != claim.JobID || progress.WorkerName != worker || len(progress.ProgressID) == 0 || len(progress.ProgressID) > 255 || sequence == 0 || sequence > math.MaxInt64 || math.IsNaN(progress.Percent) || math.IsInf(progress.Percent, 0) || progress.Percent < 0 || progress.Percent > 100 || progress.ETA < 0 {
		return [sha256.Size]byte{}, ErrEventConflict
	}
	switch progress.NotificationType {
	case model.DownloadNotification, model.MKVExtractNotification, model.PGSNotification, model.FFMPEGSNotification, model.JobVerify, model.UploadNotification:
	default:
		return [sha256.Size]byte{}, ErrEventConflict
	}
	if progress.Status != model.ProgressingTaskProgressTypeStatus && progress.Status != model.DoneTaskProgressTypeStatus && progress.Status != model.FailureTaskProgressTypeStatus {
		return [sha256.Size]byte{}, ErrEventConflict
	}
	data, err := json.Marshal(progress)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

// ApplyProtectedProgress verifies the assignment under the job lock and uses
// a per-progress sequence to reject delayed or conflicting updates.
func (s *SQLRepository) ApplyProtectedProgress(ctx context.Context, worker string, claim model.AssignmentClaim, progress model.TaskProgressType, sequence uint64) error {
	hash, err := protectedProgressHash(worker, claim, progress, sequence)
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
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || (state != "assigned" && state != "running") || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || last == nil || last.WorkerName != worker || last.Status != model.StartedNotificationStatus {
			return ErrStaleAssignment
		}
		if err := protectedLeaseValid(ctx, conn, claim, false); err != nil {
			return err
		}
		var previous int64
		var saved []byte
		err = conn.QueryRowContext(ctx, `SELECT sequence,payload_hash FROM protected_progress WHERE job_id=$1 AND generation=$2 AND progress_id=$3 AND notification_type=$4`, claim.JobID.String(), generation, progress.ProgressID, progress.NotificationType).Scan(&previous, &saved)
		if err == nil {
			if uint64(previous) == sequence && subtle.ConstantTimeCompare(saved, hash[:]) == 1 {
				return nil
			}
			if uint64(previous) >= sequence {
				return ErrEventConflict
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if progress.Status == model.ProgressingTaskProgressTypeStatus {
			result, err := conn.ExecContext(ctx, `INSERT INTO job_progress(progress_id,notification_type,job_id,worker_name,percent,eta,last_update)
VALUES($1,$2,$3,$4,$5,$6,CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
ON CONFLICT(progress_id,notification_type) DO UPDATE SET percent=EXCLUDED.percent,eta=EXCLUDED.eta,last_update=EXCLUDED.last_update
WHERE job_progress.job_id=EXCLUDED.job_id AND job_progress.worker_name=EXCLUDED.worker_name`, progress.ProgressID, progress.NotificationType, progress.JobId.String(), worker, progress.Percent, progress.ETA.Seconds())
			if err != nil {
				return err
			}
			rows, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if rows != 1 {
				return ErrEventConflict
			}
		} else if _, err := conn.ExecContext(ctx, `DELETE FROM job_progress WHERE progress_id=$1 AND notification_type=$2 AND job_id=$3 AND worker_name=$4`, progress.ProgressID, progress.NotificationType, claim.JobID.String(), worker); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO protected_progress(job_id,generation,progress_id,notification_type,sequence,payload_hash) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(job_id,generation,progress_id,notification_type) DO UPDATE SET sequence=EXCLUDED.sequence,payload_hash=EXCLUDED.payload_hash`, claim.JobID.String(), generation, progress.ProgressID, progress.NotificationType, int64(sequence), hash[:])
		return err
	})
}
