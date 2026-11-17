package repository

import (
	"context"
	"errors"
	"fmt"

	"transcoder/model"
)

const maxWorkerAssignmentInventory = 4096

func (s *SQLRepository) ListWorkerAssignments(ctx context.Context, worker string) ([]model.WorkerAssignmentSnapshot, error) {
	if worker == "" {
		return nil, errors.New("worker identity is required")
	}
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT e.job_id,e.status,COALESCE(p.generation,0),
CASE WHEN p.job_id IS NULL THEN 'legacy' WHEN p.worker_name=e.worker_name THEN p.state ELSE 'inconsistent' END
FROM (SELECT DISTINCT ON (job_id) job_id,status,worker_name FROM job_events WHERE notification_type='Job' ORDER BY job_id,job_event_id DESC) e
LEFT JOIN protected_assignments p ON p.job_id=e.job_id
WHERE e.worker_name=$1 AND e.status IN ('assigned','started') ORDER BY e.job_id LIMIT $2`, worker, maxWorkerAssignmentInventory+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assignments := make([]model.WorkerAssignmentSnapshot, 0)
	for rows.Next() {
		var assignment model.WorkerAssignmentSnapshot
		var generation int64
		if err := rows.Scan(&assignment.JobID, &assignment.Status, &generation, &assignment.State); err != nil {
			return nil, err
		}
		if generation < 0 {
			return nil, errors.New("worker assignment has an invalid generation")
		}
		assignment.Generation = uint64(generation)
		assignments = append(assignments, assignment)
		if len(assignments) > maxWorkerAssignmentInventory {
			return nil, fmt.Errorf("worker assignment inventory exceeds %d entries", maxWorkerAssignmentInventory)
		}
	}
	return assignments, rows.Err()
}
