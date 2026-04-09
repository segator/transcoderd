//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"transcoder/model"
	"transcoder/server/repository"
)

// inMemoryRepository implementa repository.Repository en memoria para tests
type inMemoryRepository struct {
	mu             sync.RWMutex
	jobs           map[string]*model.Job
	events         map[string][]*model.TaskEventType
	progress       map[string]*model.TaskProgressType
	workers        map[string]*model.Worker
	jobsByPath     map[string]*model.Job
	transactionCtx context.Context
}

func newInMemoryRepository() *inMemoryRepository {
	return &inMemoryRepository{
		jobs:       make(map[string]*model.Job),
		events:     make(map[string][]*model.TaskEventType),
		progress:   make(map[string]*model.TaskProgressType),
		workers:    make(map[string]*model.Worker),
		jobsByPath: make(map[string]*model.Job),
	}
}

func (r *inMemoryRepository) AddJob(ctx context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.jobs[job.Id.String()] = job
	r.jobsByPath[job.SourcePath] = job
	return nil
}

func (r *inMemoryRepository) GetJob(ctx context.Context, id string) (*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[id]
	if !exists {
		return nil, repository.ErrElementNotFound
	}

	// Cargar eventos
	events := r.events[id]
	jobCopy := *job
	jobCopy.Events = make([]*model.TaskEventType, len(events))
	copy(jobCopy.Events, events)

	return &jobCopy, nil
}

func (r *inMemoryRepository) GetJobByPath(ctx context.Context, path string) (*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobsByPath[path]
	if !exists {
		return nil, nil
	}

	// Cargar eventos
	events := r.events[job.Id.String()]
	jobCopy := *job
	jobCopy.Events = make([]*model.TaskEventType, len(events))
	copy(jobCopy.Events, events)

	return &jobCopy, nil
}

func (r *inMemoryRepository) AddNewTaskEvent(ctx context.Context, event *model.TaskEventType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobId := event.JobId.String()
	r.events[jobId] = append(r.events[jobId], event)

	// Actualizar el job con el último estado
	if job, exists := r.jobs[jobId]; exists {
		job.Status = string(event.Status)
		job.StatusPhase = event.NotificationType
		job.StatusMessage = event.Message
		now := time.Now()
		job.LastUpdate = &now
	}

	return nil
}

func (r *inMemoryRepository) UpdateJob(ctx context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[job.Id.String()]; !exists {
		return repository.ErrElementNotFound
	}

	r.jobs[job.Id.String()] = job
	return nil
}

func (r *inMemoryRepository) RetrieveQueuedJob(ctx context.Context) (*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, job := range r.jobs {
		events := r.events[job.Id.String()]
		if len(events) == 0 {
			continue
		}

		lastEvent := events[len(events)-1]
		if lastEvent.NotificationType == model.JobNotification &&
			lastEvent.Status == model.QueuedNotificationStatus {

			jobCopy := *job
			jobCopy.Events = make([]*model.TaskEventType, len(events))
			copy(jobCopy.Events, events)
			return &jobCopy, nil
		}
	}

	return nil, repository.ErrElementNotFound
}

func (r *inMemoryRepository) GetJobsByStatus(ctx context.Context, notificationType model.NotificationType, status model.NotificationStatus) ([]*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Job
	for _, job := range r.jobs {
		events := r.events[job.Id.String()]
		if len(events) == 0 {
			continue
		}

		lastEvent := events[len(events)-1]
		if lastEvent.NotificationType == notificationType && lastEvent.Status == status {
			jobCopy := *job
			jobCopy.Events = make([]*model.TaskEventType, len(events))
			copy(jobCopy.Events, events)
			result = append(result, &jobCopy)
		}
	}

	return result, nil
}

func (r *inMemoryRepository) ProgressJob(ctx context.Context, progress *model.TaskProgressType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s-%s", progress.JobId.String(), progress.NotificationType)
	r.progress[key] = progress
	return nil
}

func (r *inMemoryRepository) DeleteProgressJob(ctx context.Context, progressID string, notificationType model.NotificationType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.progress, progressID)
	return nil
}

func (r *inMemoryRepository) GetAllProgressJobs(ctx context.Context) ([]model.TaskProgressType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []model.TaskProgressType
	for _, p := range r.progress {
		result = append(result, *p)
	}
	return result, nil
}

func (r *inMemoryRepository) GetTimeoutJobs(ctx context.Context, timeout time.Duration) ([]*model.TaskEventType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.TaskEventType
	cutoff := time.Now().Add(-timeout)

	for _, events := range r.events {
		if len(events) == 0 {
			continue
		}

		lastEvent := events[len(events)-1]
		if lastEvent.EventTime.Before(cutoff) &&
			(lastEvent.Status == model.AssignedNotificationStatus ||
				lastEvent.Status == model.StartedNotificationStatus) {
			result = append(result, lastEvent)
		}
	}

	return result, nil
}

func (r *inMemoryRepository) PingServerUpdate(ctx context.Context, remoteAddr string, ping model.PingEventType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.workers[ping.WorkerName] = &model.Worker{
		Name:     ping.WorkerName,
		Ip:       remoteAddr,
		LastSeen: ping.EventTime,
	}
	return nil
}

func (r *inMemoryRepository) WithTransaction(ctx context.Context, fn func(context.Context, repository.Repository) error) error {
	// Para el test en memoria, simplemente ejecutamos la función
	return fn(ctx, r)
}

func (r *inMemoryRepository) Close() error {
	return nil
}

func (r *inMemoryRepository) Initialize(ctx context.Context) error {
	// Mock repository ya está inicializado
	return nil
}

func (r *inMemoryRepository) getConnection() (repository.SQLDBOperations, error) {
	// No implementado para mock en memoria
	return nil, fmt.Errorf("getConnection not implemented in mock")
}

func (r *inMemoryRepository) GetJobs(ctx context.Context) (*[]model.Job, error) {
	jobs, err := r.GetAllJobs(ctx)
	if err != nil {
		return nil, err
	}
	jobSlice := make([]model.Job, len(jobs))
	for i, job := range jobs {
		jobSlice[i] = *job
	}
	return &jobSlice, nil
}

func (r *inMemoryRepository) GetWorker(ctx context.Context, name string) (*model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, exists := r.workers[name]
	if !exists {
		return nil, repository.ErrElementNotFound
	}
	return worker, nil
}

// Métodos no implementados pero necesarios para la interfaz
func (r *inMemoryRepository) GetAllJobs(ctx context.Context) ([]*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Job
	for _, job := range r.jobs {
		events := r.events[job.Id.String()]
		jobCopy := *job
		jobCopy.Events = make([]*model.TaskEventType, len(events))
		copy(jobCopy.Events, events)
		result = append(result, &jobCopy)
	}
	return result, nil
}

func (r *inMemoryRepository) GetJobEvents(ctx context.Context, jobId string) ([]*model.TaskEventType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	events := r.events[jobId]
	result := make([]*model.TaskEventType, len(events))
	copy(result, events)
	return result, nil
}

func (r *inMemoryRepository) GetAllWorkers(ctx context.Context) ([]*model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Worker
	for _, worker := range r.workers {
		result = append(result, worker)
	}
	return result, nil
}
