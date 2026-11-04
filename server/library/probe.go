package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"transcoder/model"
	workerffmpeg "transcoder/worker/ffmpeg"

	"gopkg.in/vansante/go-ffprobe.v2"
)

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("media process output exceeded limit")
	}
	return b.Buffer.Write(p)
}

func mediaInput(cmd *exec.Cmd, f *os.File) string {
	switch runtime.GOOS {
	case "linux":
		cmd.ExtraFiles = []*os.File{f}
		return "/proc/self/fd/3"
	case "darwin", "freebsd", "openbsd":
		cmd.ExtraFiles = []*os.File{f}
		return "/dev/fd/3"
	default:
		return f.Name()
	}
}

func (l *Library) Info(ctx context.Context, id string) (model.MediaFile, error) {
	return l.info(ctx, id, true)
}

func (l *Library) info(ctx context.Context, id string, includeState bool) (model.MediaFile, error) {
	f, entry, err := l.Open(id)
	if err != nil {
		return model.MediaFile{}, err
	}
	defer f.Close()
	r, key, err := l.openRoot(entry.Root)
	if err != nil {
		return model.MediaFile{}, err
	}
	_ = r.Close()
	probe, err := l.backend.GetMediaProbe(ctx, key, entry.Path, entry.Size, entry.ModifiedNS)
	if err != nil {
		return model.MediaFile{}, err
	}
	if probe == nil {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		select {
		case l.probes <- struct{}{}:
			defer func() { <-l.probes }()
		case <-ctx.Done():
			return model.MediaFile{}, ctx.Err()
		}
		cmd := exec.CommandContext(ctx, "ffprobe")
		input := mediaInput(cmd, f)
		cmd.Args = append(cmd.Args, "-v", "error", "-show_format", "-show_streams", "-of", "json", input)
		out := &boundedBuffer{limit: 2 << 20}
		stderr := &boundedBuffer{limit: 8192}
		cmd.Stdout = out
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return model.MediaFile{}, fmt.Errorf("could not probe video: %w", err)
		}
		var raw ffprobe.ProbeData
		if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
			return model.MediaFile{}, fmt.Errorf("invalid media metadata: %w", err)
		}
		probe = workerffmpeg.ProbeDataToMediaProbe(&raw)
		if probe == nil {
			return model.MediaFile{}, ErrUnavailable
		}
		// A file modified during probing must not publish stale cached metadata.
		info, err := f.Stat()
		if err != nil || info.Size() != entry.Size || info.ModTime().UnixNano() != entry.ModifiedNS {
			return model.MediaFile{}, ErrChanged
		}
		if err := l.backend.PutMediaProbe(ctx, key, entry.Path, entry.Size, entry.ModifiedNS, probe); err != nil {
			return model.MediaFile{}, err
		}
	}
	metadata := &model.MediaMetadata{DurationSeconds: probe.DurationSeconds, Probe: probe}
	if metadata.DurationSeconds < 0 || math.IsNaN(metadata.DurationSeconds) || math.IsInf(metadata.DurationSeconds, 0) {
		metadata.DurationSeconds = 0
	}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			metadata.Width, metadata.Height, metadata.Codec = stream.Width, stream.Height, strings.ToLower(stream.CodecName)
			break
		}
	}
	if metadata.DurationSeconds > 0 {
		metadata.BitrateMbps = float64(entry.Size) * 8 / metadata.DurationSeconds / 1e6
	}
	entry.Metadata = metadata
	if !includeState {
		return entry, nil
	}
	entries := []model.MediaFile{entry}
	if err := l.annotate(ctx, entries, model.MediaFilters{}); err != nil {
		return model.MediaFile{}, err
	}
	return entries[0], nil
}

func (l *Library) Preview(ctx context.Context, id string, start float64, w io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(l.ctx, cancel)
	defer stop()
	if math.IsNaN(start) || math.IsInf(start, 0) || start < 0 || start > 7*24*3600 {
		return ErrInvalidPath
	}
	f, entry, err := l.Open(id)
	if err != nil {
		return err
	}
	defer f.Close()
	// A replaced player request can arrive just before its canceled process exits.
	// Allow that slot to settle without ever starting more than the configured cap.
	slotTimer := time.NewTimer(500 * time.Millisecond)
	defer slotTimer.Stop()
	select {
	case l.streams <- struct{}{}:
		defer func() { <-l.streams }()
	case <-ctx.Done():
		return ctx.Err()
	case <-slotTimer.C:
		return ErrBusy
	}
	cmd := exec.CommandContext(ctx, "ffmpeg")
	input := mediaInput(cmd, f)
	cmd.Args = append(cmd.Args, "-hide_banner", "-loglevel", "error", "-nostdin", "-re", "-ss", fmt.Sprintf("%.3f", start), "-threads", "2", "-i", input,
		"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn", "-vf", "scale=w='min(1280,iw)':h='min(720,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "26", "-threads", "2", "-filter_threads", "1", "-r", "24", "-g", "48", "-sc_threshold", "0",
		"-c:a", "aac", "-ac", "2", "-b:a", "128k", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-frag_duration", "1000000", "-f", "mp4", "pipe:1")
	stderr := &boundedBuffer{limit: 8192}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start media preview: %w", err)
	}
	_, copyErr := io.Copy(w, stdout)
	if copyErr != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("preview %s: %w", entry.Name, waitErr)
	}
	return nil
}
