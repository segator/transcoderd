package worker

import (
	"fmt"
	"sort"
	"sync"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/control"
	"transcoder/worker/job"
	"transcoder/worker/step"

	log "github.com/sirupsen/logrus"
)

type jobRegistry struct {
	mu    sync.RWMutex
	items map[string]control.JobState
	codec *config.FFMPEGConfig
}

func encoderProfile(codec string) string {
	if codec == config.AV1NVENC {
		return "gpu"
	}
	if codec != "" {
		return "cpu"
	}
	return ""
}

func (r *jobRegistry) set(jobContext *job.Context, phase model.NotificationType, state string) {
	codec := ""
	if r.codec != nil {
		codec = r.codec.VideoCodec
	}
	if jobContext.EncodeConfig != nil {
		codec = jobContext.EncodeConfig.VideoCodec
	}
	item := control.JobState{JobID: jobContext.JobId.String(), Phase: string(phase), State: state, Codec: codec, Profile: encoderProfile(codec)}
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
func (r *jobRegistry) get(id string) (control.JobState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return item, ok
}
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
		executor.OnAdd = func(j *job.Context) {
			j.NextPhase = phase
			if err := j.PersistJobContext(); err != nil {
				log.WithError(err).WithField("job_id", j.JobId.String()).Error("Failed to persist next job phase")
				e.recordRecoveryError(fmt.Errorf("persist next phase for job %s: %w", j.JobId, err), false)
			}
			e.admissionMu.Lock()
			e.jobs.set(j, phase, "queued")
			e.admissionMu.Unlock()
			oldAdd(j)
		}
		executor.OnJob = func(j *job.Context) {
			e.admissionMu.Lock()
			if phase == model.FFMPEGSNotification {
				delete(e.reclaimedJobs, j.JobId)
			}
			e.jobs.set(j, phase, "running")
			e.admissionMu.Unlock()
			oldJob(j)
		}
	}
}
