package worker

import (
	"sort"
	"sync"

	"transcoder/model"
	"transcoder/worker/control"
	"transcoder/worker/job"
	"transcoder/worker/step"
)

type jobRegistry struct {
	mu    sync.RWMutex
	items map[string]control.JobState
}

func (r *jobRegistry) set(jobContext *job.Context, phase model.NotificationType, state string) {
	item := control.JobState{JobID: jobContext.JobId.String(), Phase: string(phase), State: state}
	if jobContext.Source != nil {
		item.Name = jobContext.Source.OriginalName
		item.Downloaded = jobContext.Source.FilePath != ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = make(map[string]control.JobState)
	}
	r.items[item.JobID] = item
}

func (r *jobRegistry) remove(id string) { r.mu.Lock(); defer r.mu.Unlock(); delete(r.items, id) }
func (r *jobRegistry) snapshot() []control.JobState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]control.JobState, 0, len(r.items))
	for _, v := range r.items {
		items = append(items, v)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].JobID < items[j].JobID })
	return items
}

// Track both channel-buffered work and the single encode-queue pending item.
func (e *JobExecutor) trackPipeline(executors map[model.NotificationType]*step.Executor) {
	for phase, executor := range executors {
		oldAdd, oldJob := executor.OnAdd, executor.OnJob
		executor.OnAdd = func(j *job.Context) { e.jobs.set(j, phase, "queued"); oldAdd(j) }
		executor.OnJob = func(j *job.Context) { e.jobs.set(j, phase, "running"); oldJob(j) }
	}
}
