package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"transcoder/model"

	"github.com/google/uuid"
)

type operation struct {
	data   model.MediaOperation
	cancel context.CancelFunc
}

func (l *Library) newOperation(kind string) (*operation, context.Context, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	active := 0
	for id, op := range l.operations {
		if op.data.State == "running" {
			active++
		} else if time.Since(op.data.UpdatedAt) > 30*time.Minute {
			delete(l.operations, id)
		}
	}
	if active >= 2 {
		return nil, nil, ErrBusy
	}
	if len(l.operations) >= 8 {
		var oldest string
		for id, op := range l.operations {
			if op.data.State != "running" && (oldest == "" || op.data.UpdatedAt.Before(l.operations[oldest].data.UpdatedAt)) {
				oldest = id
			}
		}
		if oldest != "" {
			delete(l.operations, oldest)
		} else {
			return nil, nil, ErrBusy
		}
	}
	ctx, cancel := context.WithTimeout(l.ctx, 2*time.Hour)
	now := time.Now().UTC()
	op := &operation{data: model.MediaOperation{ID: uuid.NewString(), Kind: kind, State: "running", Stage: "starting", StartedAt: now, UpdatedAt: now, Files: []model.MediaFile{}, Results: []model.MediaLoadResult{}}, cancel: cancel}
	l.operations[op.data.ID] = op
	return op, ctx, nil
}

func (l *Library) update(op *operation, fn func(*model.MediaOperation)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(&op.data)
	op.data.UpdatedAt = time.Now().UTC()
}

func (l *Library) finish(op *operation, err error) {
	l.update(op, func(d *model.MediaOperation) {
		d.State = "completed"
		d.Stage = "done"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			d.State = "canceled"
		} else if err != nil {
			d.State = "failed"
			d.Error = err.Error()
		}
	})
	op.cancel()
}

func (l *Library) Operation(id string) (model.MediaOperation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	op, ok := l.operations[id]
	if !ok || op.data.State != "running" && time.Since(op.data.UpdatedAt) > 30*time.Minute {
		delete(l.operations, id)
		return model.MediaOperation{}, ErrOperationNotFound
	}
	d := op.data
	d.Files = append([]model.MediaFile{}, d.Files...)
	d.Results = append([]model.MediaLoadResult{}, d.Results...)
	return d, nil
}

func (l *Library) Cancel(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	op, ok := l.operations[id]
	if !ok {
		return ErrOperationNotFound
	}
	op.cancel()
	return nil
}

func normalizeFilters(f *model.MediaFilters) error {
	if f.Root == "" {
		f.Root = "source"
	}
	if f.Path == "" {
		f.Path = "."
	}
	if !ValidPath(f.Path, true) {
		return ErrInvalidPath
	}
	if f.CandidateLimit == 0 {
		f.CandidateLimit = 1000
	}
	if f.CandidateLimit < 1 || f.CandidateLimit > 5000 || f.MinBytes < 0 || f.MaxBytes < 0 || f.MinWidth < 0 || f.MinBitrateMbps < 0 || math.IsNaN(f.MinBitrateMbps) || math.IsInf(f.MinBitrateMbps, 0) || len(f.Contains) > 1024 || len(f.Exclude) > 1024 || len(f.IncludeCodecs) > 32 || len(f.ExcludeCodecs) > 32 {
		return fmt.Errorf("invalid media filters")
	}
	if f.MaxBytes > 0 && f.MaxBytes < f.MinBytes {
		return fmt.Errorf("maximum size is smaller than minimum size")
	}
	return nil
}

func cheapMatch(f model.MediaFile, filters model.MediaFilters) bool {
	p := strings.ToLower(f.Path)
	if filters.Contains != "" && !strings.Contains(p, strings.ToLower(filters.Contains)) {
		return false
	}
	if filters.Exclude != "" && strings.Contains(p, strings.ToLower(filters.Exclude)) {
		return false
	}
	return f.Size >= filters.MinBytes && (filters.MaxBytes == 0 || f.Size <= filters.MaxBytes)
}

func metadataMatch(m *model.MediaMetadata, f model.MediaFilters) bool {
	if m == nil || m.Codec == "" {
		return false
	}
	if f.MinWidth > 0 && m.Width <= f.MinWidth {
		return false
	}
	if f.MinBitrateMbps > 0 && m.BitrateMbps <= f.MinBitrateMbps {
		return false
	}
	codec := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "h265" || s == "x265" {
			return "hevc"
		}
		if s == "x264" {
			return "h264"
		}
		return s
	}
	for _, c := range f.ExcludeCodecs {
		if codec(c) == codec(m.Codec) {
			return false
		}
	}
	if len(f.IncludeCodecs) > 0 {
		for _, c := range f.IncludeCodecs {
			if codec(c) == codec(m.Codec) {
				return true
			}
		}
		return false
	}
	return true
}

func (l *Library) StartScan(filters model.MediaFilters) (model.MediaOperation, error) {
	if err := normalizeFilters(&filters); err != nil {
		return model.MediaOperation{}, err
	}
	r, _, err := l.openRoot(filters.Root)
	if err != nil {
		return model.MediaOperation{}, err
	}
	if err = CheckPath(r, filters.Path, true); err != nil {
		_ = r.Close()
		return model.MediaOperation{}, err
	}
	info, statErr := r.Stat(filters.Path)
	_ = r.Close()
	if statErr != nil {
		return model.MediaOperation{}, statErr
	}
	if !info.IsDir() {
		return model.MediaOperation{}, ErrInvalidPath
	}
	op, ctx, err := l.newOperation("scan")
	if err != nil {
		return model.MediaOperation{}, err
	}
	l.update(op, func(d *model.MediaOperation) { d.Filters = filters; d.Stage = "discovering" })
	go func() { err := l.scan(ctx, op, filters); l.finish(op, err) }()
	return l.Operation(op.data.ID)
}

func (l *Library) scan(ctx context.Context, op *operation, filters model.MediaFilters) error {
	r, _, err := l.openRoot(filters.Root)
	if err != nil {
		return err
	}
	defer r.Close()
	candidates := []model.MediaFile{}
	discovered := 0
	err = fs.WalkDir(r.FS(), filters.Path, func(p string, e fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if p != filters.Path && (strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0) {
			if e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			if !filters.Recursive && p != filters.Path {
				return fs.SkipDir
			}
			return nil
		}
		if !IsVideo(p) {
			return nil
		}
		discovered++
		if discovered > 100000 {
			l.update(op, func(d *model.MediaOperation) { d.Truncated = true })
			return fs.SkipAll
		}
		if discovered%100 == 0 {
			l.update(op, func(d *model.MediaOperation) { d.Discovered = discovered })
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f := mediaFile(filters.Root, p, info)
		if cheapMatch(f, filters) {
			candidates = append(candidates, f)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size > candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})
	// State lookup happens in bounded chunks before limiting probes, so existing
	// jobs do not consume the scarce metadata-probing budget by default.
	eligible := make([]model.MediaFile, 0, len(candidates))
	for i := 0; i < len(candidates); i += 500 {
		end := min(i+500, len(candidates))
		chunk := candidates[i:end]
		if err := l.annotate(ctx, chunk, filters); err != nil {
			return err
		}
		for _, f := range chunk {
			if !filters.OnlyEligible || f.Eligible {
				eligible = append(eligible, f)
			}
		}
	}
	if len(eligible) > filters.CandidateLimit {
		eligible = eligible[:filters.CandidateLimit]
		l.update(op, func(d *model.MediaOperation) { d.Truncated = true })
	}
	l.update(op, func(d *model.MediaOperation) {
		d.Discovered = min(discovered, 100000)
		d.Candidates = len(eligible)
		d.Stage = "probing"
	})
	work := make(chan model.MediaFile)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				full, err := l.info(ctx, f.ID, false)
				l.update(op, func(d *model.MediaOperation) {
					d.Examined++
					if err != nil {
						d.Failed++
						d.Results = append(d.Results, model.MediaLoadResult{ID: f.ID, Path: f.Path, State: "failed", Message: "Could not analyze the video"})
						return
					}
					if full.Size != f.Size || full.ModifiedNS != f.ModifiedNS {
						d.Failed++
						d.Results = append(d.Results, model.MediaLoadResult{ID: f.ID, Path: f.Path, State: "failed", Message: "The file changed during analysis"})
						return
					}
					full.Job = f.Job
					full.Eligible = f.Eligible
					full.Reason = f.Reason
					if metadataMatch(full.Metadata, filters) {
						d.Matched++
						d.Files = append(d.Files, full)
					}
				})
			}
		}()
	}
	for _, f := range eligible {
		select {
		case work <- f:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(work)
	wg.Wait()
	l.update(op, func(d *model.MediaOperation) {
		sort.Slice(d.Files, func(i, j int) bool {
			if d.Files[i].Size != d.Files[j].Size {
				return d.Files[i].Size > d.Files[j].Size
			}
			return d.Files[i].Path < d.Files[j].Path
		})
	})
	return ctx.Err()
}

func (l *Library) StartBatch(req model.MediaBatchRequest) (model.MediaOperation, error) {
	if req.Target < 1 || req.Target > 5000 || len(req.IDs) < 1 || len(req.IDs) > 5000 {
		return model.MediaOperation{}, fmt.Errorf("select files and a target between 1 and 5000")
	}
	scan, err := l.Operation(req.ScanID)
	if err != nil {
		return model.MediaOperation{}, err
	}
	if scan.Kind != "scan" || scan.State != "completed" || scan.Filters.Root != "source" {
		return model.MediaOperation{}, fmt.Errorf("a completed source preview is required")
	}
	selection := make(map[string]bool, len(req.IDs))
	for _, id := range req.IDs {
		if selection[id] {
			return model.MediaOperation{}, fmt.Errorf("duplicate selection")
		}
		selection[id] = true
	}
	files := []model.MediaFile{}
	for _, f := range scan.Files {
		if selection[f.ID] {
			if !f.Eligible {
				return model.MediaOperation{}, fmt.Errorf("selection contains an ineligible file")
			}
			files = append(files, f)
			delete(selection, f.ID)
		}
	}
	if len(selection) > 0 {
		return model.MediaOperation{}, fmt.Errorf("selection does not belong to the reviewed preview")
	}
	op, ctx, err := l.newOperation("queue")
	if err != nil {
		return model.MediaOperation{}, err
	}
	l.update(op, func(d *model.MediaOperation) {
		d.Target = req.Target
		d.Candidates = len(files)
		d.Filters = scan.Filters
		d.Stage = "queueing"
	})
	go func() { defer op.cancel(); err := l.load(ctx, op, files, scan.Filters, req.Target); l.finish(op, err) }()
	return l.Operation(op.data.ID)
}

func (l *Library) load(ctx context.Context, op *operation, files []model.MediaFile, filters model.MediaFilters, target int) error {
	scheduled := 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if scheduled >= target {
			break
		}
		result := model.MediaLoadResult{ID: f.ID, Path: f.Path, State: "skipped"}
		opened, current, err := l.Open(f.ID)
		if err == nil {
			_ = opened.Close()
			if current.Size != f.Size || current.ModifiedNS != f.ModifiedNS {
				err = ErrChanged
			}
		}
		if err != nil {
			result.Message = "The file changed or is no longer available"
		} else {
			entries := []model.MediaFile{f}
			if err := l.annotate(ctx, entries, filters); err != nil {
				return err
			}
			if !entries[0].Eligible {
				result.Message = entries[0].Reason
			} else {
				j, err := l.backend.QueueMediaFile(ctx, f, filters)
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					result.State = "failed"
					result.Message = err.Error()
					if errors.Is(err, ErrChanged) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrInvalidPath) {
						result.State = "skipped"
						result.Message = "The file changed or is no longer available"
					} else {
						// Another intake can win after our state read. Report that
						// collision as a skip, without consuming the success target.
						latest := []model.MediaFile{f}
						if stateErr := l.annotate(ctx, latest, filters); stateErr == nil && !latest[0].Eligible {
							result.State = "skipped"
							result.Message = latest[0].Reason
						}
					}
				} else {
					result.State = "scheduled"
					result.JobID = j.Id.String()
					scheduled++
				}
			}
		}
		l.update(op, func(d *model.MediaOperation) {
			d.Examined++
			d.Results = append(d.Results, result)
			switch result.State {
			case "scheduled":
				d.Scheduled++
			case "skipped":
				d.Skipped++
			case "failed":
				d.Failed++
			}
		})
	}
	return nil
}

// Presets reproduce the operators' existing SQL criteria, using real files and
// ffprobe instead of requiring access to a separate Plex database.
func Presets() map[string]model.MediaFilters {
	return map[string]model.MediaFilters{
		"movies": {Root: "source", Path: ".", Recursive: true, Contains: "Peliculas", MinWidth: 1800, MinBitrateMbps: 4, ExcludeCodecs: []string{"hevc", "h265", "av1"}, CandidateLimit: 1000, OnlyEligible: true},
		"series": {Root: "source", Path: ".", Recursive: true, Contains: "Series", MinWidth: 1800, MinBitrateMbps: 4, ExcludeCodecs: []string{"hevc", "h265"}, CandidateLimit: 1000, OnlyEligible: true},
		"all":    {Root: "source", Path: ".", Recursive: true, CandidateLimit: 1000, OnlyEligible: true},
	}
}
