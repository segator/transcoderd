//go:build integration

package step

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

func TestGeneratedAV1CommandsEncodeAndDecode(t *testing.T) {
	ffmpegBinary := os.Getenv("AV1_TEST_FFMPEG")
	if ffmpegBinary == "" {
		t.Skip("set AV1_TEST_FFMPEG to a build with libsvtav1, av1_nvenc, and libfdk_aac")
	}
	source, err := filepath.Abs("../../integration/testdata/e2e_fixture.mkv")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	probe, err := ffmpeg.ExtractFFProbeData(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := ffmpeg.NormalizeFFProbeData(probe)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		codec string
		crf   int
	}{
		{"CPU", config.AV1SVT, 35},
	}
	if os.Getenv("AV1_TEST_GPU") == "1" {
		tests = append(tests, struct {
			name  string
			codec string
			crf   int
		}{"GPU", config.AV1NVENC, 0})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := t.TempDir()
			for _, subtitle := range normalized.Subtitle {
				if !subtitle.IsImageTypeSubtitle() {
					continue
				}
				path := filepath.Join(workingDir, fmt.Sprintf("%d.srt", subtitle.Id))
				if err := os.WriteFile(path, []byte("1\n00:00:00,000 --> 00:00:01,000\nAV1 fixture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.FFMPEGConfig{VideoCodec: tt.codec, VideoCRF: tt.crf, AudioCodec: "libfdk_aac", AudioVBR: 5}
			generator := FFMPEGGenerator{Config: cfg}
			jobContext := &job.Context{WorkingDir: workingDir, Source: &job.VideoData{FilePath: source, FFProbeData: normalized}}
			generator.setInputFilters(jobContext)
			if err := generator.setVideoFilters(normalized); err != nil {
				t.Fatal(err)
			}
			generator.setAudioFilters(normalized)
			generator.setSubtFilters(normalized)
			output := filepath.Join(workingDir, "av1 output.mkv")
			args := generator.buildArguments(0, "", output)
			encoded, err := exec.CommandContext(ctx, ffmpegBinary, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("generated FFmpeg command failed: %v\n%s", err, encoded)
			}
			var result struct {
				Streams []struct {
					CodecType string `json:"codec_type"`
					CodecName string `json:"codec_name"`
					PixFmt    string `json:"pix_fmt"`
				} `json:"streams"`
				Format struct {
					Duration string `json:"duration"`
				} `json:"format"`
			}
			probeJSON, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", output).Output()
			if err != nil {
				t.Fatalf("ffprobe output: %v", err)
			}
			if err := json.Unmarshal(probeJSON, &result); err != nil {
				t.Fatal(err)
			}
			counts := make(map[string]int)
			for _, stream := range result.Streams {
				counts[stream.CodecType+":"+stream.CodecName]++
				if stream.CodecType == "video" && stream.PixFmt != "yuv420p10le" {
					t.Errorf("video pixel format = %q, want yuv420p10le", stream.PixFmt)
				}
			}
			if counts["video:av1"] != 1 || counts["audio:aac"] != 2 || counts["subtitle:subrip"] != 4 {
				t.Errorf("output stream counts = %v", counts)
			}
			if result.Format.Duration == "" {
				t.Error("output has no duration")
			}
			decoded, err := exec.CommandContext(ctx, ffmpegBinary, "-v", "error", "-i", output, "-map", "0:v:0", "-f", "null", "-").CombinedOutput()
			if err != nil {
				t.Errorf("AV1 output did not decode: %v\n%s", err, decoded)
			}
		})
	}
}
