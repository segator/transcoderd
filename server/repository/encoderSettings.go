package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"transcoder/model"
)

func (s *SQLRepository) GetWorkerSettings(ctx context.Context, name string) (*model.WorkerSettings, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	settings := &model.WorkerSettings{}
	err = db.QueryRowContext(ctx, "SELECT max_active_jobs,cpu_jobs,gpu_jobs FROM worker_settings WHERE worker_name=$1", name).
		Scan(&settings.MaxActiveJobs, &settings.CPUJobs, &settings.GPUJobs)
	if errors.Is(err, sql.ErrNoRows) {
		return &model.WorkerSettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *SQLRepository) SetWorkerSettings(ctx context.Context, name string, settings model.WorkerSettings) error {
	if name == "" {
		return fmt.Errorf("worker name is required")
	}
	if err := settings.ValidateWrite(); err != nil {
		return err
	}
	if !settings.Independent() {
		return s.SetWorkerMaxActiveJobs(ctx, name, *settings.MaxActiveJobs)
	}
	total, err := settings.Total()
	if err != nil {
		return err
	}
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO worker_settings (worker_name,max_active_jobs,cpu_jobs,gpu_jobs,updated_at)
 VALUES ($1,$2,$3,$4,CURRENT_TIMESTAMP) ON CONFLICT (worker_name) DO UPDATE SET
 max_active_jobs=$2,cpu_jobs=$3,gpu_jobs=$4,updated_at=CURRENT_TIMESTAMP`, name, total, *settings.CPUJobs, *settings.GPUJobs)
	return err
}

func (r *InMemoryRepository) GetWorkerSettings(ctx context.Context, name string) (*model.WorkerSettings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if pair, ok := r.workerEncoderLimits[name]; ok {
		clone := pair.Clone()
		return &clone, nil
	}
	if max, ok := r.workerMaxJobs[name]; ok {
		return &model.WorkerSettings{MaxActiveJobs: &max}, nil
	}
	return &model.WorkerSettings{}, nil
}

func (r *InMemoryRepository) SetWorkerSettings(ctx context.Context, name string, settings model.WorkerSettings) error {
	if name == "" {
		return fmt.Errorf("worker name is required")
	}
	if err := settings.ValidateWrite(); err != nil {
		return err
	}
	if !settings.Independent() {
		return r.SetWorkerMaxActiveJobs(ctx, name, *settings.MaxActiveJobs)
	}
	total, err := settings.Total()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := settings.Clone()
	clone.MaxActiveJobs = &total
	r.workerEncoderLimits[name] = clone
	r.workerMaxJobs[name] = total
	return nil
}
