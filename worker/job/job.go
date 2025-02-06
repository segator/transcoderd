package job

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"sync"
	"transcoder/model"
	"transcoder/worker/ffmpeg"
)

type Context struct {
	JobId      uuid.UUID     `json:"job_id"`
	EventId    int           `json:"event_id"`
	WorkingDir string        `json:"working_dir"`
	LastEvent  *ContextEvent `json:"last_event"`
	Source     *VideoData    `json:"source"`
	Target     *VideoData    `json:"target"`
	mu         sync.Mutex
}

type VideoData struct {
	FilePath    string                    `json:"file_path"`
	Checksum    string                    `json:"checksum"`
	FFProbeData *ffmpeg.NormalizedFFProbe `json:"ffprobe_data"`
	Size        int64
}

type ContextEvent struct {
	EventId          int                      `json:"event_id"`
	NotificationType model.NotificationType   `json:"notification_type"`
	Status           model.NotificationStatus `json:"status"`
	Message          string                   `json:"message"`
}

func NewContext(jobId uuid.UUID, lastEvent int, workingDir string) *Context {
	return &Context{
		mu:         sync.Mutex{},
		JobId:      jobId,
		EventId:    lastEvent,
		WorkingDir: workingDir,
		Source:     &VideoData{},
	}
}
func (j *Context) Init() error {
	return os.MkdirAll(j.WorkingDir, os.ModePerm)
}

func (j *Context) Clean() error {
	return os.RemoveAll(j.WorkingDir)
}

func (j *Context) UpdateEvent(notificationType model.NotificationType, status model.NotificationStatus, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	newEventID := j.EventId + 1
	if j.LastEvent != nil {
		newEventID = j.LastEvent.EventId + 1
	}
	JobCtxEvent := &ContextEvent{
		EventId:          newEventID,
		NotificationType: notificationType,
		Status:           status,
		Message:          message,
	}
	j.LastEvent = JobCtxEvent
}

func (e *Context) PersistJobContext() error {
	b, err := json.MarshalIndent(e, "", "\t")
	if err != nil {
		return err
	}
	eventFile, err := os.OpenFile(filepath.Join(e.WorkingDir, fmt.Sprintf("%s.json", e.JobId)), os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return err
	}
	defer eventFile.Close()
	_, err = eventFile.Write(b)
	if err != nil {
		return err
	}
	return eventFile.Sync()
}

func ReadContextFromDiskByPath(filepath string) *Context {
	eventFile, err := os.Open(filepath)
	if err != nil {
		panic(err)
	}
	defer eventFile.Close()
	b, err := io.ReadAll(eventFile)
	if err != nil {
		panic(err)
	}
	jobContext := &Context{}
	err = json.Unmarshal(b, jobContext)
	if err != nil {
		panic(err)
	}
	return jobContext
}
