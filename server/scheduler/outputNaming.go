package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func uploadStagingPath(sourceRoot, sourcePath, jobID string) (string, error) {
	if !filepath.IsLocal(sourcePath) {
		return "", fmt.Errorf("source path %q must be relative to the configured source root", sourcePath)
	}
	return filepath.Join(filepath.Dir(filepath.Join(sourceRoot, sourcePath)), ".transcoderd-upload", uploadStagingName(jobID)), nil
}

func uploadStagingDirectory(sourceRoot string) string {
	return filepath.Join(sourceRoot, ".transcoderd-upload")
}

// copyUploadToDestinationFilesystem migrates an upload made by older versions
// from the source root staging directory to a directory beside the source.
// The copy is written to a temporary file and linked into place only after it is synced,
// so a retry never mistakes a partial copy for a complete upload.
func copyUploadToDestinationFilesystem(sourcePath, destinationPath string) error {
	if _, err := os.Stat(destinationPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), os.ModePerm); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open legacy upload staging file: %w", err)
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destinationPath), ".transcoderd-migrate-*.tmp")
	if err != nil {
		return fmt.Errorf("create destination-local upload staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("copy legacy upload staging file: %w", err)
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync migrated upload staging file: %w", err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("close migrated upload staging file: %w", err)
	}
	if err = os.Link(temporaryPath, destinationPath); err != nil {
		if _, statErr := os.Stat(destinationPath); statErr == nil {
			return nil
		}
		return fmt.Errorf("publish migrated upload staging file: %w", err)
	}
	return nil
}

func uploadStagingName(jobID string) string {
	return jobID + ".upload"
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
