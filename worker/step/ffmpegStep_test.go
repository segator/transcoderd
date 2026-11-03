package step

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

func TestFFMPEGGeneratorVideoProfiles(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.FFMPEGConfig
		want []string
	}{
		{
			name: "default NVENC",
			cfg:  config.FFMPEGConfig{},
			want: []string{"-pix_fmt", "p010le", "-c:v", "av1_nvenc", "-preset", "p7", "-tune", "uhq", "-rc", "vbr", "-cq", "38", "-b:v", "0", "-multipass", "fullres", "-bf", "4", "-b_ref_mode", "middle", "-split_encode_mode", "disabled", "-qmin", "0", "-qmax", "100"},
		},
		{
			name: "SVT CPU",
			cfg:  config.FFMPEGConfig{VideoCodec: config.AV1SVT},
			want: []string{"-pix_fmt", "yuv420p10le", "-c:v", "libsvtav1", "-preset", "4", "-crf", "25"},
		},
		{
			name: "x265 rollback",
			cfg:  config.FFMPEGConfig{VideoCodec: config.HEVCX265},
			want: []string{"-pix_fmt", "yuv420p10le", "-c:v", "libx265", "-crf", "21", "-profile:v", "main10", "-preset", "medium"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := FFMPEGGenerator{Config: &tt.cfg}
			if err := generator.setVideoFilters(&ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Id: 2}}); err != nil {
				t.Fatalf("setVideoFilters() error = %v", err)
			}
			wantPrefix := []string{"-map", "0:2", "-avoid_negative_ts", "make_zero", "-copyts", "-map_chapters", "-1", "-flags", "+global_header", "-filter:v", "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"}
			want := slices.Concat(wantPrefix, tt.want)
			if !reflect.DeepEqual(generator.VideoFilter, want) {
				t.Errorf("setVideoFilters() = %v, want %v", generator.VideoFilter, want)
			}
			if slices.Contains(generator.VideoFilter, "-hwaccel") {
				t.Error("CPU scale profile unexpectedly enabled hardware decode")
			}
		})
	}
}

func TestFFMPEGExecutionUsesPinnedProfileForEveryOption(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "arguments")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ENCODER_TEST_ARGS\"\nexit 23\n"
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ENCODER_TEST_ARGS", argsPath)
	tests := []struct {
		name             string
		fallback, pinned config.FFMPEGConfig
		want, absent     []string
	}{
		{name: "CPU snapshot on GPU primary", fallback: config.FFMPEGConfig{VideoCodec: config.AV1NVENC, Threads: 11, ExtraArgs: "-gpu_only enabled"}, pinned: config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 27, Threads: 3, ExtraArgs: "-cpu_only enabled"}, want: []string{"-c:v", "libsvtav1", "-threads", "3", "-crf", "27", "-cpu_only", "enabled"}, absent: []string{"-cq", "-gpu_only", "av1_nvenc"}},
		{name: "GPU snapshot on CPU primary", fallback: config.FFMPEGConfig{VideoCodec: config.HEVCX265, Threads: 12, ExtraArgs: "-cpu_only enabled"}, pinned: config.FFMPEGConfig{VideoCodec: config.AV1NVENC, VideoCQ: 42, Threads: 5, ExtraArgs: "-gpu_only enabled"}, want: []string{"-c:v", "av1_nvenc", "-threads", "5", "-cq", "42", "-gpu_only", "enabled"}, absent: []string{"-crf", "-cpu_only", "libx265"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &job.Context{WorkingDir: dir, EncodeConfig: &tt.pinned, Source: &job.VideoData{FilePath: filepath.Join(dir, "source.mkv"), FFProbeData: &ffmpeg.NormalizedFFProbe{Video: &ffmpeg.Video{Id: 0, Duration: time.Minute, FrameRate: 24}}}}
			executor := &FFMPEGStepExecutor{ffmpegConfig: &tt.fallback}
			if err := executor.ffmpeg(t.Context(), &mockLogger{}, j, make(chan ffmpegProgress, 1)); err == nil {
				t.Fatal("capture fixture did not terminate FFmpeg as expected")
			}
			raw, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSpace(string(raw)), "\n")
			for _, want := range tt.want {
				if !slices.Contains(args, want) {
					t.Errorf("pinned invocation missing %q: %v", want, args)
				}
			}
			for _, absent := range tt.absent {
				if slices.Contains(args, absent) {
					t.Errorf("pinned invocation leaked %q: %v", absent, args)
				}
			}
		})
	}
}

func TestFFMPEGGeneratorKeepsPathsAndStreamMapping(t *testing.T) {
	cfg := &config.FFMPEGConfig{AudioCodec: "libfdk_aac", AudioVBR: 5}
	generator := FFMPEGGenerator{Config: cfg, inputPaths: []string{"/media/A film (2024).mkv", "/tmp/sub title.srt"}}
	container := &ffmpeg.NormalizedFFProbe{
		Video:    &ffmpeg.Video{Id: 0, Duration: time.Minute, FrameRate: 24},
		Audios:   []*ffmpeg.Audio{{Id: 1, Language: "eng", ChannelLayour: "5.1"}},
		Subtitle: []*ffmpeg.Subtitle{{Id: 2, Language: "spa", Format: "S_HDMV/PGS", Title: "Spanish PGS"}},
	}
	if err := generator.setVideoFilters(container); err != nil {
		t.Fatal(err)
	}
	generator.setAudioFilters(container)
	generator.setSubtFilters(container)
	args := generator.buildArguments(0, "", "/tmp/output film.mkv")
	for _, want := range [][]string{
		{"-i", "/media/A film (2024).mkv"},
		{"-i", "/tmp/sub title.srt"},
		{"-map", "0:0"},
		{"-map", "0:1", "-metadata:s:a:0", "title=eng (5.1)", "-c:a:0", "libfdk_aac", "-vbr", "5"},
		{"-map", "1:0", "-c:s:0", "srt"},
		{"-metadata:s:s:0", "language=spa", "-metadata:s:s:0", "title=Spanish"},
		{"-y", "/tmp/output film.mkv"},
	} {
		if !containsSequence(args, want) {
			t.Errorf("arguments %v missing sequence %v", args, want)
		}
	}
}

func TestFFMPEGGeneratorMapsMovTextDirectlyWithMixedSubtitleInputs(t *testing.T) {
	workingDir := t.TempDir()
	sourcePath := "/media/source.mp4"
	container := &ffmpeg.NormalizedFFProbe{
		Video: &ffmpeg.Video{Id: 0},
		Subtitle: []*ffmpeg.Subtitle{
			{Id: 2, Format: "none", Language: "fre", Title: "French WebVTT"},
			{Id: 3, Format: "mov_text", Language: "spa", Title: "Spanish", Forced: true},
			{Id: 4, Format: "hdmv_pgs_subtitle", Language: "eng", Title: "English PGS"},
		},
	}
	generator := FFMPEGGenerator{Config: &config.FFMPEGConfig{AudioCodec: "aac"}}
	generator.setInputFilters(&job.Context{
		WorkingDir: workingDir,
		Source:     &job.VideoData{FilePath: sourcePath, FFProbeData: container},
	})
	generator.setSubtFilters(container)

	wantInputs := []string{sourcePath, filepath.Join(workingDir, "2.srt"), filepath.Join(workingDir, "4.srt")}
	if !slices.Equal(generator.inputPaths, wantInputs) {
		t.Fatalf("input paths = %v, want %v", generator.inputPaths, wantInputs)
	}
	args := generator.buildArguments(0, "", filepath.Join(workingDir, "output.mkv"))
	for _, want := range [][]string{
		{"-map", "1:0", "-c:s:0", "srt"},
		{"-map", "0:3", "-c:s:1", "srt", "-disposition:s:1", "forced+default"},
		{"-metadata:s:s:1", "language=spa", "-metadata:s:s:1", "title=Spanish"},
		{"-map", "2:0", "-c:s:2", "srt"},
	} {
		if !containsSequence(args, want) {
			t.Errorf("arguments %v missing sequence %v", args, want)
		}
	}
	if slices.Contains(args, filepath.Join(workingDir, "3.srt")) {
		t.Errorf("mov_text should not use an extracted SRT input: %v", args)
	}
}

func containsSequence(args, sequence []string) bool {
	for i := 0; i+len(sequence) <= len(args); i++ {
		if slices.Equal(args[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
