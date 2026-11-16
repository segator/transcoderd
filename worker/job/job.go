package job

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
)

var ErrInvalidPrefetchedSource = errors.New("invalid prefetched source")

type Context struct {
	JobId            uuid.UUID              `json:"job_id"`
	EventId          int                    `json:"event_id"`
	WorkingDir       string                 `json:"working_dir"`
	LastEvent        *ContextEvent          `json:"last_event"`
	NextPhase        model.NotificationType `json:"next_phase,omitempty"`
	Source           *VideoData             `json:"source"`
	Target           *VideoData             `json:"target"`
	TargetMediaProbe *model.MediaProbe      `json:"target_media_probe,omitempty"`
	EncodeKind       string                 `json:"encode_kind,omitempty"`
	EncodeConfig     *config.FFMPEGConfig   `json:"encode_config,omitempty"`
	Assignment       *model.AssignmentClaim `json:"assignment,omitempty"`
	RequestID        uuid.UUID              `json:"request_id,omitempty"`
	PendingEvent     *model.TaskEventType   `json:"pending_event,omitempty"`
	ParkIntent       bool                   `json:"park_intent,omitempty"`
	Parked           bool                   `json:"parked,omitempty"`
	mu               sync.Mutex
}

type VideoData struct {
	FilePath     string                    `json:"file_path"`
	OriginalName string                    `json:"original_name,omitempty"`
	Checksum     string                    `json:"checksum"`
	FFProbeData  *ffmpeg.NormalizedFFProbe `json:"ffprobe_data"`
	Size         int64
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
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.persistJobContextLocked()
}

func (e *Context) persistJobContextLocked() error {
	if e.WorkingDir == "" {
		return fmt.Errorf("persist job context: working directory is empty")
	}
	b, err := json.MarshalIndent(e, "", "\t")
	if err != nil {
		return err
	}
	return writeAtomicContextFile(e.WorkingDir, fmt.Sprintf("%s.json", e.JobId), b)
}

func writeAtomicContextFile(dir, name string, b []byte) error {
	// Write the complete snapshot before replacing the previous one. A crash or
	// short write must not destroy the only recoverable copy of an accepted job.
	eventFile, err := os.CreateTemp(dir, ".job-context-*")
	if err != nil {
		return err
	}
	tempPath := eventFile.Name()
	defer os.Remove(tempPath)
	if _, err = eventFile.Write(b); err != nil {
		_ = eventFile.Close()
		return fmt.Errorf("write job context: %w", err)
	}
	if err = eventFile.Sync(); err != nil {
		_ = eventFile.Close()
		return fmt.Errorf("sync job context: %w", err)
	}
	if err = eventFile.Close(); err != nil {
		return fmt.Errorf("close job context: %w", err)
	}
	if err = replaceContextFile(tempPath, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("replace job context: %w", err)
	}
	return nil
}

// ValidatePrefetchedSource verifies the downloaded data before it is treated as
// recoverable. The path must stay inside the saved job workspace.
func (e *Context) ValidatePrefetchedSource() error {
	if e.WorkingDir == "" || e.Source == nil || e.Source.FilePath == "" || e.Source.Size <= 0 || e.Source.Checksum == "" || e.Source.FFProbeData == nil {
		return fmt.Errorf("%w: incomplete source metadata", ErrInvalidPrefetchedSource)
	}
	rootPath, err := filepath.Abs(e.WorkingDir)
	if err != nil {
		return fmt.Errorf("%w: resolve workspace: %v", ErrInvalidPrefetchedSource, err)
	}
	sourcePath, err := filepath.Abs(e.Source.FilePath)
	if err != nil {
		return fmt.Errorf("%w: resolve source: %v", ErrInvalidPrefetchedSource, err)
	}
	relative, err := filepath.Rel(rootPath, sourcePath)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("%w: source outside workspace", ErrInvalidPrefetchedSource)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return fmt.Errorf("%w: open workspace: %v", ErrInvalidPrefetchedSource, err)
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return fmt.Errorf("%w: open source: %v", ErrInvalidPrefetchedSource, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != e.Source.Size {
		return fmt.Errorf("%w: source is not a regular file of the expected size", ErrInvalidPrefetchedSource)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("%w: read source: %v", ErrInvalidPrefetchedSource, err)
	}
	if n != e.Source.Size {
		return fmt.Errorf("%w: source changed while reading", ErrInvalidPrefetchedSource)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), e.Source.Checksum) {
		return fmt.Errorf("%w: checksum mismatch", ErrInvalidPrefetchedSource)
	}
	return nil
}

func ReadContextFromDiskByPath(path string) (*Context, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("open job context directory %q: %w", path, err)
	}
	defer root.Close()
	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("read job context %q: %w", path, err)
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, 16<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read job context %q: %w", path, err)
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("read job context %q: file is too large", path)
	}
	jobContext := new(Context)
	if err := json.Unmarshal(b, jobContext); err != nil {
		return nil, fmt.Errorf("decode job context %q: %w", path, err)
	}
	return jobContext, nil
}
