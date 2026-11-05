package repository

import (
	"context"
	"database/sql"
	"errors"

	"transcoder/model"
)

func (s *SQLRepository) GetWorkerResumeSettings(ctx context.Context, name string) (*model.WorkerSettings, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	settings := &model.WorkerSettings{}
	err = db.QueryRowContext(ctx, `SELECT resume_max_active_jobs,resume_cpu_jobs,resume_gpu_jobs FROM worker_settings WHERE worker_name=$1`, name).Scan(&settings.MaxActiveJobs, &settings.CPUJobs, &settings.GPUJobs)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	return settings, err
}
func (s *SQLRepository) GetWorkerControl(ctx context.Context, name string) (*model.WorkerControl, error) {
	db, err := s.getConnection()
	if err != nil {
		return nil, err
	}
	c := &model.WorkerControl{}
	err = db.QueryRowContext(ctx, `SELECT id,action,state,error,cpu_jobs,gpu_jobs,max_active_jobs FROM worker_controls WHERE worker_name=$1`, name).Scan(&c.ID, &c.Action, &c.State, &c.Error, &c.CPUJobs, &c.GPUJobs, &c.MaxActiveJobs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}
func (s *SQLRepository) SetWorkerControl(ctx context.Context, name string, c model.WorkerControl) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO worker_controls(worker_name,id,action,state,error,cpu_jobs,gpu_jobs,max_active_jobs) VALUES($1,$2,$3,'pending','',$4,$5,$6)
 ON CONFLICT(worker_name) DO UPDATE SET id=$2,action=$3,state='pending',error='',cpu_jobs=$4,gpu_jobs=$5,max_active_jobs=$6`, name, c.ID, c.Action, c.CPUJobs, c.GPUJobs, c.MaxActiveJobs)
	return err
}
func (s *SQLRepository) AckWorkerControl(ctx context.Context, name string, ack model.WorkerControlAck) error {
	db, err := s.getConnection()
	if err != nil {
		return err
	}
	state := "applied"
	if ack.Error != "" {
		state = "failed"
	}
	result, err := db.ExecContext(ctx, `UPDATE worker_controls SET state=$3,error=$4 WHERE worker_name=$1 AND id=$2`, name, ack.ID, state, ack.Error)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrElementNotFound
	}
	return err
}
func (r *InMemoryRepository) GetWorkerResumeSettings(_ context.Context, name string) (*model.WorkerSettings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.workerResumeLimits[name].Clone()
	return &v, nil
}
func (r *InMemoryRepository) GetWorkerControl(_ context.Context, name string) (*model.WorkerControl, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.workerCommands[name]
	if !ok {
		return nil, nil
	}
	c := v.Clone()
	return &c, nil
}
func (r *InMemoryRepository) SetWorkerControl(_ context.Context, name string, c model.WorkerControl) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.State = "pending"
	c.Error = ""
	r.workerCommands[name] = c.Clone()
	return nil
}
func (r *InMemoryRepository) AckWorkerControl(_ context.Context, name string, ack model.WorkerControlAck) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.workerCommands[name]
	if !ok || c.ID != ack.ID {
		return ErrElementNotFound
	}
	c.State = "applied"
	if ack.Error != "" {
		c.State = "failed"
	}
	c.Error = ack.Error
	r.workerCommands[name] = c
	return nil
}
