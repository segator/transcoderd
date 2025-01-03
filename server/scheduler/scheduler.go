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
	x264ex = regexp.MustCompile(`(?i)(((x|h)264)|mpeg-4|mpeg-1|mpeg-2|mpeg|xvid|divx|vc-1|av1|vp8|vp9|wmv3|mp43)`)
	ac3ex  = regexp.MustCompile(`(?i)(ac3|eac3|pcm|flac|mp2|dts|mp2|mp3|truehd|wma|vorbis|opus|mpeg audio)`)
)

type Scheduler interface {
	Run(wg *sync.WaitGroup, ctx context.Context)
	ScheduleJobRequests(ctx context.Context, jobRequest *model.JobRequest) (*ScheduleJobRequestResult, error)
	GetUploadJobWriter(ctx context.Context, uuid string) (*UploadJobStream, error)
	GetDownloadJobWriter(ctx context.Context, uuid string) (*DownloadJobStream, error)
	GetChecksum(ctx context.Context, uuid string) (string, error)
	RequestJob(ctx context.Context) (*model.TaskEncode, error)
	HandleWorkerEvent(ctx context.Context, taskEvent *model.TaskEvent) error
}

type SchedulerConfig struct {
	ScheduleTime           time.Duration `mapstructure:"scheduleTime"`
	JobTimeout             time.Duration `mapstructure:"jobTimeout"`
	SourcePath             string        `mapstructure:"sourcePath"`
	DeleteSourceOnComplete bool          `mapstructure:"deleteOnComplete"`
	MinFileSize            int64         `mapstructure:"minFileSize"`
	checksums              map[string][]byte
}

type RuntimeScheduler struct {
	config          SchedulerConfig
	repo            repository.Repository
	checksumChan    chan PathChecksum
	pathChecksumMap map[string]string
}

func (R *RuntimeScheduler) RequestJob(ctx context.Context) (*model.TaskEncode, error) {
	mut := sync.Mutex{}
	mut.Lock()
	defer mut.Unlock()
	video, err := R.repo.RetrieveQueuedJob(ctx)
	if err != nil {
		if errors.As(err, &repository.ErrElementNotFound) {
			return nil, NoJobsAvailable
		}
		return nil, err
	}
	if video == nil {
		return nil, nil
	}
	newEvent := video.AddEvent(model.NotificationEvent, model.JobNotification, model.AssignedNotificationStatus)
	if err = R.repo.AddNewTaskEvent(ctx, newEvent); err != nil {
		return nil, err
	}

	task := &model.TaskEncode{
		Id:      video.Id,
		EventID: video.Events.GetLatest().EventID,
	}
	return task, nil
}

func (R *RuntimeScheduler) HandleWorkerEvent(ctx context.Context, jobEvent *model.TaskEvent) error {
	// Store any event
	var mut sync.Mutex
	mut.Lock()
	defer mut.Unlock()
	if err := R.repo.ProcessEvent(ctx, jobEvent); err != nil {
		return err
	}

	if jobEvent.EventType == model.NotificationEvent && jobEvent.NotificationType == model.JobNotification && jobEvent.Status == model.CompletedNotificationStatus {
		if err := R.completeJob(ctx, jobEvent); err != nil {
			log.Error(err)
		}
	}
	return nil
}

func (R *RuntimeScheduler) completeJob(ctx context.Context, jobEvent *model.TaskEvent) error {
	video, err := R.repo.GetJob(ctx, jobEvent.Id.String())
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(R.config.SourcePath, video.SourcePath)
	target := filepath.Join(R.config.SourcePath, video.TargetPath)
	targetStat, err := os.Stat(target)
	if err != nil {
		log.Warnf("Job %s completed, target file %s can not be found because %s", jobEvent.Id.String(), target, err)
		return err
	}
	video.TargetSize = targetStat.Size()

	err = R.repo.UpdateJob(ctx, video)
	if err != nil {
		log.Warnf("Job %s completed, target file %s can not be updated because %s", jobEvent.Id.String(), target)
		return err
	}

	if R.config.DeleteSourceOnComplete {
		log.Infof("Job %s completed, removing source file %s", jobEvent.Id.String(), sourcePath)
		err = os.Remove(sourcePath)
		if err != nil {
			log.Warnf("Job %s completed, source file %s can not be removed because %s", jobEvent.Id.String(), sourcePath, err)
			return err
		}
	}

	return nil
}

func NewScheduler(config SchedulerConfig, repo repository.Repository) (*RuntimeScheduler, error) {
	runtimeScheduler := &RuntimeScheduler{
		config:          config,
		repo:            repo,
		checksumChan:    make(chan PathChecksum),
		pathChecksumMap: make(map[string]string),
	}

	return runtimeScheduler, nil
}

func (R *RuntimeScheduler) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("Starting Scheduler...")
	R.start(ctx)
	log.Info("Started Scheduler...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Info("Stopping Scheduler...")
		R.stop()
		wg.Done()
	}()
}

func (R *RuntimeScheduler) start(ctx context.Context) {
	go R.schedule(ctx)
}

func (R *RuntimeScheduler) schedule(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case checksumPath := <-R.checksumChan:
			R.pathChecksumMap[checksumPath.path] = checksumPath.checksum
		case <-time.After(R.config.ScheduleTime):
			taskEvents, err := R.repo.GetTimeoutJobs(ctx, R.config.JobTimeout)
			if err != nil {
				log.Error(err)
			}
			for _, taskEvent := range taskEvents {
				if taskEvent.IsAssigned() {
					log.Infof("Rescheduling %s after job timeout", taskEvent.Id.String())
					video, err := R.repo.GetJob(ctx, taskEvent.Id.String())
					if err != nil {
						log.Error(err)
						continue
					}
					jobRequest := &model.JobRequest{
						SourcePath:    video.SourcePath,
						TargetPath:    video.TargetPath,
						ForceAssigned: true,
					}
					video, err = R.scheduleJobRequest(ctx, jobRequest)
					if err != nil {
						log.Error(err)
					}
				}
			}
		}
	}
}

type JobRequestResult struct {
	jobRequest *model.JobRequest
	errors     []string
}

func (R *RuntimeScheduler) createNewJobRequestByJobRequestDirectory(ctx context.Context, parentJobRequest *model.JobRequest, searchJobRequestChan chan<- *JobRequestResult) {
	defer close(searchJobRequestChan)
	filepath.Walk(filepath.Join(R.config.SourcePath, parentJobRequest.SourcePath), func(pathFile string, f os.FileInfo, err error) error {
		var jobRequestErrors []string
		select {
		case <-ctx.Done():
			return fmt.Errorf("search for new Jobs canceled")
		default:
			if f.IsDir() {
				return nil
			}
			if f.Size() < R.config.MinFileSize {
				jobRequestErrors = append(jobRequestErrors, fmt.Sprintf("%s File Size bigger than %d", pathFile, R.config.MinFileSize))
			}
			extension := filepath.Ext(f.Name())[1:]
			if !helper.ValidExtension(extension) {
				jobRequestErrors = append(jobRequestErrors, fmt.Sprintf("%s Invalid Extension %s", pathFile, extension))
			}

			relativePathSource, err := filepath.Rel(R.config.SourcePath, filepath.FromSlash(pathFile))
			if err != nil {
				jobRequestErrors = append(jobRequestErrors, err.Error())
			}

			relativePathTarget := formatTargetName(relativePathSource)
			if relativePathTarget == relativePathSource {
				ext := filepath.Ext(relativePathTarget)
				relativePathTarget = strings.Replace(relativePathTarget, ext, "_encoded.mkv", 1)
			}
			pathFile = filepath.ToSlash(pathFile)
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
}

type ScheduleJobRequestResult struct {
	ScheduledJobs    []*model.Job             `json:"scheduled"`
	FailedJobRequest []*model.JobRequestError `json:"failed"`
	SkippedFiles     []*model.JobRequestError `json:"skipped"`
}

func (R *RuntimeScheduler) scheduleJobRequest(ctx context.Context, jobRequest *model.JobRequest) (video *model.Job, err error) {
	err = R.repo.WithTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		video, err = tx.GetJobByPath(ctx, jobRequest.SourcePath)
		if err != nil {
			return err
		}
		var eventsToAdd []*model.TaskEvent
		if video == nil {
			newUUID, _ := uuid.NewUUID()
			video = &model.Job{
				SourcePath: jobRequest.SourcePath,
				SourceSize: jobRequest.SourceSize,
				TargetPath: jobRequest.TargetPath,
				Id:         newUUID,
			}
			err = tx.AddJob(ctx, video)
			if err != nil {
				return err
			}
			startEvent := video.AddEvent(model.NotificationEvent, model.JobNotification, model.QueuedNotificationStatus)
			eventsToAdd = append(eventsToAdd, startEvent)
		} else {
			//If video exist we check if we can retry the job
			lastEvent := video.Events.GetLatestPerNotificationType(model.JobNotification)
			status := video.Events.GetStatus()
			if jobRequest.ForceAssigned && (status == model.AssignedNotificationStatus || status == model.StartedNotificationStatus) {
				cancelEvent := video.AddEvent(model.NotificationEvent, model.JobNotification, model.CanceledNotificationStatus)
				eventsToAdd = append(eventsToAdd, cancelEvent)
			}
			if (jobRequest.ForceCompleted && status == model.CompletedNotificationStatus) ||
				(jobRequest.ForceFailed && (status == model.FailedNotificationStatus || status == model.CanceledNotificationStatus)) ||
				(jobRequest.ForceAssigned && (status == model.StartedNotificationStatus || status == model.AssignedNotificationStatus)) {
				requeueEvent := video.AddEvent(model.NotificationEvent, model.JobNotification, model.QueuedNotificationStatus)
				eventsToAdd = append(eventsToAdd, requeueEvent)
			} else if !(jobRequest.ForceAssigned && status == model.QueuedNotificationStatus) {
				return errors.New(fmt.Sprintf("%s (%s) job is in %s state by %s, can not be rescheduled", video.Id.String(), jobRequest.SourcePath, lastEvent.Status, lastEvent.WorkerName))
			}
		}
		if len(eventsToAdd) > 0 {
			for _, taskEvent := range eventsToAdd {
				err = tx.AddNewTaskEvent(ctx, taskEvent)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
	return video, err
}

func (R *RuntimeScheduler) ScheduleJobRequests(ctx context.Context, jobRequest *model.JobRequest) (result *ScheduleJobRequestResult, returnError error) {
	result = &ScheduleJobRequestResult{}
	searchJobRequestChan := make(chan *JobRequestResult, 10)
	_, returnError = os.Stat(filepath.Join(R.config.SourcePath, jobRequest.SourcePath))
	if os.IsNotExist(returnError) {
		return nil, returnError
	}

	go R.createNewJobRequestByJobRequestDirectory(ctx, jobRequest, searchJobRequestChan)

	for jobRequestResponse := range searchJobRequestChan {
		var err error
		var video *model.Job
		if jobRequestResponse.errors == nil {
			video, err = R.scheduleJobRequest(ctx, jobRequestResponse.jobRequest)
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

func (R *RuntimeScheduler) isValidStremeableJob(ctx context.Context, uuid string) (*model.Job, error) {
	video, err := R.repo.GetJob(ctx, uuid)
	if err != nil {
		return nil, err
	}
	status := video.Events.GetLatestPerNotificationType(model.JobNotification).Status
	if status != model.StartedNotificationStatus {
		return nil, fmt.Errorf("%w: job is in status %s", ErrorStreamNotAllowed, status)
	}
	return video, nil
}
func (R *RuntimeScheduler) GetDownloadJobWriter(ctx context.Context, uuid string) (*DownloadJobStream, error) {
	video, err := R.isValidStremeableJob(ctx, uuid)
	if err != nil {
		return nil, err
	}
	filePath := filepath.Join(R.config.SourcePath, video.SourcePath)
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
		if os.IsNotExist(err) {
			return nil, ErrorJobNotFound
		} else {
			return nil, err
		}
	}
	return &DownloadJobStream{
		JobStream: &JobStream{
			video:             video,
			file:              downloadFile,
			path:              filePath,
			checksumPublisher: R.checksumChan,
		},
		FileSize: dfStat.Size(),
		FileName: dfStat.Name(),
	}, nil

}

func (R *RuntimeScheduler) GetUploadJobWriter(ctx context.Context, uuid string) (*UploadJobStream, error) {
	video, err := R.isValidStremeableJob(ctx, uuid)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(R.config.SourcePath, video.TargetPath)
	err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err != nil {
		return nil, err
	}
	temporalPath := filePath + ".upload"
	uploadFile, err := os.OpenFile(temporalPath, os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	return &UploadJobStream{
		&JobStream{
			video:        video,
			file:         uploadFile,
			path:         filePath,
			temporalPath: temporalPath,
		},
	}, nil
}

func (R *RuntimeScheduler) GetChecksum(ctx context.Context, uuid string) (string, error) {
	video, err := R.repo.GetJob(ctx, uuid)
	if err != nil {
		return "", err
	}
	filePath := filepath.Join(R.config.SourcePath, video.SourcePath)
	checksum := R.pathChecksumMap[filePath]
	if checksum == "" {
		return "", fmt.Errorf("%w: Checksum not found for %s", ErrorJobNotFound, filePath)
	}
	return checksum, nil
}

func (S *RuntimeScheduler) stop() {

}

func formatTargetName(path string) string {
	p := x264ex.ReplaceAllString(path, "x265")
	p = ac3ex.ReplaceAllString(p, "AAC")
	extension := filepath.Ext(p)
	p = strings.Replace(p, extension, ".mkv", 1)

	return p
}
