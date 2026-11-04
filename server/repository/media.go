package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"transcoder/model"

	"github.com/lib/pq"
)

func (s *SQLRepository) GetMediaProbe(ctx context.Context, root, path string, size, modified int64) (*model.MediaProbe, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT probe FROM media_metadata_cache WHERE root_key=$1 AND relative_path=$2 AND source_size=$3 AND modified_ns=$4`, root, path, size, modified).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var probe model.MediaProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	return &probe, nil
}

func (s *SQLRepository) PutMediaProbe(ctx context.Context, root, path string, size, modified int64, probe *model.MediaProbe) error {
	raw, err := json.Marshal(probe)
	if err != nil {
		return err
	}
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO media_metadata_cache(root_key,relative_path,source_size,modified_ns,probe)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(root_key,relative_path) DO UPDATE SET source_size=$3,modified_ns=$4,probe=$5,updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')`, root, path, size, modified, raw)
	return err
}

func (s *SQLRepository) GetMediaJobStates(ctx context.Context, paths []string) (map[string]model.MediaJobState, error) {
	result := make(map[string]model.MediaJobState)
	if len(paths) == 0 {
		return result, nil
	}
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT j.id,j.source_path,j.target_path,
 CASE WHEN s.notification_type='Job' AND s.status IN ('queued','completed','failed','canceled') THEN s.status ELSE 'active' END
 FROM jobs j JOIN job_status s ON s.job_id=j.id WHERE j.source_path=ANY($1) OR j.target_path=ANY($1) ORDER BY s.event_time`, pq.Array(paths))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		wanted[p] = true
	}
	for rows.Next() {
		var id, source, target, state string
		if err := rows.Scan(&id, &source, &target, &state); err != nil {
			return nil, err
		}
		if wanted[source] {
			result[source] = model.MediaJobState{ID: id, State: state, Role: "source"}
		}
		if wanted[target] && target != source && state == "completed" {
			if existing, ok := result[target]; !ok || existing.Role != "source" {
				result[target] = model.MediaJobState{ID: id, State: state, Role: "output"}
			}
		}
	}
	return result, rows.Err()
}

type cachedMediaProbe struct {
	size, modified int64
	raw            []byte
}

func (r *InMemoryRepository) GetMediaProbe(ctx context.Context, root, path string, size, modified int64) (*model.MediaProbe, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.mediaProbes[root+"\x00"+path]
	if !ok || v.size != size || v.modified != modified {
		return nil, nil
	}
	var probe model.MediaProbe
	if err := json.Unmarshal(v.raw, &probe); err != nil {
		return nil, err
	}
	return &probe, nil
}

func (r *InMemoryRepository) PutMediaProbe(ctx context.Context, root, path string, size, modified int64, probe *model.MediaProbe) error {
	raw, err := json.Marshal(probe)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mediaProbes[root+"\x00"+path] = cachedMediaProbe{size: size, modified: modified, raw: raw}
	return nil
}

func (r *InMemoryRepository) GetMediaJobStates(ctx context.Context, paths []string) (map[string]model.MediaJobState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wanted := make(map[string]bool, len(paths))
	for _, p := range paths {
		wanted[p] = true
	}
	result := make(map[string]model.MediaJobState)
	for _, j := range r.jobs {
		events := model.TaskEvents(r.events[j.Id.String()])
		last := events.GetLatest()
		if last == nil {
			continue
		}
		state := "active"
		if last.NotificationType == model.JobNotification {
			switch last.Status {
			case model.QueuedNotificationStatus, model.CompletedNotificationStatus, model.FailedNotificationStatus, model.CanceledNotificationStatus:
				state = string(last.Status)
			}
		}
		if wanted[j.SourcePath] {
			result[j.SourcePath] = model.MediaJobState{ID: j.Id.String(), State: state, Role: "source"}
		}
		if wanted[j.TargetPath] && j.TargetPath != j.SourcePath && state == "completed" {
			if existing, ok := result[j.TargetPath]; !ok || existing.Role != "source" {
				result[j.TargetPath] = model.MediaJobState{ID: j.Id.String(), State: state, Role: "output"}
			}
		}
	}
	return result, nil
}
