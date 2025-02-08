package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"transcoder/helper"
	"transcoder/model"
	"transcoder/server/repository"
)

var (
	x264ex = regexp.MustCompile(`(?i)((([xh])264)|mpeg-4|mpeg-1|mpeg-2|mpeg|xvid|divx|vc-1|av1|vp8|vp9|wmv3|mp43)`)
	ac3ex  = regexp.MustCompile(`(?i)(ac3|eac3|pcm|flac|mp2|dts|mp3|truehd|wma|vorbis|opus|mpeg audio)`)
)

type Scheduler interface {
	Run(wg *sync.WaitGroup, ctx context.Context)
	ScheduleJobRequests(ctx context.Context, jobRequest *model.JobRequest) (*ScheduleJobRequestResult, error)
	GetUploadJobWriter(ctx context.Context, uuid string, workerName string) (*UploadJobStream, error)
	GetDownloadJobWriter(ctx context.Context, uuid string, workerName string) (*DownloadJobStream, error)
	GetChecksum(ctx context.Context, uuid string) (string, error)
	RequestJob(ctx context.Context, workerName string) (*model.RequestJobResponse, error)
	HandleWorkerEvent(ctx context.Context, taskEvent *model.EnvelopEvent) error
	CancelJob(ctx context.Context, id string) error
}

type Config struct {
	ScheduleTime           time.Duration `mapstructure:"scheduleTime"`
	JobTimeout             time.Duration `mapstructure:"jobTimeout"`
	SourcePath             string        `mapstructure:"sourcePath"`
	DeleteSourceOnComplete bool          `mapstructure:"deleteOnComplete"`
	MinFileSize            int64         `mapstructure:"minFileSize"`
}

type RuntimeScheduler struct {
	config          *Config
	repo            repository.Repository
	checksumChan    chan PathChecksum
	pathChecksumMap map[string]string
	jobRequestMu    sync.Mutex
	handleEventMu   sync.Mutex
}

func (r *RuntimeScheduler) RequestJob(ctx context.Context, workerName string) (*model.RequestJobResponse, error) {
	r.jobRequestMu.Lock()
	defer r.jobRequestMu.Unlock()
	video, err := r.repo.RetrieveQueuedJob(ctx)
	if err != nil {
		if errors.Is(err, repository.ErrElementNotFound) {
			return nil, NoJobsAvailable
		}
		return nil, err
	}
	if video == nil {
		return nil, nil
	}
	newEvent := video.AddEvent(model.JobNotification, model.AssignedNotificationStatus)
	newEvent.WorkerName = workerName
	if err = r.repo.AddNewTaskEvent(ctx, newEvent); err != nil {
		return nil, err
	}

	task := &model.RequestJobResponse{
		Id:      video.Id,
		EventID: video.Events.GetLatest().EventID,
	}
	log.WithFields(log.Fields{
		"job_id":      video.Id.String(),
		"worker":      workerName,
		"source_path": video.SourcePath,
	}).Infof("Job assigned to %s", workerName)
	return task, nil
}

func (r *RuntimeScheduler) HandleWorkerEvent(ctx context.Context, envelopedEvent *model.EnvelopEvent) error {
	r.handleEventMu.Lock()
	defer r.handleEventMu.Unlock()
	if err := r.processEvent(ctx, envelopedEvent); err != nil {
		return err
	}
	return nil
}

func (r *RuntimeScheduler) CancelJob(ctx context.Context, id string) error {
	job, err := r.repo.GetJob(ctx, id)
	if err != nil {
		return err
	}

	status := job.Events.GetStatus()
	switch {
	case status == model.CompletedNotificationStatus:
		return fmt.Errorf("job already completed")
	case status == model.FailedNotificationStatus:
		return fmt.Errorf("job is failed")
	case status == model.CanceledNotificationStatus:
		return fmt.Errorf("job already canceled")
	case status == model.AssignedNotificationStatus, status == model.StartedNotificationStatus:
		newEvent := job.AddEventComplete(model.JobNotification, model.CanceledNotificationStatus, "Job canceled by user")
		err = r.repo.AddNewTaskEvent(ctx, newEvent)
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("job %s is in unknown state", id)
}

func (r *RuntimeScheduler) processEvent(ctx context.Context, event *model.EnvelopEvent) error {
	var err error
	switch event.EventType {
	case model.PingEvent:
		pingEvent := model.PingEventType{}
		if err = json.Unmarshal(event.EventData, &pingEvent); err != nil {
			return err
		}
		return r.repo.PingServerUpdate(ctx, pingEvent)
	case model.NotificationEvent:
		taskEvent := model.TaskEventType{}
		if err = json.Unmarshal(event.EventData, &taskEvent); err != nil {
			return err
		}
		if err = r.repo.AddNewTaskEvent(ctx, &taskEvent); err != nil {
			return err
		}
		if !taskEvent.IsCompleted() {
			return nil
		}
		if err = r.completeJob(ctx, &taskEvent); err != nil {
			return err
		}
	case model.ProgressEvent:
		taskProgress := model.TaskProgressType{}
		if err = json.Unmarshal(event.EventData, &taskProgress); err != nil {
			return err
		}
		if taskProgress.Status != model.ProgressingTaskProgressTypeStatus {
			if err = r.repo.DeleteProgressJob(ctx, taskProgress.ProgressID, taskProgress.NotificationType); err != nil {
				return err
			}
			return nil
		}
		if err = r.repo.ProgressJob(ctx, &taskProgress); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown event type %s", event.EventType)
	}

	return nil
}

func (r *RuntimeScheduler) completeJob(ctx context.Context, jobEvent *model.TaskEventType) error {
	video, err := r.repo.GetJob(ctx, jobEvent.JobId.String())
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(r.config.SourcePath, video.SourcePath)
	target := filepath.Join(r.config.SourcePath, video.TargetPath)
	l := log.WithFields(log.Fields{
		"job_id":      jobEvent.JobId.String(),
		"source_path": sourcePath,
		"target_path": target,
	})
	targetStat, err := os.Stat(target)
	if err != nil {
		l.Warnf("target path can not be found because: %v", err)
		return err
	}
	video.TargetSize = targetStat.Size()

	err = r.repo.UpdateJob(ctx, video)
	if err != nil {
		l.Warnf("target job can not be updated because %v", err)
		return err
	}

	l.Infof("Job completed")

	if r.config.DeleteSourceOnComplete {
		l.Info("Job completed, removing source file")
		err = os.Remove(sourcePath)
		if err != nil {
			l.Warnf("Job completed, source file can not be removed because %v", err)
			return err
		}
	}

	return nil
}

func NewScheduler(config *Config, repo repository.Repository) (*RuntimeScheduler, error) {
	runtimeScheduler := &RuntimeScheduler{
		config:          config,
		repo:            repo,
		checksumChan:    make(chan PathChecksum),
		pathChecksumMap: make(map[string]string),
	}

	return runtimeScheduler, nil
}

func (r *RuntimeScheduler) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("Starting Scheduler...")
	r.start(ctx)
	log.Info("Started Scheduler...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("Stopping Scheduler...")
		r.stop()
		wg.Done()
	}()
}

func (r *RuntimeScheduler) start(ctx context.Context) {
	go r.scheduleRoutine(ctx)
}

func (r *RuntimeScheduler) scheduleRoutine(ctx context.Context) {
	progressTicker := time.NewTicker(time.Minute * 1)
	maintenanceTicker := time.NewTicker(r.config.ScheduleTime)
	defer progressTicker.Stop()
	defer maintenanceTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case checksumPath := <-r.checksumChan:
			r.pathChecksumMap[checksumPath.path] = checksumPath.checksum
		case <-progressTicker.C:
			if err := r.progressJobMaitenance(ctx); err != nil {
				log.Errorf("Error on progress job maintenance: %s", err)
			}
		case <-maintenanceTicker.C:
			if err := r.jobMaintenance(ctx); err != nil {
				log.Errorf("Error on job maintenance: %s", err)
			}
		}
	}
}

type JobRequestResult struct {
	jobRequest *model.JobRequest
	errors     []string
}

func (r *RuntimeScheduler) createNewJobRequestByJobRequestDirectory(ctx context.Context, parentJobRequest *model.JobRequest, searchJobRequestChan chan<- *JobRequestResult) {
	defer close(searchJobRequestChan)
	err := filepath.Walk(filepath.Join(r.config.SourcePath, parentJobRequest.SourcePath), func(pathFile string, f os.FileInfo, err error) error {
		var jobRequestErrors []string
		select {
		case <-ctx.Done():
			return fmt.Errorf("search for new Jobs canceled")
		default:
			if f.IsDir() {
				return nil
			}
			if f.Size() < r.config.MinFileSize {
				jobRequestErrors = append(jobRequestErrors, fmt.Sprintf("%s File Size bigger than %d", pathFile, r.config.MinFileSize))
			}
			extension := filepath.Ext(f.Name())[1:]
			if !helper.ValidExtension(extension) {
				jobRequestErrors = append(jobRequestErrors, fmt.Sprintf("%s Invalid Extension %s", pathFile, extension))
			}

			relativePathSource, err := filepath.Rel(r.config.SourcePath, filepath.FromSlash(pathFile))
			if err != nil {
				jobRequestErrors = append(jobRequestErrors, err.Error())
			}

			relativePathTarget := formatTargetName(relativePathSource)
			if relativePathTarget == relativePathSource {
				ext := filepath.Ext(relativePathTarget)
				relativePathTarget = strings.Replace(relativePathTarget, ext, "_encoded.mkv", 1)
			}

			searchJobRequestChan <- &JobRequestResult{
				jobRequest: &model.JobRequest{
					SourcePath:     relativePathSource,
					SourceSize:     f.Size(),
					TargetPath:     relativePathTarget,
					ForceCompleted: parentJobRequest.ForceCompleted,
					ForceFailed:    parentJobRequest.ForceFailed,
					ForceAssigned:  parentJobRequest.ForceAssigned,
				},
				errors: jobRequestErrors,
			}
		}
		return nil
	})
	if err != nil {
		log.Errorf("error on search for new Jobs %s", err)
		return
	}
}

type ScheduleJobRequestResult struct {
	ScheduledJobs    []*model.Job             `json:"scheduled"`
	FailedJobRequest []*model.JobRequestError `json:"failed"`
	SkippedFiles     []*model.JobRequestError `json:"skipped"`
}

func (r *RuntimeScheduler) scheduleJobRequest(ctx context.Context, jobRequest *model.JobRequest) (job *model.Job, err error) {
	l := log.WithFields(log.Fields{
		"source_path": jobRequest.SourcePath,
	})
	err = r.repo.WithTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		job, err = tx.GetJobByPath(ctx, jobRequest.SourcePath)
		if err != nil {
			return err
		}

		var eventsToAdd []*model.TaskEventType
		if job == nil {
			job, err = r.newJob(ctx, tx, jobRequest)
			if err != nil {
				return err
			}
			eventsToAdd = job.Events
		} else {
			// If job exist we check if we can retry the job
			eventsToAdd, err = r.updateJobByRequest(job, jobRequest)
			if err != nil {
				return err
			}
		}

		for _, taskEvent := range eventsToAdd {
			err = tx.AddNewTaskEvent(ctx, taskEvent)
			if err != nil {
				return err
			}
			l.WithField("job_id", job.Id.String()).Infof("job is now %s", taskEvent.Status)
		}

		return nil
	})
	return job, err
}

func (r *RuntimeScheduler) newJob(ctx context.Context, tx repository.Repository, jobRequest *model.JobRequest) (*model.Job, error) {
	l := log.WithFields(log.Fields{
		"source_path": jobRequest.SourcePath,
	})
	newUUID, _ := uuid.NewUUID()
	job := &model.Job{
		SourcePath: jobRequest.SourcePath,
		SourceSize: jobRequest.SourceSize,
		TargetPath: jobRequest.TargetPath,
		Id:         newUUID,
	}
	l.WithField("job_id", job.Id.String()).Info("Creating new job")
	err := tx.AddJob(ctx, job)
	if err != nil {
		return nil, err
	}
	job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)
	return job, nil
}

func (r *RuntimeScheduler) ScheduleJobRequests(ctx context.Context, jobRequest *model.JobRequest) (result *ScheduleJobRequestResult, returnError error) {
	result = &ScheduleJobRequestResult{}
	searchJobRequestChan := make(chan *JobRequestResult, 10)
	_, returnError = os.Stat(filepath.Join(r.config.SourcePath, jobRequest.SourcePath))
	if os.IsNotExist(returnError) {
		return nil, returnError
	}

	go r.createNewJobRequestByJobRequestDirectory(ctx, jobRequest, searchJobRequestChan)

	for jobRequestResponse := range searchJobRequestChan {
		var err error
		var video *model.Job
		if jobRequestResponse.errors == nil {
			video, err = r.scheduleJobRequest(ctx, jobRequestResponse.jobRequest)
			if err == nil {
				video.Events = nil
			}
		} else {
			b, _ := json.Marshal(jobRequestResponse.errors)
			err = errors.New(string(b))
		}
		if err != nil {
			if errors.Is(err, ErrorFileSkipped) {
				result.SkippedFiles = append(result.SkippedFiles, &model.JobRequestError{
					JobRequest: *jobRequestResponse.jobRequest,
					Error:      errors.Unwrap(err).Error(),
				})
			} else {
				result.FailedJobRequest = append(result.FailedJobRequest, &model.JobRequestError{
					JobRequest: *jobRequestResponse.jobRequest,
					Error:      err.Error(),
				})
			}
		} else {
			result.ScheduledJobs = append(result.ScheduledJobs, video)
		}
	}
	return result, returnError
}

func (r *RuntimeScheduler) isValidStremeableJob(ctx context.Context, uuid string, workerName string) (*model.Job, error) {
	video, err := r.repo.GetJob(ctx, uuid)
	if err != nil {
		return nil, err
	}
	te := video.Events.GetLatestPerNotificationType(model.JobNotification)
	if te.Status != model.StartedNotificationStatus {
		return nil, fmt.Errorf("%w: job is in status %s", ErrorStreamNotAllowed, te.Status)
	}
	if te.WorkerName != workerName {
		return nil, fmt.Errorf("%w: job is not assigned to worker %s", ErrorStreamNotAllowed, workerName)
	}
	return video, nil
}
func (r *RuntimeScheduler) GetDownloadJobWriter(ctx context.Context, uuid string, workerName string) (*DownloadJobStream, error) {
	video, err := r.isValidStremeableJob(ctx, uuid, workerName)
	if err != nil {
		return nil, err
	}
	filePath := filepath.Join(r.config.SourcePath, video.SourcePath)
	downloadFile, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrorJobNotFound
		} else {
			return nil, err
		}
	}
	dfStat, err := downloadFile.Stat()
	if err != nil {
		return nil, err
	}
	return &DownloadJobStream{
		JobStream: &JobStream{
			video:             video,
			file:              downloadFile,
			path:              filePath,
			checksumPublisher: r.checksumChan,
		},
		FileSize: dfStat.Size(),
		FileName: dfStat.Name(),
	}, nil

}

func (r *RuntimeScheduler) GetUploadJobWriter(ctx context.Context, uuid string, workerName string) (*UploadJobStream, error) {
	video, err := r.isValidStremeableJob(ctx, uuid, workerName)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(r.config.SourcePath, video.TargetPath)
	err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return nil, err
	}
	temporalPath := filePath + ".upload"
	uploadFile, err := os.OpenFile(temporalPath, os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return &UploadJobStream{
		&JobStream{
			video:        video,
			file:         uploadFile,
			path:         filePath,
			temporalPath: temporalPath,
		},
	}, nil
}

func (r *RuntimeScheduler) GetChecksum(ctx context.Context, uuid string) (string, error) {
	video, err := r.repo.GetJob(ctx, uuid)
	if err != nil {
		return "", err
	}
	filePath := filepath.Join(r.config.SourcePath, video.SourcePath)
	checksum := r.pathChecksumMap[filePath]
	if checksum == "" {
		return "", fmt.Errorf("%w: Checksum not found for %s", ErrorJobNotFound, filePath)
	}
	return checksum, nil
}

func (r *RuntimeScheduler) stop() {

}

func (r *RuntimeScheduler) progressJobMaitenance(ctx context.Context) error {
	progressJobs, err := r.repo.GetAllProgressJobs(ctx)
	if err != nil {
		return err
	}
	for _, progressJob := range progressJobs {
		if time.Since(progressJob.EventTime) > time.Hour*24 {
			if err = r.repo.DeleteProgressJob(ctx, progressJob.ProgressID, progressJob.NotificationType); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RuntimeScheduler) jobMaintenance(ctx context.Context) error {
	if err := r.queuedJobMaintenance(ctx); err != nil {
		return err
	}
	if err := r.failedJobMaintenance(ctx); err != nil {
		return err
	}
	return r.assignedJobMaintenance(ctx)

}

func (r *RuntimeScheduler) queuedJobMaintenance(ctx context.Context) error {
	queuedJobs, err := r.repo.GetJobsByStatus(ctx, model.JobNotification, model.QueuedNotificationStatus)
	if err != nil {
		return err
	}
	for _, job := range queuedJobs {
		sourcePath := filepath.Join(r.config.SourcePath, job.SourcePath)
		// Check if source file exists
		_, err = os.Stat(sourcePath)
		if os.IsNotExist(err) {
			newEvent := job.AddEventComplete(model.JobNotification, model.FailedNotificationStatus, "job source file not found")
			if err = r.repo.AddNewTaskEvent(ctx, newEvent); err != nil {
				return err
			}
			continue
		}
	}
	return nil
}

func (r *RuntimeScheduler) failedJobMaintenance(ctx context.Context) error {
	failedJobs, err := r.repo.GetJobsByStatus(ctx, model.JobNotification, model.FailedNotificationStatus)
	if err != nil {
		return err
	}
	for _, failedJob := range failedJobs {
		if !verifyFailureMessage(failedJob.StatusMessage) {
			continue
		}

		failureJobEvents := failedJob.Events.FilterBy(model.JobNotification, model.FailedNotificationStatus)
		if len(failureJobEvents) > 10 {
			newEvent := failedJob.AddEventComplete(model.JobNotification, model.CanceledNotificationStatus, "Job canceled by system, because of too many failed attempts")
			if err = r.repo.AddNewTaskEvent(ctx, newEvent); err != nil {
				return err
			}
			continue
		}

		jobRequest := &model.JobRequest{
			SourcePath:  failedJob.SourcePath,
			TargetPath:  failedJob.TargetPath,
			ForceFailed: true,
		}
		_, err = r.scheduleJobRequest(ctx, jobRequest)
		if err != nil {
			return err
		}

	}
	return nil
}

func (r *RuntimeScheduler) assignedJobMaintenance(ctx context.Context) error {
	taskEvents, err := r.repo.GetTimeoutJobs(ctx, r.config.JobTimeout)
	if err != nil {
		return err
	}
	for _, taskEvent := range taskEvents {
		if taskEvent.IsAssigned() {
			log.Infof("Rescheduling %s after job timeout", taskEvent.JobId.String())
			job, err := r.repo.GetJob(ctx, taskEvent.JobId.String())
			if err != nil {
				return err
			}
			jobRequest := &model.JobRequest{
				SourcePath:    job.SourcePath,
				TargetPath:    job.TargetPath,
				ForceAssigned: true,
			}
			_, err = r.scheduleJobRequest(ctx, jobRequest)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *RuntimeScheduler) updateJobByRequest(job *model.Job, jobRequest *model.JobRequest) ([]*model.TaskEventType, error) {
	var eventsToAdd []*model.TaskEventType
	lastEvent := job.Events.GetLatestPerNotificationType(model.JobNotification)
	status := lastEvent.Status

	switch {
	case jobRequest.ForceAssigned && (status == model.AssignedNotificationStatus || status == model.StartedNotificationStatus):
		eventsToAdd = append(eventsToAdd, job.AddEvent(model.JobNotification, model.CanceledNotificationStatus))
		eventsToAdd = append(eventsToAdd, job.AddEvent(model.JobNotification, model.QueuedNotificationStatus))

	case jobRequest.ForceCompleted && status == model.CompletedNotificationStatus,
		jobRequest.ForceFailed && status == model.FailedNotificationStatus,
		jobRequest.ForceCanceled && status == model.CanceledNotificationStatus:
		requeueEvent := job.AddEvent(model.JobNotification, model.QueuedNotificationStatus)
		eventsToAdd = append(eventsToAdd, requeueEvent)
	default:
		return nil, fmt.Errorf("%s (%s) job is in %s state by %s, can not be rescheduled", job.Id.String(), jobRequest.SourcePath, lastEvent.Status, lastEvent.WorkerName)
	}
	return eventsToAdd, nil
}

func simpleRegex(pattern string, string string) bool {
	m, err := regexp.MatchString(strings.ToLower(pattern), strings.ToLower(string))
	if err != nil {
		panic(err)
	}
	return m
}

func verifyFailureMessage(message string) bool {
	if simpleRegex(`job not found`, message) {
		return false
	}
	if simpleRegex(`runtime error: index out of range`, message) {
		return false
	}

	if simpleRegex(`not 200 respose in dowload code 404`, message) {
		return false
	}
	if simpleRegex(`source File size `, message) {
		return false
	}
	if simpleRegex(`source File duration `, message) {
		return false
	}

	if simpleRegex(`Disk quota exceeded`, message) || simpleRegex(`No space left on device`, message) {
		return true
	}
	// if simpleRegex(`At least one output file must be specified`, message) {
	//	return true
	//}
	if simpleRegex(`MKVExtract unexpected error`, message) {
		return true
	}
	// if simpleRegex(`core dumped`, message) {
	//	return true
	//}
	if simpleRegex(`dow(n)?load code 500`, message) {
		return true
	}
	// TODO al arreglar el tema del trailing descomentar
	// if simpleRegex(`Trailing option\(s\) found in the command`, message) {
	//	return true
	//}
	// if simpleRegex(`signal: killed`, message) {
	//	return true
	//}
	// if simpleRegex(`signal: aborted`, message) {
	//	return true
	//}
	// if simpleRegex(`error getting data`, message) {
	//	return true
	//}
	if message == "exit status 1: stder: stdout:" {
		return true
	}
	if simpleRegex(`runtime error: invalid memory address or nil pointer dereference`, message) {
		return true
	}
	if simpleRegex(`CHecksum error on download source`, message) {
		return true
	}
	if simpleRegex(`maybe incorrect parameters such as bit_rate`, message) || simpleRegex(`Could not find codec parameters for stream`, message) {
		return true
	}
	if simpleRegex(`GLIBC_2.34 not found`, message) || simpleRegex(`libc.so.6: version`, message) {
		return true
	}
	if simpleRegex(`received signal 15`, message) {
		return true
	}
	if simpleRegex(`no such host`, message) || simpleRegex(`server misbehaving`, message) || simpleRegex(`i/o timeout`, message) || simpleRegex(`connection failed because connected host has failed to respond`, message) {
		return true
	}
	if simpleRegex(`connection refused`, message) {
		return true
	}

	if simpleRegex(`with 0 items`, message) {
		return false
	}

	// if simpleRegex(`srt: Invalid data found when processing input`, message) {
	//	return true
	//}
	// if simpleRegex(`segmentation fault`, message) {
	//	return true
	//}
	if simpleRegex(`Subtitle: mov_text`, message) {
		return false
	}
	if simpleRegex(`unsupported AVCodecID S_TEXT/WEBVTT`, message) {
		return false
	}
	if simpleRegex(`Errorf while decoding stream`, message) {
		return false
	}
	if simpleRegex(`Data: bin_data`, message) {
		return false
	}
	if simpleRegex(`scale/rate is 0/0 which is invalid`, message) {
		return false
	}
	if simpleRegex(`probably corrupt input`, message) {
		return false
	}

	return false
}

func formatTargetName(path string) string {
	p := x264ex.ReplaceAllString(path, "x265")
	p = ac3ex.ReplaceAllString(p, "AAC")
	extension := filepath.Ext(p)
	p = strings.Replace(p, extension, ".mkv", 1)

	return p
}
