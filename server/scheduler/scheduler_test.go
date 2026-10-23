package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"transcoder/model"
	"transcoder/server/repository"

	"github.com/google/uuid"
)

func TestProbeOutputRejectsInvalidMedia(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "ffprobe failure", script: "#!/bin/sh\nexit 1\n"},
		{name: "no video stream", script: "#!/bin/sh\nprintf '%s\\n' '{\"streams\":[{\"codec_type\":\"audio\",\"codec_name\":\"aac\"}]}'\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte(tt.script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if _, err := probeOutput(context.Background(), "uploaded.mkv"); err == nil {
				t.Fatal("probeOutput() succeeded for invalid output, want error")
			}
		})
	}
}

func TestOutputNameForCodec(t *testing.T) {
	tests := []struct {
		name, source, codec, want string
	}{
		{name: "h264 input to av1", source: "Movies/Film (2024) x264.mkv", codec: "av1", want: "Movies/Film (2024) av1.mkv"},
		{name: "h264 output convention", source: "Film.mkv", codec: "h264", want: "Film.x264.mkv"},
		{name: "hevc input to av1", source: "Series/Episode H265.mp4", codec: "av1", want: "Series/Episode av1.mkv"},
		{name: "hevc output convention", source: "Film h264.mkv", codec: "hevc", want: "Film x265.mkv"},
		{name: "codec missing", source: "Film.mkv", codec: "av1", want: "Film.av1.mkv"},
		{name: "codec-like substring in title", source: "The H264ers.mkv", codec: "av1", want: "The H264ers.av1.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outputNameForCodec(tt.source, tt.codec); got != tt.want {
				t.Errorf("outputNameForCodec(%q, %q) = %q, want %q", tt.source, tt.codec, got, tt.want)
			}
		})
	}
}

func TestFormatTargetNameRecognizesHEVCLabels(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Movies/Film.h265.mp4", want: "Movies/Film.x265.mkv"},
		{input: "Movies/Film.HEVC.mkv", want: "Movies/Film.x265.mkv"},
		{input: "Movies/The H264ers.mkv", want: "Movies/The H264ers.mkv"},
	}
	for _, tt := range tests {
		if got := formatTargetName(tt.input); got != tt.want {
			t.Errorf("formatTargetName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUploadStagingPathRejectsNonLocalSourcePaths(t *testing.T) {
	tests := []string{"../outside.mkv", "/absolute/movie.mkv"}
	for _, sourcePath := range tests {
		t.Run(sourcePath, func(t *testing.T) {
			if _, err := uploadStagingPath(t.TempDir(), sourcePath, uuid.NewString()); err == nil {
				t.Fatalf("uploadStagingPath(%q) succeeded, want non-local path error", sourcePath)
			}
		})
	}
}

func TestCompletedJobFinalizesBeforeDeleteOnComplete(t *testing.T) {
	for _, deleteSource := range []bool{false, true} {
		name := "preserve source"
		if deleteSource {
			name = "delete source after completion"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			bin := t.TempDir()
			ffprobe := filepath.Join(bin, "ffprobe")
			if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\ncat <<'EOF'\n{\"format\":{\"duration\":\"12.5\"},\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\",\"width\":1920,\"height\":1080}]}\nEOF\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			oldPath := os.Getenv("PATH")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+oldPath)
			id := uuid.New()
			sourcePath := filepath.Join(root, "Movie x265.AC3.mkv")
			if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
				t.Fatal(err)
			}
			job := &model.Job{Id: id, SourcePath: "Movie x265.AC3.mkv", TargetPath: "Movie x265.AAC.mkv"}
			repo := repository.NewInMemoryRepository()
			if err := repo.AddJob(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			staging, err := uploadStagingPath(root, job.SourcePath, id.String())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(staging), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(staging, []byte("encoded data"), 0o600); err != nil {
				t.Fatal(err)
			}
			occupiedPath := filepath.Join(root, "Movie av1.AAC.mkv")
			if err := os.WriteFile(occupiedPath, []byte("keep existing"), 0o600); err != nil {
				t.Fatal(err)
			}
			scheduler := &RuntimeScheduler{config: &Config{SourcePath: root, DeleteSourceOnComplete: deleteSource}, repo: repo}
			event := model.TaskEventType{JobId: id, EventID: 0, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
			payload, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if err := scheduler.processEvent(context.Background(), &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: payload}); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetJob(context.Background(), id.String())
			if err != nil {
				t.Fatal(err)
			}
			if stored.TargetPath != "Movie av1.AAC_encoded.mkv" {
				t.Errorf("stored target path = %q, want Movie av1.AAC_encoded.mkv", stored.TargetPath)
			}
			if stored.TargetSize != int64(len("encoded data")) {
				t.Errorf("stored target size = %d", stored.TargetSize)
			}
			if stored.TargetProbe == nil || len(stored.TargetProbe.Streams) == 0 || stored.TargetProbe.Streams[0].CodecName != "av1" {
				t.Errorf("stored target probe = %#v, want AV1 video stream", stored.TargetProbe)
			}
			if stored.Status != string(model.CompletedNotificationStatus) {
				t.Errorf("stored status = %q, want completed", stored.Status)
			}
			_, sourceErr := os.Stat(sourcePath)
			if deleteSource && !os.IsNotExist(sourceErr) {
				t.Errorf("source should be deleted after completion, stat error = %v", sourceErr)
			}
			if !deleteSource && sourceErr != nil {
				t.Errorf("source should be preserved, stat error = %v", sourceErr)
			}
			if _, err := os.Stat(filepath.Join(root, stored.TargetPath)); err != nil {
				t.Errorf("final output does not exist: %v", err)
			}
			occupied, err := os.ReadFile(occupiedPath)
			if err != nil || string(occupied) != "keep existing" {
				t.Errorf("existing output was modified: contents=%q err=%v", occupied, err)
			}
			if err := scheduler.processEvent(context.Background(), &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: payload}); err != nil {
				t.Errorf("replaying completed event should be idempotent: %v", err)
			}
		})
	}
}

func TestCompletedJobMigratesLegacyStagingAcrossFilesystems(t *testing.T) {
	mediaRoot, err := os.MkdirTemp("/dev/shm", "transcoderd-media-")
	if err != nil {
		t.Skipf("/dev/shm is unavailable for cross-filesystem test: %v", err)
	}
	defer os.RemoveAll(mediaRoot)
	root := t.TempDir()
	mediaLink := filepath.Join(root, "Movies")
	if err := os.Symlink(mediaRoot, mediaLink); err != nil {
		t.Skipf("cannot create cross-filesystem media mount simulation: %v", err)
	}
	bin := t.TempDir()
	ffprobe := filepath.Join(bin, "ffprobe")
	if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\ncat <<'EOF'\n{\"format\":{\"duration\":\"12.5\"},\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\",\"width\":1920,\"height\":1080}]}\nEOF\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	id := uuid.New()
	job := &model.Job{Id: id, SourcePath: "Movies/Film x265.mkv", TargetPath: "Movies/Film x265.mkv"}
	if err := os.WriteFile(filepath.Join(mediaRoot, "Film x265.mkv"), []byte("original source"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewInMemoryRepository()
	if err := repo.AddJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	legacyStage := filepath.Join(uploadStagingDirectory(root), uploadStagingName(id.String()))
	if err := os.MkdirAll(filepath.Dir(legacyStage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyStage, []byte("encoded output"), 0o600); err != nil {
		t.Fatal(err)
	}
	scheduler := &RuntimeScheduler{config: &Config{SourcePath: root}, repo: repo}
	event := model.TaskEventType{JobId: id, EventID: 0, NotificationType: model.JobNotification, Status: model.CompletedNotificationStatus}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	envelope := &model.EnvelopEvent{EventType: model.NotificationEvent, EventData: payload}
	if err := scheduler.processEvent(context.Background(), envelope); err != nil {
		t.Fatalf("processEvent() failed to finalize cross-filesystem upload: %v", err)
	}
	stored, err := repo.GetJob(context.Background(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	if stored.TargetPath != "Movies/Film av1.mkv" {
		t.Errorf("target path = %q, want Movies/Film av1.mkv", stored.TargetPath)
	}
	if stored.TargetSize != int64(len("encoded output")) {
		t.Errorf("target size = %d, want %d", stored.TargetSize, len("encoded output"))
	}
	if _, err := os.Stat(legacyStage); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("legacy staging file was not cleaned up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mediaRoot, "Film av1.mkv")); err != nil {
		t.Errorf("final output is missing from media filesystem: %v", err)
	}
	if err := scheduler.processEvent(context.Background(), envelope); err != nil {
		t.Errorf("retry after migration should be idempotent: %v", err)
	}
}

func TestLinkOutputWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, ".job.upload")
	if err := os.WriteFile(stage, []byte("encoded"), 0o600); err != nil {
		t.Fatal(err)
	}
	desired := filepath.Join(dir, "Film.av1.mkv")
	if err := os.WriteFile(desired, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := linkOutputWithoutOverwrite(stage, desired, filepath.Join(dir, "source.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Film.av1_encoded.mkv"); got != want {
		t.Fatalf("linkOutputWithoutOverwrite() = %q, want %q", got, want)
	}
	contents, err := os.ReadFile(desired)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "existing" {
		t.Errorf("occupied destination was modified: got %q", contents)
	}
	contents, err = os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "encoded" {
		t.Errorf("alternate destination contents = %q, want encoded", contents)
	}
	if err := os.WriteFile(filepath.Join(dir, "Film.av1_encoded-2.mkv"), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Film.av1_encoded.mkv"), []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = linkOutputWithoutOverwrite(stage, desired, filepath.Join(dir, "source.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Film.av1_encoded.mkv"); got != want {
		t.Fatalf("retry linkOutputWithoutOverwrite() = %q, want existing idempotent path %q", got, want)
	}
	stage2 := filepath.Join(dir, ".another-job.upload")
	if err := os.WriteFile(stage2, []byte("second encoded"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = linkOutputWithoutOverwrite(stage2, desired, filepath.Join(dir, "another-source.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Film.av1_encoded-3.mkv"); got != want {
		t.Fatalf("numbered collision path = %q, want %q", got, want)
	}
}

func TestLinkOutputAvoidsSourcePath(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, ".job.upload")
	if err := os.WriteFile(stage, []byte("encoded"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "Film.av1.mkv")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := linkOutputWithoutOverwrite(stage, source, source)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "Film.av1_encoded.mkv"); got != want {
		t.Fatalf("linkOutputWithoutOverwrite() = %q, want %q", got, want)
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "original" {
		t.Errorf("source file was modified: got %q", contents)
	}
}

func TestVerifyFailureMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "mov_text subtitle now retries",
			message: "error: Subtitle: mov_text is not supported",
			want:    true,
		},
		{
			name:    "WebVTT codec now retries",
			message: "unsupported AVCodecID S_TEXT/WEBVTT in stream 3",
			want:    true,
		},
		{
			name:    "disk quota exceeded retries",
			message: "Disk quota exceeded",
			want:    true,
		},
		{
			name:    "no space left retries",
			message: "No space left on device",
			want:    true,
		},
		{
			name:    "job not found does not retry",
			message: "job not found",
			want:    false,
		},
		{
			name:    "index out of range does not retry",
			message: "runtime error: index out of range [3] with length 2",
			want:    false,
		},
		{
			name:    "source file size does not retry",
			message: "source File size mismatch",
			want:    false,
		},
		{
			name:    "404 download does not retry",
			message: "not 200 respose in dowload code 404",
			want:    false,
		},
		{
			name:    "mkvextract error retries",
			message: "MKVExtract unexpected error during extraction",
			want:    true,
		},
		{
			name:    "download 500 retries",
			message: "download code 500",
			want:    true,
		},
		{
			name:    "connection refused retries",
			message: "connection refused",
			want:    true,
		},
		{
			name:    "checksum error retries",
			message: "CHecksum error on download source",
			want:    true,
		},
		{
			name:    "GLIBC not found retries",
			message: "GLIBC_2.34 not found",
			want:    true,
		},
		{
			name:    "received signal 15 retries",
			message: "received signal 15",
			want:    true,
		},
		{
			name:    "bin_data does not retry",
			message: "Data: bin_data found in stream",
			want:    false,
		},
		{
			name:    "scale/rate 0/0 does not retry",
			message: "scale/rate is 0/0 which is invalid",
			want:    false,
		},
		{
			name:    "probably corrupt does not retry",
			message: "probably corrupt input file",
			want:    false,
		},
		{
			name:    "with 0 items does not retry",
			message: "completed with 0 items",
			want:    false,
		},
		{
			name:    "exit status 1 empty stderr retries",
			message: "exit status 1: stder: stdout:",
			want:    true,
		},
		{
			name:    "nil pointer retries",
			message: "runtime error: invalid memory address or nil pointer dereference",
			want:    true,
		},
		{
			name:    "maybe incorrect parameters retries",
			message: "maybe incorrect parameters such as bit_rate, rate, width or height",
			want:    true,
		},
		{
			name:    "no such host retries",
			message: "dial tcp: lookup foo.bar: no such host",
			want:    true,
		},
		{
			name:    "i/o timeout retries",
			message: "i/o timeout connecting to server",
			want:    true,
		},
		{
			name:    "Errorf while decoding does not retry",
			message: "Errorf while decoding stream #0:1",
			want:    false,
		},
		{
			name:    "source file duration does not retry",
			message: "source File duration mismatch",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyFailureMessage(tt.message)
			if got != tt.want {
				t.Errorf("verifyFailureMessage(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestFormatTargetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "x264 to x265 mkv",
			input: "movies/test.x264.avi",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "h264 to x265 mkv",
			input: "movies/test.h264.mp4",
			want:  "movies/test.x265.mkv",
		},
		{
			name:  "ac3 to AAC",
			input: "movies/test.ac3.mkv",
			want:  "movies/test.AAC.mkv",
		},
		{
			name:  "no codec match same extension returns unchanged",
			input: "movies/test.mkv",
			want:  "movies/test.mkv",
		},
		{
			name:  "no codec match different extension changes to mkv",
			input: "movies/test.avi",
			want:  "movies/test.mkv",
		},
		{
			name:  "multiple replacements",
			input: "shows/Season 1/show.x264.dts.avi",
			want:  "shows/Season 1/show.x265.AAC.mkv",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTargetName(tt.input)
			if got != tt.want {
				t.Errorf("formatTargetName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
