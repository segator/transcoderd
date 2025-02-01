package model

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
	"transcoder/helper/max"
)

type EventType string
type NotificationType string
type NotificationStatus string
type JobAction string
type TaskEvents []*TaskEventType

const (
	PingEvent         EventType = "Ping"
	NotificationEvent EventType = "Notification"
	ProgressEvent     EventType = "Progress"

	JobNotification             NotificationType   = "Job"
	DownloadNotification        NotificationType   = "Download"
	UploadNotification          NotificationType   = "Upload"
	MKVExtractNotification      NotificationType   = "MKVExtract"
	FFProbeNotification         NotificationType   = "FFProbe"
	PGSNotification             NotificationType   = "PGS"
	FFMPEGSNotification         NotificationType   = "FFMPEG"
	JobVerify                   NotificationType   = "JobVerify"
	QueuedNotificationStatus    NotificationStatus = "queued"
	AssignedNotificationStatus  NotificationStatus = "assigned"
	StartedNotificationStatus   NotificationStatus = "started"
	CompletedNotificationStatus NotificationStatus = "completed"
	CanceledNotificationStatus  NotificationStatus = "canceled"
	FailedNotificationStatus    NotificationStatus = "failed"
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

type JobEvent struct {
	Id     uuid.UUID `json:"id"`
	Action JobAction `json:"action"`
}

type JobType string

type RequestJobResponse struct {
	Id      uuid.UUID `json:"id"`
	EventID int       `json:"eventID"`
}

type TaskPGS struct {
	PGSID         int
	PGSSourcePath string
	PGSLanguage   string
	PGSTargetPath string
}

type EnvelopEvent struct {
	EventType EventType       `json:"eventType"`
	EventData json.RawMessage `json:"eventData"`
}

type Event struct {
	EventTime  time.Time `json:"eventTime"`
	WorkerName string    `json:"workerName"`
}
type TaskEventType struct {
	Event
	JobId            uuid.UUID          `json:"Id"`
	EventID          int                `json:"eventID"`
	NotificationType NotificationType   `json:"notificationType"`
	Status           NotificationStatus `json:"status"`
	Message          string             `json:"message"`
}

type PingEventType struct {
	Event
	IP string `json:"ip"`
}

type TaskProgressType struct {
	Event
	JobId            uuid.UUID        `json:"jobId"`
	ProgressID       string           `json:"progressID"`
	Percent          float64          `json:"percent"`
	ETA              time.Duration    `json:"eta"`
	NotificationType NotificationType `json:"notificationType"`
}

func (e TaskEventType) IsAssigned() bool {
	if e.NotificationType == JobNotification && (e.Status == AssignedNotificationStatus || e.Status == StartedNotificationStatus) {
		return true
	}
	return false
}

func (e TaskEventType) IsCompleted() bool {
	if e.NotificationType == JobNotification && e.Status == CompletedNotificationStatus {
		return true
	}
	return false
}

func (e TaskEventType) IsDownloading() bool {
	if e.NotificationType == DownloadNotification && e.Status == StartedNotificationStatus {
		return true
	}

	if e.NotificationType == JobNotification && (e.Status == StartedNotificationStatus) {
		return true
	}
	return false
}

func (e TaskEventType) IsEncoding() bool {
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

func (e TaskEventType) IsUploading() bool {
	if e.NotificationType == FFMPEGSNotification && e.Status == CompletedNotificationStatus {
		return true
	}

	if e.NotificationType == UploadNotification && e.Status == StartedNotificationStatus {
		return true
	}

	return false
}

func (t TaskEvents) GetLatest() *TaskEventType {
	if len(t) == 0 {
		return nil
	}
	return max.Max(t).(*TaskEventType)
}
func (t TaskEvents) GetLatestPerNotificationType(notificationType NotificationType) (returnEvent *TaskEventType) {
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
	ForceCanceled  bool   `json:"forceCanceled"`
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
func (t TaskEvents) GetByEventId(i int) (*TaskEventType, error) {
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
func (v *Job) AddEvent(notificationType NotificationType, notificationStatus NotificationStatus) (newEvent *TaskEventType) {
	return v.AddEventComplete(notificationType, notificationStatus, "")
}

func (v *Job) AddEventComplete(notificationType NotificationType, notificationStatus NotificationStatus, message string) (newEvent *TaskEventType) {
	latestEvent := v.Events.GetLatest()
	newEventID := 0
	if latestEvent != nil {
		newEventID = latestEvent.EventID + 1
	}

	newEvent = &TaskEventType{
		Event: Event{
			EventTime: time.Now(),
		},
		JobId:            v.Id,
		EventID:          newEventID,
		NotificationType: notificationType,
		Status:           notificationStatus,
		Message:          message,
	}
	v.Events = append(v.Events, newEvent)
	return newEvent
}
