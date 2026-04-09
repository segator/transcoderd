package step

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/job"
)

type MKVExtractStepExecutor struct {
}

func NewMKVExtractStepExecutor(options ...ExecutorOption) *Executor {
	mkvStep := &MKVExtractStepExecutor{}
	return NewStepExecutor(model.MKVExtractNotification, mkvStep.actions, options...)
}

func (m *MKVExtractStepExecutor) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(ctx context.Context, stepTracker Tracker) error {
				return m.mkvExtract(ctx, stepTracker, jobContext)
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (m *MKVExtractStepExecutor) mkvExtract(ctx context.Context, tracker Tracker, jobContext *job.Context) error {
	tracker.SetTotal(100)
	var outLog string
	progressRegex := regexp.MustCompile(`Progress: (\d+)%`)
	mkvExtractCommand := command.NewCommand(helper.GetMKVExtractPath(), "tracks", jobContext.Source.FilePath).
		SetWorkDir(jobContext.WorkingDir).
		BuffSize(128).
		SetStdoutFunc(func(buffer []byte, exit bool) {
			str := string(buffer)
			outLog += str
			progressMatch := progressRegex.FindStringSubmatch(str)
			if len(progressMatch) > 0 {
				p, err := strconv.Atoi(progressMatch[len(progressMatch)-1])
				if err != nil {
					return
				}
				tracker.UpdateValue(int64(p))
			}
		})

	mkvExtractCommand.AddEnv("LC_ALL=C")
	mkvExtractCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetMKVExtractPath())))

	// Extract PGS subtitle tracks as .sup files for OCR processing
	for _, subtitle := range jobContext.Source.FFProbeData.GetPGSSubtitles() {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.sup", subtitle.Id, subtitle.Id))
	}

	// Extract unsupported codec subtitle tracks (e.g. S_TEXT/WEBVTT) as raw files
	// mkvextract will output the raw subtitle data which we'll convert to SRT afterwards
	for _, subtitle := range jobContext.Source.FFProbeData.GetExtractableSubtitles() {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.extract", subtitle.Id, subtitle.Id))
	}

	_, err := mkvExtractCommand.RunWithContext(ctx, command.NewAllowedCodesOption(0, 1))
	if err != nil {
		tracker.Logger().Cmdf("MKVExtract Command:%s", mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v", err)
	}

	// Convert extracted subtitle tracks to SRT using FFmpeg
	for _, subtitle := range jobContext.Source.FFProbeData.GetExtractableSubtitles() {
		if err := m.convertExtractedToSrt(ctx, tracker, jobContext, subtitle.Id); err != nil {
			return err
		}
	}

	return nil
}

// convertExtractedToSrt converts an extracted subtitle file to SRT using FFmpeg.
// The extracted file (e.g. WebVTT data) is read by FFmpeg as a standalone file,
// bypassing the MKV demuxer codec ID limitation.
func (m *MKVExtractStepExecutor) convertExtractedToSrt(ctx context.Context, tracker Tracker, jobContext *job.Context, trackId uint8) error {
	inputPath := filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%d.extract", trackId))
	outputPath := filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%d.srt", trackId))

	ffmpegCmd := command.NewCommand(helper.GetFFmpegPath(), "-i", inputPath, "-c:s", "srt", outputPath, "-y").
		SetWorkDir(jobContext.WorkingDir)

	if runtime.GOOS == "linux" {
		ffmpegCmd.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetFFmpegPath())))
	}

	tracker.Logger().Cmdf("Converting extracted subtitle to SRT: %s %s", helper.GetFFmpegPath(), ffmpegCmd.GetFullCommand())
	exitCode, err := ffmpegCmd.RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to convert extracted subtitle track %d to SRT: %v", trackId, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("ffmpeg subtitle conversion for track %d exited with code %d", trackId, exitCode)
	}

	return nil
}
