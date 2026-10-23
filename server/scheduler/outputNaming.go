package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Tags      struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
	} `json:"streams"`
}

func probeOutput(ctx context.Context, path string) (ffprobeOutput, error) {
	var probe ffprobeOutput
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration:stream=codec_type,codec_name,width,height:stream_tags=language,title", "-of", "json", path).Output()
	if err != nil {
		return probe, fmt.Errorf("probe uploaded output: %w", err)
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return probe, fmt.Errorf("decode ffprobe output: %w", err)
	}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" && stream.CodecName != "" {
			return probe, nil
		}
	}
	return probe, errors.New("uploaded output has no video stream")
}

func uploadStagingPath(sourceRoot string, jobID string) string {
	return filepath.Join(sourceRoot, ".transcoderd-upload", jobID+".upload")
}

func outputCodecLabel(codec string) string {
	switch strings.ToLower(codec) {
	case "av1":
		return "av1"
	case "hevc", "h265":
		return "x265"
	case "h264", "avc":
		return "x264"
	default:
		return strings.ToLower(codec)
	}
}

func outputNameForCodec(sourcePath, codec string) string {
	label := outputCodecLabel(codec)
	dir, file := filepath.Split(sourcePath)
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	base = ac3ex.ReplaceAllString(base, "AAC")
	if videoCodecNameEx.MatchString(base) {
		base = videoCodecNameEx.ReplaceAllString(base, `${1}`+label+`${3}`)
	} else {
		base += "." + label
	}
	return filepath.Join(dir, base+".mkv")
}

// linkOutputWithoutOverwrite reserves a final output name atomically. A collision
// is handled by trying _encoded, _encoded-2, and subsequent numbered names.
func linkOutputWithoutOverwrite(stagingPath, desiredPath, sourcePath string) (string, error) {
	dir := filepath.Dir(desiredPath)
	ext := filepath.Ext(desiredPath)
	base := strings.TrimSuffix(filepath.Base(desiredPath), ext)
	for index := 0; ; index++ {
		candidate := desiredPath
		if index == 1 {
			candidate = filepath.Join(dir, base+"_encoded"+ext)
		} else if index > 1 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_encoded-%d%s", base, index, ext))
		}
		if samePath(candidate, sourcePath) {
			continue
		}
		if err := os.Link(stagingPath, candidate); err != nil {
			if errors.Is(err, os.ErrExist) {
				stageInfo, stageErr := os.Stat(stagingPath)
				candidateInfo, candidateErr := os.Stat(candidate)
				if stageErr == nil && candidateErr == nil && os.SameFile(stageInfo, candidateInfo) {
					return candidate, nil
				}
				continue
			}
			return "", fmt.Errorf("reserve final output %q: %w", candidate, err)
		}
		return candidate, nil
	}
}

func samePath(first, second string) bool {
	firstAbs, firstErr := filepath.Abs(first)
	secondAbs, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && filepath.Clean(firstAbs) == filepath.Clean(secondAbs)
}
