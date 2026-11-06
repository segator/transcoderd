package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"transcoder/model"
)

func (r *InMemoryRepository) GetDashboard(ctx context.Context, hours int, now time.Time, ranges ...model.DashboardRange) (*model.Dashboard, error) {
	return r.dashboard(ctx, hours, now, nil, ranges...)
}

func (r *InMemoryRepository) dashboard(ctx context.Context, hours int, now time.Time, query *model.DashboardJobsQuery, ranges ...model.DashboardRange) (*model.Dashboard, error) {
	period, err := model.ResolveDashboardRange(hours, now, ranges...)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d := model.NewDashboard(now, hours)
	d.Range = period
	since, until, step := period.Start, period.End, period.Step()
	for t := since.UTC().Truncate(step); t.Before(until); t = t.Add(step) {
		d.Timeline = append(d.Timeline, model.DashboardBucket{Time: t})
	}
	workers := map[string]*model.DashboardWorker{}
	worker := func(name string) *model.DashboardWorker {
		if workers[name] == nil {
			workers[name] = &model.DashboardWorker{Name: name}
		}
		return workers[name]
	}
	for name, w := range r.workers {
		v := worker(name)
		v.IP, v.LastSeen, v.Runtime = w.Ip, &w.LastSeen, w.Runtime
	}
	for name, max := range r.workerMaxJobs {
		worker(name).DesiredMax = &max
	}
	for name, settings := range r.workerEncoderLimits {
		pair := settings.Clone()
		worker(name).DesiredCPUJobs, worker(name).DesiredGPUJobs = pair.CPUJobs, pair.GPUJobs
	}
	codecs := map[string]*model.DashboardCodec{}
	var durationSum float64
	var durationCount int
	for id, j := range r.jobs {
		events := model.TaskEvents(r.events[id])
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
		d.Summary.Total++
		switch state {
		case "queued":
			d.Summary.Queued++
		case "active":
			d.Summary.Active++
		case "completed":
			d.Summary.Completed++
		case "failed":
			d.Summary.Failed++
		case "canceled":
			d.Summary.Canceled++
		}
		valid := state == "completed" && j.SourceSize > 0 && j.TargetSize > 0
		if valid {
			d.Summary.SourceBytes += j.SourceSize
			d.Summary.TargetBytes += j.TargetSize
		}
		inRange := period.Contains(last.EventTime)
		var w *model.DashboardWorker
		if last.WorkerName != "" {
			w = worker(last.WorkerName)
			if state == "active" {
				w.ActiveJobs++
			}
		}
		if inRange && (state == "completed" || state == "failed") {
			if state == "completed" {
				d.Summary.CompletedInRange++
				if w != nil {
					w.Completed++
				}
				for i := len(events) - 1; i >= 0; i-- {
					e := events[i]
					if e.NotificationType == model.JobNotification && e.Status == model.StartedNotificationStatus {
						durationSum += last.EventTime.Sub(e.EventTime).Seconds()
						durationCount++
						break
					}
				}
			} else {
				d.Summary.FailedInRange++
				if w != nil {
					w.Failed++
				}
			}
			for i := range d.Timeline {
				if d.Timeline[i].Time.Equal(last.EventTime.UTC().Truncate(step)) {
					if state == "completed" {
						d.Timeline[i].Completed++
					} else {
						d.Timeline[i].Failed++
					}
					if valid {
						d.Timeline[i].SourceBytes += j.SourceSize
						d.Timeline[i].TargetBytes += j.TargetSize
					}
				}
			}
			if valid {
				d.Period.SourceBytes += j.SourceSize
				d.Period.TargetBytes += j.TargetSize
				if w != nil {
					w.SourceBytes += j.SourceSize
					w.TargetBytes += j.TargetSize
				}
				codec := "unknown"
				if j.TargetProbe != nil {
					for _, stream := range j.TargetProbe.Streams {
						if stream.CodecType == "video" {
							codec = stream.CodecName
							break
						}
					}
				}
				if codecs[codec] == nil {
					codecs[codec] = &model.DashboardCodec{Name: codec}
				}
				c := codecs[codec]
				c.Jobs++
				c.SourceBytes += j.SourceSize
				c.TargetBytes += j.TargetSize
			}
		}
		job := model.DashboardJob{ID: id, SourcePath: j.SourcePath, SourceBytes: j.SourceSize, TargetBytes: j.TargetSize,
			State: state, Phase: string(last.NotificationType), Worker: last.WorkerName, Message: last.Message,
			UpdatedAt: last.EventTime, Progress: []model.DashboardProgress{}}
		if state == "active" && last.Status == model.StartedNotificationStatus {
			for _, progress := range r.progress {
				if progress.JobId.String() == id && progress.WorkerName == last.WorkerName && progress.NotificationType == last.NotificationType {
					p := model.DashboardProgress{ID: progress.ProgressID, Phase: string(progress.NotificationType), Percent: progress.Percent,
						ETASeconds: progress.ETA.Seconds(), UpdatedAt: progress.EventTime}
					normalizeDashboardProgress(&p, now)
					job.Progress = append(job.Progress, p)
				}
			}
		}
		d.Jobs = append(d.Jobs, job)
		for _, e := range events {
			if period.Contains(e.EventTime) && (e.NotificationType == model.JobNotification || e.Status == model.FailedNotificationStatus) {
				d.Activity = append(d.Activity, model.DashboardActivity{JobID: id, SourcePath: j.SourcePath, Worker: e.WorkerName,
					Phase: string(e.NotificationType), Status: string(e.Status), Message: e.Message, Time: e.EventTime})
			}
		}
	}
	if durationCount > 0 {
		d.Summary.AverageSeconds = durationSum / float64(durationCount)
	}
	for _, w := range workers {
		if c, ok := r.workerCommands[w.Name]; ok {
			copy := c.Clone()
			w.Control = &copy
		}
		d.Workers = append(d.Workers, *w)
	}
	sort.Slice(d.Workers, func(i, j int) bool { return d.Workers[i].Name < d.Workers[j].Name })
	rank := map[string]int{"active": 0, "queued": 1, "completed": 2, "failed": 2, "canceled": 2}
	sort.Slice(d.Jobs, func(i, j int) bool {
		a, b := d.Jobs[i], d.Jobs[j]
		if rank[a.State] != rank[b.State] {
			return rank[a.State] < rank[b.State]
		}
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.ID < b.ID
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
	if query == nil {
		if len(d.Jobs) > 100 {
			d.Jobs = d.Jobs[:100]
		}
	} else {
		matching := []model.DashboardJob{}
		for _, j := range d.Jobs {
			if query.State != "" && query.State != "all" && j.State != query.State {
				continue
			}
			if !strings.Contains(strings.ToLower(j.SourcePath+" "+j.Worker+" "+j.ID), strings.ToLower(query.Search)) {
				continue
			}
			matching = append(matching, j)
		}
		d.Jobs = matching
	}
	sort.Slice(d.Activity, func(i, j int) bool { return d.Activity[i].Time.After(d.Activity[j].Time) })
	if len(d.Activity) > 30 {
		d.Activity = d.Activity[:30]
	}
	for _, c := range codecs {
		d.Codecs = append(d.Codecs, *c)
	}
	sort.Slice(d.Codecs, func(i, j int) bool {
		if d.Codecs[i].Jobs == d.Codecs[j].Jobs {
			return d.Codecs[i].Name < d.Codecs[j].Name
		}
		return d.Codecs[i].Jobs > d.Codecs[j].Jobs
	})
	if len(d.Codecs) > 12 {
		d.Codecs = d.Codecs[:12]
	}
	d.Finalize()
	return d, nil
}

func (r *InMemoryRepository) GetDashboardJobs(ctx context.Context, query model.DashboardJobsQuery) (*model.DashboardJobPage, error) {
	d, err := r.dashboard(ctx, 24, time.Now().UTC(), &query)
	if err != nil {
		return nil, err
	}
	total := len(d.Jobs)
	start := min(query.Offset, total)
	end := min(start+query.Limit, total)
	return &model.DashboardJobPage{Jobs: d.Jobs[start:end], Total: int64(total), Offset: query.Offset, Limit: query.Limit}, nil
}
