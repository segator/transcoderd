package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"transcoder/model"
)

// InMemoryRepository implementa Repository en memoria para tests
type InMemoryRepository struct {
	mu             sync.RWMutex
	jobs           map[string]*model.Job
	events         map[string][]*model.TaskEventType
	progress       map[string]*model.TaskProgressType
	workers        map[string]*model.Worker
	jobsByPath     map[string]*model.Job
	transactionCtx context.Context
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		jobs:       make(map[string]*model.Job),
		events:     make(map[string][]*model.TaskEventType),
		progress:   make(map[string]*model.TaskProgressType),
		workers:    make(map[string]*model.Worker),
		jobsByPath: make(map[string]*model.Job),
	}
}

func (r *InMemoryRepository) getConnection() (SQLDBOperations, error) {
	// No implementado para mock en memoria
	return nil, fmt.Errorf("getConnection not implemented in mock")
}

func (r *InMemoryRepository) Initialize(ctx context.Context) error {
	// Mock repository ya está inicializado
	return nil
}

func (r *InMemoryRepository) AddJob(ctx context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.jobs[job.Id.String()] = job
	r.jobsByPath[job.SourcePath] = job
	return nil
}

func (r *InMemoryRepository) GetJob(ctx context.Context, id string) (*model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	job, exists := r.jobs[id]
	if !exists {
		return nil, ErrElementNotFound
	}

	// Cargar eventos
	events := r.events[id]
	jobCopy := *job
	jobCopy.Events = make([]*model.TaskEventType, len(events))
	copy(jobCopy.Events, events)

	return &jobCopy, nil
}

func (r *InMemoryRepository) GetJobByPath(ctx context.Context, path string) (*model.Job, error) {
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

func (r *InMemoryRepository) AddNewTaskEvent(ctx context.Context, event *model.TaskEventType) error {
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

func (r *InMemoryRepository) UpdateJob(ctx context.Context, job *model.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[job.Id.String()]; !exists {
		return ErrElementNotFound
	}

	r.jobs[job.Id.String()] = job
	return nil
}

func (r *InMemoryRepository) RetrieveQueuedJob(ctx context.Context) (*model.Job, error) {
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

	return nil, ErrElementNotFound
}

func (r *InMemoryRepository) GetJobsByStatus(ctx context.Context, notificationType model.NotificationType, status model.NotificationStatus) ([]*model.Job, error) {
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

func (r *InMemoryRepository) ProgressJob(ctx context.Context, progress *model.TaskProgressType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s-%s", progress.JobId.String(), progress.NotificationType)
	r.progress[key] = progress
	return nil
}

func (r *InMemoryRepository) DeleteProgressJob(ctx context.Context, progressID string, notificationType model.NotificationType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.progress, progressID)
	return nil
}

func (r *InMemoryRepository) GetAllProgressJobs(ctx context.Context) ([]model.TaskProgressType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []model.TaskProgressType
	for _, p := range r.progress {
		result = append(result, *p)
	}
	return result, nil
}

func (r *InMemoryRepository) GetTimeoutJobs(ctx context.Context, timeout time.Duration) ([]*model.TaskEventType, error) {
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

func (r *InMemoryRepository) PingServerUpdate(ctx context.Context, remoteAddr string, ping model.PingEventType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.workers[ping.WorkerName] = &model.Worker{
		Name:     ping.WorkerName,
		Ip:       remoteAddr,
		LastSeen: ping.EventTime,
	}
	return nil
}

func (r *InMemoryRepository) WithTransaction(ctx context.Context, fn func(context.Context, Repository) error) error {
	// Para el test en memoria, simplemente ejecutamos la función
	return fn(ctx, r)
}

func (r *InMemoryRepository) GetJobs(ctx context.Context) (*[]model.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	jobs := make([]model.Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		events := r.events[job.Id.String()]
		jobCopy := *job
		jobCopy.Events = make([]*model.TaskEventType, len(events))
		copy(jobCopy.Events, events)
		jobs = append(jobs, jobCopy)
	}
	return &jobs, nil
}

func (r *InMemoryRepository) GetWorker(ctx context.Context, name string) (*model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	worker, exists := r.workers[name]
	if !exists {
		return nil, ErrElementNotFound
	}
	return worker, nil
}
