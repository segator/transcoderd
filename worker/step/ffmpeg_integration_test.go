//go:build integration

package step

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		{"CPU", config.AV1SVT, 0},
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

func TestGeneratedAV1CommandConvertsMP4MovTextToSRT(t *testing.T) {
	ffmpegBinary := os.Getenv("AV1_TEST_FFMPEG")
	if ffmpegBinary == "" {
		t.Skip("set AV1_TEST_FFMPEG to a build with libsvtav1 and mov_text support")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	workingDir := t.TempDir()
	subtitlePath := filepath.Join(workingDir, "source.srt")
	if err := os.WriteFile(subtitlePath, []byte("1\n00:00:00,500 --> 00:00:01,500\nDirect mov_text conversion works\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workingDir, "source.mp4")
	createSource := exec.CommandContext(ctx, ffmpegBinary,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:r=24:d=2",
		"-i", subtitlePath, "-map", "0:v:0", "-map", "1:0",
		"-t", "2", "-c:v", "mpeg4", "-q:v", "5", "-c:s", "mov_text",
		"-metadata:s:s:0", "language=spa", "-metadata:s:s:0", "title=Spanish",
		source)
	if output, err := createSource.CombinedOutput(); err != nil {
		t.Fatalf("could not create MP4 mov_text fixture: %v\n%s", err, output)
	}

	probe, err := ffmpeg.ExtractFFProbeData(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := ffmpeg.NormalizeFFProbeData(probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Subtitle) != 1 || !normalized.Subtitle[0].NeedsDirectSRTConversion() {
		t.Fatalf("source subtitle streams = %+v, want one mov_text stream", normalized.Subtitle)
	}
	if normalized.HaveExtractableSubtitle() {
		t.Fatal("mov_text-only MP4 should not require the MKVExtract phase")
	}

	config := &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoCRF: 35}
	generator := FFMPEGGenerator{Config: config}
	jobContext := &job.Context{WorkingDir: workingDir, Source: &job.VideoData{FilePath: source, FFProbeData: normalized}}
	generator.setInputFilters(jobContext)
	if len(generator.inputPaths) != 1 || generator.inputPaths[0] != source {
		t.Fatalf("FFmpeg inputs = %v, want source only (no extracted subtitle input)", generator.inputPaths)
	}
	if err := generator.setVideoFilters(normalized); err != nil {
		t.Fatal(err)
	}
	generator.setAudioFilters(normalized)
	generator.setSubtFilters(normalized)
	output := filepath.Join(workingDir, "output.mkv")
	encode := exec.CommandContext(ctx, ffmpegBinary, generator.buildArguments(0, "", output)...)
	if outputBytes, err := encode.CombinedOutput(); err != nil {
		t.Fatalf("generated FFmpeg command failed: %v\n%s", err, outputBytes)
	}

	outputProbe, err := ffmpeg.ExtractFFProbeData(ctx, output)
	if err != nil {
		t.Fatal(err)
	}
	outputNormalized, err := ffmpeg.NormalizeFFProbeData(outputProbe)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputNormalized.Subtitle) != 1 || outputNormalized.Subtitle[0].Format != "subrip" {
		t.Fatalf("output subtitle streams = %+v, want one subrip stream", outputNormalized.Subtitle)
	}
	if outputNormalized.Subtitle[0].Language != "spa" {
		t.Errorf("output subtitle language = %q, want spa", outputNormalized.Subtitle[0].Language)
	}
	extracted, err := exec.CommandContext(ctx, ffmpegBinary, "-hide_banner", "-loglevel", "error", "-i", output,
		"-map", "0:s:0", "-f", "srt", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("could not decode output SRT track: %v\n%s", err, extracted)
	}
	if !strings.Contains(string(extracted), "Direct mov_text conversion works") {
		t.Errorf("decoded subtitle text %q does not contain fixture text", extracted)
	}
	decoded, err := exec.CommandContext(ctx, ffmpegBinary, "-hide_banner", "-loglevel", "error", "-i", output, "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Errorf("output did not fully decode: %v\n%s", err, decoded)
	}
}
