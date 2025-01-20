package model

import (
	"fmt"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
	"transcoder/helper/max"
)

type EventType string
type NotificationType string
type NotificationStatus string
type JobAction string
type TaskEvents []*TaskEvent

const (
	PingEvent         EventType = "Ping"
	NotificationEvent EventType = "Notification"

	JobNotification        NotificationType = "Job"
	DownloadNotification   NotificationType = "Download"
	UploadNotification     NotificationType = "Upload"
	MKVExtractNotification NotificationType = "MKVExtract"
	FFProbeNotification    NotificationType = "FFProbe"
	PGSNotification        NotificationType = "PGS"
	FFMPEGSNotification    NotificationType = "FFMPEG"

	QueuedNotificationStatus    NotificationStatus = "queued"
	AssignedNotificationStatus  NotificationStatus = "assigned"
	StartedNotificationStatus   NotificationStatus = "started"
	CompletedNotificationStatus NotificationStatus = "completed"
	CanceledNotificationStatus  NotificationStatus = "canceled"
	FailedNotificationStatus    NotificationStatus = "failed"

	CancelJob JobAction = "cancel"
)

type Identity interface {
	getUUID() uuid.UUID
}
type Job struct {
	SourcePath    string           `json:"source_path,omitempty"`
	TargetPath    string           `json:"destination_path,omitempty"`
	Id            uuid.UUID        `json:"id"`
	Events        TaskEvents       `json:"events,omitempty"`
	Status        string           `json:"status,omitempty"`
	StatusPhase   NotificationType `json:"status_phase,omitempty"`
	StatusMessage string           `json:"status_message,omitempty"`
	LastUpdate    *time.Time       `json:"last_update,omitempty"`
	SourceSize    int64            `json:"source_size,omitempty"`
	TargetSize    int64            `json:"target_size,omitempty"`
}

type JobEventQueue struct {
	Queue    string
	JobEvent *JobEvent
}
type Worker struct {
	Name      string
	Ip        string
	QueueName string
	LastSeen  time.Time
}

type ControlEvent struct {
	Event       *TaskEncode
	ControlChan chan interface{}
}

type JobEvent struct {
	Id     uuid.UUID `json:"id"`
	Action JobAction `json:"action"`
}

type JobType string

type TaskEncode struct {
	Id      uuid.UUID `json:"id"`
	EventID int       `json:"eventID"`
}

type WorkTaskEncode struct {
	TaskEncode     *TaskEncode
	WorkDir        string
	SourceFilePath string
	TargetFilePath string
}

type TaskPGS struct {
	PGSID         int
	PGSSourcePath string
	PGSLanguage   string
	PGSTargetPath string
}

type TaskPGSResponse struct {
	Id    uuid.UUID `json:"id"`
	PGSID int       `json:"pgsid"`
	Srt   []byte    `json:"srt"`
	Err   string    `json:"error"`
	Queue string    `json:"queue"`
}

func (t TaskEncode) getUUID() uuid.UUID {
	return t.Id
}

type TaskEvent struct {
	Id               uuid.UUID          `json:"id"`
	EventID          int                `json:"eventID"`
	EventType        EventType          `json:"eventType"`
	WorkerName       string             `json:"workerName"`
	EventTime        time.Time          `json:"eventTime"`
	IP               string             `json:"ip"`
	NotificationType NotificationType   `json:"notificationType"`
	Status           NotificationStatus `json:"status"`
	Message          string             `json:"message"`
}

type TaskStatus struct {
	LastState *TaskEvent
	Task      *WorkTaskEncode
}

func (e TaskEvent) IsAssigned() bool {
	if e.EventType != NotificationEvent {
		return false
	}
	if e.NotificationType == JobNotification && (e.Status == AssignedNotificationStatus || e.Status == StartedNotificationStatus) {
		return true
	}
	return false
}

func (e TaskEvent) IsCompleted() bool {
	if e.EventType != NotificationEvent {
		return false
	}
	if e.NotificationType == JobNotification && e.Status == CompletedNotificationStatus {
		return true
	}
	return false
}

func (e TaskEvent) IsDownloading() bool {
	if e.EventType != NotificationEvent {
		return false
	}
	if e.NotificationType == DownloadNotification && e.Status == StartedNotificationStatus {
		return true
	}

	if e.NotificationType == JobNotification && (e.Status == StartedNotificationStatus) {
		return true
	}
	return false
}

func (e TaskEvent) IsEncoding() bool {
	if e.EventType != NotificationEvent {
		return false
	}
	if e.NotificationType == DownloadNotification && e.Status == CompletedNotificationStatus {
		return true
	}

	if e.NotificationType == MKVExtractNotification && (e.Status == StartedNotificationStatus || e.Status == CompletedNotificationStatus) {
		return true
	}
	if e.NotificationType == FFProbeNotification && (e.Status == StartedNotificationStatus || e.Status == CompletedNotificationStatus) {
		return true
	}
	if e.NotificationType == PGSNotification && (e.Status == StartedNotificationStatus || e.Status == CompletedNotificationStatus) {
		return true
	}
	if e.NotificationType == FFMPEGSNotification && e.Status == StartedNotificationStatus {
		return true
	}

	return false
}

func (e TaskEvent) IsUploading() bool {
	if e.EventType != NotificationEvent {
		return false
	}
	if e.NotificationType == FFMPEGSNotification && e.Status == CompletedNotificationStatus {
		return true
	}

	if e.NotificationType == UploadNotification && e.Status == StartedNotificationStatus {
		return true
	}

	return false
}

func (w *WorkTaskEncode) Clean() error {
	log.Debugf("[%s] Cleaning up Task Workspace", w.TaskEncode.Id.String())
	err := os.RemoveAll(w.WorkDir)
	if err != nil {
		return err
	}
	return nil
}

func (t TaskEvents) GetLatest() *TaskEvent {
	if len(t) == 0 {
		return nil
	}
	return max.Max(t).(*TaskEvent)
}
func (t TaskEvents) GetLatestPerNotificationType(notificationType NotificationType) (returnEvent *TaskEvent) {
	eventID := -1
	for _, event := range t {
		if event.NotificationType == notificationType && event.EventID > eventID {
			eventID = event.EventID
			returnEvent = event
		}
	}
	return returnEvent
}
func (t TaskEvents) GetStatus() NotificationStatus {
	return t.GetLatestPerNotificationType(JobNotification).Status
}

type JobRequestError struct {
	JobRequest
	Error string `json:"error"`
}
type JobRequest struct {
	SourcePath     string `json:"sourcePath"`
	ForceCompleted bool   `json:"forceCompleted"`
	ForceFailed    bool   `json:"forceFailed"`
	ForceAssigned  bool   `json:"forceAssigned"`
	SourceSize     int64
	TargetPath     string
}

func (t TaskEvents) Len() int {
	return len(t)
}
func (t TaskEvents) Less(i, j int) bool {
	return t[i].EventID < t[j].EventID
}
func (t TaskEvents) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}
func (t TaskEvents) GetByEventId(i int) (*TaskEvent, error) {
	for _, event := range t {
		if event.EventID == i {
			return event, nil
		}
	}
	return nil, fmt.Errorf("event not found")
}
func (t TaskEvents) GetLastElement(i int) interface{} {
	return t[i]
}
func (v *Job) AddEvent(eventType EventType, notificationType NotificationType, notificationStatus NotificationStatus) (newEvent *TaskEvent) {
	return v.AddEventComplete(eventType, notificationType, notificationStatus, "")
}

func (v *Job) AddEventComplete(eventType EventType, notificationType NotificationType, notificationStatus NotificationStatus, message string) (newEvent *TaskEvent) {
	latestEvent := v.Events.GetLatest()
	newEventID := 0
	if latestEvent != nil {
		newEventID = latestEvent.EventID + 1
	}

	newEvent = &TaskEvent{
		Id:               v.Id,
		EventID:          newEventID,
		EventType:        eventType,
		EventTime:        time.Now(),
		NotificationType: notificationType,
		Status:           notificationStatus,
		Message:          message,
	}
	v.Events = append(v.Events, newEvent)
	return newEvent
}
