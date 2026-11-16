package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

// Bound leases so a lost worker cannot monopolize an assignment indefinitely
// and a transient slow response cannot produce an unusably short lease.
func validateLeaseRequest(worker string, claim model.AssignmentClaim, instance uuid.UUID, duration time.Duration) error {
	if worker == "" || claim.JobID == uuid.Nil || claim.Generation == 0 || claim.Capability == "" || instance == uuid.Nil || duration < 10*time.Second || duration > 5*time.Minute || duration%time.Second != 0 {
		return ErrStaleAssignment
	}
	return nil
}

// AcquireProtectedLease serializes worker-process ownership with cancellation,
// parking and reassignment. Deadlines are based on the database clock, shared
// by all server replicas. Callers must stop locally before their lease expires.
func (s *SQLRepository) AcquireProtectedLease(ctx context.Context, worker string, claim model.AssignmentClaim, instance uuid.UUID, duration time.Duration) (time.Time, error) {
	if err := validateLeaseRequest(worker, claim, instance, duration); err != nil {
		return time.Time{}, err
	}
	var expiry time.Time
	err := s.WithTransaction(ctx, func(ctx context.Context, tx Repository) error {
		conn, job, err := lockJob(ctx, tx, claim.JobID.String())
		if err != nil {
			return err
		}
		var owner, state string
		var generation int64
		var expected []byte
		var leaseInstance sql.NullString
		var leaseExpiry sql.NullTime
		var now time.Time
		err = conn.QueryRowContext(ctx, `SELECT worker_name,generation,state,capability_hash,lease_instance::text,lease_expires_at,clock_timestamp() FROM protected_assignments WHERE job_id=$1`, claim.JobID.String()).Scan(&owner, &generation, &state, &expected, &leaseInstance, &leaseExpiry, &now)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrStaleAssignment
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(claim.Capability))
		last := job.Events.GetLatestPerNotificationType(model.JobNotification)
		if owner != worker || generation < 1 || uint64(generation) != claim.Generation || subtle.ConstantTimeCompare(expected, provided[:]) != 1 || (state != "assigned" && state != "running" && state != "parked" && state != "publishing") || last == nil || last.WorkerName != worker || (last.Status != model.AssignedNotificationStatus && last.Status != model.StartedNotificationStatus) {
			return ErrStaleAssignment
		}
		// Publication owns its generation. Only the process that entered it may
		// renew a still-live lease; no new process may take over publication.
		if state == "publishing" && (!leaseInstance.Valid || leaseInstance.String != instance.String() || !leaseExpiry.Valid || !leaseExpiry.Time.After(now)) {
			return ErrLeaseHeld
		}
		if state != "parked" && leaseInstance.Valid && leaseExpiry.Valid && leaseExpiry.Time.After(now) && leaseInstance.String != instance.String() {
			return ErrLeaseHeld
		}
		return conn.QueryRowContext(ctx, `UPDATE protected_assignments SET lease_instance=$2,lease_expires_at=clock_timestamp()+($3 * interval '1 second'),state=CASE WHEN state='parked' THEN 'assigned' ELSE state END,updated_at=CURRENT_TIMESTAMP WHERE job_id=$1 RETURNING lease_expires_at`, claim.JobID.String(), instance.String(), int64(duration/time.Second)).Scan(&expiry)
	})
	return expiry, err
}

// protectedLeaseValid is called while the assignment's job row is locked.
// A parked worker may retry its parking acknowledgement after expiry, provided
// no other process has reclaimed it in the meantime.
func protectedLeaseValid(ctx context.Context, conn SQLDBOperations, claim model.AssignmentClaim, parkedAck bool) error {
	if claim.InstanceID == uuid.Nil {
		return ErrStaleAssignment
	}
	var valid bool
	err := conn.QueryRowContext(ctx, `SELECT COALESCE(lease_instance=$2 AND (lease_expires_at > clock_timestamp() OR ($3 AND state='parked')),false) FROM protected_assignments WHERE job_id=$1`, claim.JobID.String(), claim.InstanceID.String(), parkedAck).Scan(&valid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleAssignment
	}
	if err != nil {
		return err
	}
	if !valid {
		return ErrStaleAssignment
	}
	return nil
}
