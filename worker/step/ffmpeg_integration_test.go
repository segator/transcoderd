//go:build integration

package step

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

func TestGeneratedNVDECPipelineRealMedia(t *testing.T) {
	binary := os.Getenv("AV1_TEST_FFMPEG")
	if binary == "" {
		t.Skip("set AV1_TEST_FFMPEG")
	}
	for _, variable := range []string{"AV1_TEST_SOURCE_8BIT", "AV1_TEST_SOURCE_10BIT"} {
		source := os.Getenv(variable)
		if source == "" {
			t.Skip("set AV1_TEST_SOURCE_8BIT and AV1_TEST_SOURCE_10BIT")
		}
		t.Run(variable, func(t *testing.T) {
			probe, err := ffmpeg.ExtractFFProbeData(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			media, err := ffmpeg.NormalizeFFProbeData(probe)
			if err != nil {
				t.Fatal(err)
			}
			profile := &config.FFMPEGConfig{}
			g := FFMPEGGenerator{Config: profile, inputPaths: []string{source}, hardwareDecode: useHardwareDecode(profile, media.Video)}
			if !g.hardwareDecode {
				t.Fatalf("source not eligible: %+v", media.Video)
			}
			if err := g.setVideoFilters(media); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "result.mkv")
			args := g.buildArguments(0, "", output)
			args = slices.Insert(args, len(args)-1, "-frames:v", "96")
			if out, err := exec.CommandContext(t.Context(), binary, args...).CombinedOutput(); err != nil {
				t.Fatalf("NVDEC encode: %v\n%s", err, out)
			}
			result, err := ffmpeg.ExtractFFProbeData(t.Context(), output)
			if err != nil {
				t.Fatal(err)
			}
			stream := result.StreamType("video")[0]
			if stream.CodecName != "av1" || stream.PixFmt != "yuv420p10le" || stream.Width != media.Video.Width {
				t.Fatalf("unexpected output: %+v", stream)
			}
			if out, err := exec.CommandContext(t.Context(), binary, "-v", "error", "-xerror", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
		})
	}
}

func TestGeneratedGPU4KUsesSoftwareDecodeAndScales(t *testing.T) {
	binary, source := os.Getenv("AV1_TEST_FFMPEG"), os.Getenv("AV1_TEST_SOURCE_4K")
	if binary == "" || source == "" {
		t.Skip("set AV1_TEST_FFMPEG and AV1_TEST_SOURCE_4K")
	}
	probe, err := ffmpeg.ExtractFFProbeData(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	media, err := ffmpeg.NormalizeFFProbeData(probe)
	if err != nil {
		t.Fatal(err)
	}
	profile := &config.FFMPEGConfig{}
	g := FFMPEGGenerator{Config: profile, inputPaths: []string{source}, hardwareDecode: useHardwareDecode(profile, media.Video)}
	if g.hardwareDecode {
		t.Fatal("4K should use software decoding")
	}
	if err := g.setVideoFilters(media); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "result.mkv")
	args := g.buildArguments(0, "", output)
	args = slices.Insert(args, len(args)-1, "-frames:v", "48")
	if out, err := exec.CommandContext(t.Context(), binary, args...).CombinedOutput(); err != nil {
		t.Fatalf("4K encode: %v\n%s", err, out)
	}
	result, err := ffmpeg.ExtractFFProbeData(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	stream := result.StreamType("video")[0]
	if stream.CodecName != "av1" || stream.PixFmt != "yuv420p10le" || stream.Width != 1920 {
		t.Fatalf("unexpected output: %+v", stream)
	}
	if out, err := exec.CommandContext(t.Context(), binary, "-v", "error", "-xerror", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
}

func TestGeneratedNVDECPreservesHDRColorMetadata(t *testing.T) {
	binary, source := os.Getenv("AV1_TEST_FFMPEG"), os.Getenv("AV1_TEST_SOURCE_HDR")
	if binary == "" || source == "" {
		t.Skip("set AV1_TEST_FFMPEG and AV1_TEST_SOURCE_HDR")
	}
	input := filepath.Join(t.TempDir(), "hdr-hevc.mkv")
	args := []string{"-v", "error", "-i", source, "-frames:v", "24", "-an", "-vf", "scale=1920:1080", "-pix_fmt", "yuv420p10le", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "pools=2:frame-threads=1:log-level=error", input}
	if out, err := exec.CommandContext(t.Context(), binary, args...).CombinedOutput(); err != nil {
		t.Fatalf("create HDR source: %v\n%s", err, out)
	}
	probe, err := ffmpeg.ExtractFFProbeData(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	media, err := ffmpeg.NormalizeFFProbeData(probe)
	if err != nil {
		t.Fatal(err)
	}
	original := probe.StreamType("video")[0]
	if original.ColorTransfer != "smpte2084" || original.ColorPrimaries != "bt2020" || original.ColorSpace != "bt2020nc" {
		t.Fatalf("input HDR metadata = %+v", original)
	}
	profile := &config.FFMPEGConfig{}
	g := FFMPEGGenerator{Config: profile, inputPaths: []string{input}, hardwareDecode: useHardwareDecode(profile, media.Video)}
	if !g.hardwareDecode {
		t.Fatalf("HDR source not eligible: %+v", media.Video)
	}
	if err := g.setVideoFilters(media); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "hdr-av1.mkv")
	if out, err := exec.CommandContext(t.Context(), binary, g.buildArguments(0, "", output)...).CombinedOutput(); err != nil {
		t.Fatalf("HDR NVDEC encode: %v\n%s", err, out)
	}
	result, err := ffmpeg.ExtractFFProbeData(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	stream := result.StreamType("video")[0]
	if stream.PixFmt != "yuv420p10le" || stream.ColorTransfer != original.ColorTransfer || stream.ColorPrimaries != original.ColorPrimaries || stream.ColorSpace != original.ColorSpace {
		t.Fatalf("output HDR metadata = %+v", stream)
	}
	if out, err := exec.CommandContext(t.Context(), binary, "-v", "error", "-xerror", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("decode HDR: %v\n%s", err, out)
	}
}

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
			generator := FFMPEGGenerator{Config: cfg, hardwareDecode: useHardwareDecode(cfg, normalized.Video)}
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

func TestGeneratedAV1CommandConvertsEIA608ToSRT(t *testing.T) {
	ffmpegBinary := os.Getenv("AV1_TEST_FFMPEG")
	if ffmpegBinary == "" {
		t.Skip("set AV1_TEST_FFMPEG to an SVT-AV1 build")
	}
	dir := t.TempDir()
	scc := filepath.Join(dir, "captions.scc")
	if err := os.WriteFile(scc, []byte("Scenarist_SCC V1.0\n\n00:00:01:00\t9420 9420 94ae 94ae c8e9 942f 942f\n\n00:00:02:00\t942c 942c\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "captions.mov")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=black:s=128x72:r=25:d=3", "-i", scc,
		"-map", "0:v", "-map", "1:0", "-c:v", "mpeg4", "-c:s", "copy", "-metadata:s:s:0", "language=eng", "-y", source}
	if out, err := exec.CommandContext(t.Context(), ffmpegBinary, args...).CombinedOutput(); err != nil {
		t.Fatalf("create caption fixture: %v: %s", err, out)
	}
	probe, err := ffmpeg.ExtractFFProbeData(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := ffmpeg.NormalizeFFProbeData(probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.Subtitle) != 1 || normalized.Subtitle[0].Format != "eia_608" || normalized.HaveExtractableSubtitle() {
		t.Fatalf("caption fixture = %+v, want direct EIA-608", normalized.Subtitle)
	}
	normalized.Subtitle[0].Forced, normalized.Subtitle[0].Comment = true, true
	normalized.Subtitle[0].Title = "Closed captions"
	g := FFMPEGGenerator{Config: &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoPreset: "12", VideoCRF: 35}}
	g.setInputFilters(&job.Context{WorkingDir: dir, Source: &job.VideoData{FilePath: source, FFProbeData: normalized}})
	if err := g.setVideoFilters(normalized); err != nil {
		t.Fatal(err)
	}
	g.setSubtFilters(normalized)
	output := filepath.Join(dir, "output.mkv")
	if out, err := exec.CommandContext(t.Context(), ffmpegBinary, g.buildArguments(1, "", output)...).CombinedOutput(); err != nil {
		t.Fatalf("encode captions: %v: %s", err, out)
	}
	result, err := ffmpeg.ExtractFFProbeData(t.Context(), output)
	if err != nil {
		t.Fatal(err)
	}
	streams := result.StreamType("subtitle")
	if len(streams) != 1 {
		t.Fatalf("output subtitle count = %d, want 1", len(streams))
	}
	s := streams[0]
	language, _ := s.TagList.GetString("language")
	title, _ := s.TagList.GetString("title")
	if s.CodecName != "subrip" || language != "eng" || title != "Closed captions" || s.Disposition.Forced != 1 || s.Disposition.Comment != 1 {
		t.Fatalf("output caption metadata = %+v", s)
	}
	text, err := exec.CommandContext(t.Context(), ffmpegBinary, "-v", "error", "-i", output, "-map", "0:s", "-f", "srt", "-").CombinedOutput()
	if err != nil || !strings.Contains(string(text), "Hi") {
		t.Fatalf("decoded captions = %s, error %v", text, err)
	}
	if out, err := exec.CommandContext(t.Context(), ffmpegBinary, "-v", "error", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("decode output: %v: %s", err, out)
	}
}

func TestGeneratedAV1CommandPreservesValidColorsAndClearsInvalidMatrix(t *testing.T) {
	ffmpegBinary := os.Getenv("AV1_TEST_FFMPEG")
	if ffmpegBinary == "" {
		t.Skip("set AV1_TEST_FFMPEG to an SVT-AV1 build")
	}
	tests := []struct{ name, matrix, primaries, transfer, wantMatrix string }{
		{"invalid YUV identity", "0", "bt709", "bt709", "unknown"},
		{"SDR", "bt709", "bt709", "bt709", "bt709"},
		{"HDR", "bt2020nc", "bt2020", "smpte2084", "bt2020nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.mkv")
			args := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=black:s=128x72:r=24:d=1", "-c:v", "ffv1", "-pix_fmt", "yuv420p10le",
				"-vf", "setparams=colorspace=" + tt.matrix + ":color_primaries=" + tt.primaries + ":color_trc=" + tt.transfer, "-y", source}
			if out, err := exec.CommandContext(t.Context(), ffmpegBinary, args...).CombinedOutput(); err != nil {
				t.Fatalf("create color fixture: %v: %s", err, out)
			}
			probe, err := ffmpeg.ExtractFFProbeData(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			normalized, err := ffmpeg.NormalizeFFProbeData(probe)
			if err != nil {
				t.Fatal(err)
			}
			g := FFMPEGGenerator{Config: &config.FFMPEGConfig{VideoCodec: config.AV1SVT, VideoPreset: "12", VideoCRF: 35}}
			g.setInputFilters(&job.Context{WorkingDir: dir, Source: &job.VideoData{FilePath: source, FFProbeData: normalized}})
			if err := g.setVideoFilters(normalized); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "output.mkv")
			if out, err := exec.CommandContext(t.Context(), ffmpegBinary, g.buildArguments(1, "", output)...).CombinedOutput(); err != nil {
				t.Fatalf("encode colors: %v: %s", err, out)
			}
			result, err := ffmpeg.ExtractFFProbeData(t.Context(), output)
			if err != nil {
				t.Fatal(err)
			}
			v := result.StreamType("video")[0]
			if (v.ColorSpace != tt.wantMatrix && !(tt.wantMatrix == "unknown" && v.ColorSpace == "")) || v.ColorPrimaries != tt.primaries || v.ColorTransfer != tt.transfer {
				t.Fatalf("output colors = %s/%s/%s, want %s/%s/%s", v.ColorSpace, v.ColorPrimaries, v.ColorTransfer, tt.wantMatrix, tt.primaries, tt.transfer)
			}
			if out, err := exec.CommandContext(t.Context(), ffmpegBinary, "-v", "error", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil {
				t.Fatalf("decode output: %v: %s", err, out)
			}
		})
	}
}
