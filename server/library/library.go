package library

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"transcoder/model"
)

var (
	ErrInvalidPath       = errors.New("invalid media path")
	ErrUnavailable       = errors.New("media unavailable")
	ErrBusy              = errors.New("media operations busy")
	ErrOperationNotFound = errors.New("media operation expired or not found")
	ErrChanged           = errors.New("media changed since preview")
)

type Backend interface {
	MediaSourceRoot() string
	MediaMinimumSize() int64
	GetMediaProbe(context.Context, string, string, int64, int64) (*model.MediaProbe, error)
	PutMediaProbe(context.Context, string, string, int64, int64, *model.MediaProbe) error
	GetMediaJobStates(context.Context, []string) (map[string]model.MediaJobState, error)
	QueueMediaFile(context.Context, model.MediaFile, model.MediaFilters) (*model.Job, error)
	GetJob(context.Context, string) (*model.Job, error)
}

type Library struct {
	backend    Backend
	roots      map[string]string
	mu         sync.Mutex
	operations map[string]*operation
	probes     chan struct{}
	streams    chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}

type BrowseResult struct {
	Root   string            `json:"root"`
	Path   string            `json:"path"`
	Total  int               `json:"total"`
	Offset int               `json:"offset"`
	Files  []model.MediaFile `json:"files"`
}

func New(backend Backend, extraRoots map[string]string, maxStreams int) *Library {
	if maxStreams < 1 {
		maxStreams = 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &Library{backend: backend, roots: map[string]string{"source": backend.MediaSourceRoot()}, operations: make(map[string]*operation), probes: make(chan struct{}, 2), streams: make(chan struct{}, maxStreams), ctx: ctx, cancel: cancel}
	for id, dir := range extraRoots {
		if id != "source" && validRootID(id) && filepath.IsAbs(dir) {
			l.roots[id] = dir
		}
	}
	return l
}

func (l *Library) Close() { l.cancel() }

func validRootID(id string) bool {
	if len(id) == 0 || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func ValidPath(p string, directory bool) bool {
	if directory && (p == "" || p == ".") {
		return true
	}
	if !fs.ValidPath(p) || strings.ContainsAny(p, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

func IsVideo(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mkv", ".mp4", ".m4v", ".avi", ".mov", ".webm", ".mpg", ".mpeg", ".ts", ".m2ts", ".vob", ".ogv", ".wmv":
		return true
	}
	return false
}

func FileID(root, p string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(root + "\x00" + p))
}

func decodeID(id string) (string, string, error) {
	if len(id) > 8192 {
		return "", "", ErrInvalidPath
	}
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", "", ErrInvalidPath
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 2 || !validRootID(parts[0]) || !ValidPath(parts[1], false) || !IsVideo(parts[1]) {
		return "", "", ErrInvalidPath
	}
	return parts[0], parts[1], nil
}

func (l *Library) openRoot(id string) (*os.Root, string, error) {
	dir, ok := l.roots[id]
	if !ok {
		return nil, "", ErrInvalidPath
	}
	if !filepath.IsAbs(dir) {
		return nil, "", ErrUnavailable
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		_ = r.Close()
		return nil, "", ErrUnavailable
	}
	return r, fmt.Sprintf("%x", sha256.Sum256([]byte(real))), nil
}

func CheckPath(r *os.Root, p string, directory bool) error {
	if !ValidPath(p, directory) {
		return ErrInvalidPath
	}
	if p == "" || p == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(p, "/") {
		current = path.Join(current, part)
		info, err := r.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidPath
		}
	}
	return nil
}

func (l *Library) Roots() []model.MediaRoot {
	result := []model.MediaRoot{}
	for id := range l.roots {
		name := id
		if id == "source" {
			name = "Origen"
		}
		if id == "encoded" {
			name = "Codificados"
		}
		r, _, err := l.openRoot(id)
		if err == nil {
			_ = r.Close()
		}
		result = append(result, model.MediaRoot{ID: id, Name: name, Queueable: id == "source", Available: err == nil})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == "source" {
			return true
		}
		if result[j].ID == "source" {
			return false
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func mediaFile(root, p string, info fs.FileInfo) model.MediaFile {
	return model.MediaFile{ID: FileID(root, p), Root: root, Path: p, Name: info.Name(), Directory: info.IsDir(), Size: info.Size(), ModifiedAt: info.ModTime().UTC(), ModifiedNS: info.ModTime().UnixNano()}
}

func (l *Library) Open(id string) (*os.File, model.MediaFile, error) {
	root, p, err := decodeID(id)
	if err != nil {
		return nil, model.MediaFile{}, err
	}
	r, _, err := l.openRoot(root)
	if err != nil {
		return nil, model.MediaFile{}, err
	}
	defer r.Close()
	if err := CheckPath(r, p, false); err != nil {
		return nil, model.MediaFile{}, err
	}
	f, err := r.Open(p)
	if err != nil {
		return nil, model.MediaFile{}, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, model.MediaFile{}, ErrInvalidPath
	}
	return f, mediaFile(root, p, info), nil
}

func (l *Library) Browse(ctx context.Context, root, p string, offset, limit int) (*BrowseResult, error) {
	if p == "" {
		p = "."
	}
	if offset < 0 || limit < 1 || limit > 200 {
		return nil, ErrInvalidPath
	}
	r, _, err := l.openRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if err := CheckPath(r, p, true); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(r.FS(), p)
	if err != nil {
		return nil, err
	}
	filtered := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() || IsVideo(e.Name()) {
			filtered = append(filtered, e)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return strings.ToLower(filtered[i].Name()) < strings.ToLower(filtered[j].Name())
	})
	result := &BrowseResult{Root: root, Path: p, Total: len(filtered), Offset: offset, Files: []model.MediaFile{}}
	for i := offset; i < len(filtered) && i < offset+limit; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := filtered[i].Info()
		if err != nil {
			continue
		}
		result.Files = append(result.Files, mediaFile(root, path.Join(p, info.Name()), info))
	}
	if err := l.annotate(ctx, result.Files, model.MediaFilters{}); err != nil {
		return nil, err
	}
	return result, nil
}

func (l *Library) annotate(ctx context.Context, files []model.MediaFile, filters model.MediaFilters) error {
	paths := []string{}
	for _, f := range files {
		if !f.Directory && f.Root == "source" {
			paths = append(paths, f.Path)
		}
	}
	states, err := l.backend.GetMediaJobStates(ctx, paths)
	if err != nil {
		return err
	}
	for i := range files {
		f := &files[i]
		f.Eligible = false
		switch {
		case f.Directory:
			f.Reason = "Carpeta"
		case f.Root != "source":
			f.Reason = "Biblioteca de salida (solo reproducción)"
		case f.Size < l.backend.MediaMinimumSize():
			f.Reason = "Inferior al tamaño mínimo del servidor"
		default:
			f.Eligible = true
		}
		if state, ok := states[f.Path]; ok && f.Root == "source" {
			copy := state
			f.Job = &copy
			f.Eligible = false
			f.Reason = "Ya tiene un job: " + state.State
			if state.Role == "output" {
				f.Reason = "Salida de un job completado"
			} else if f.Size >= l.backend.MediaMinimumSize() && (state.State == "failed" && filters.RetryFailed || state.State == "canceled" && filters.RetryCanceled) {
				f.Eligible = true
				f.Reason = "Reintento explícito"
			}
		}
	}
	return nil
}

func (l *Library) JobAssets(ctx context.Context, id string) ([]model.MediaFile, error) {
	j, err := l.backend.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	assets := []model.MediaFile{}
	for _, asset := range []struct{ p, role string }{{j.SourcePath, "source"}, {j.TargetPath, "output"}} {
		if asset.role == "output" {
			event := j.Events.GetLatestPerNotificationType(model.JobNotification)
			if event == nil || event.Status != model.CompletedNotificationStatus {
				continue
			}
		}
		for _, root := range l.Roots() {
			if asset.role == "source" && root.ID != "source" {
				continue
			}
			f, entry, err := l.Open(FileID(root.ID, asset.p))
			if err != nil {
				continue
			}
			_ = f.Close()
			entry.Job = &model.MediaJobState{ID: id, State: j.Status, Role: asset.role}
			assets = append(assets, entry)
			break
		}
	}
	return assets, nil
}
