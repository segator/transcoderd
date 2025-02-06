package step

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
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

	for _, subtitle := range jobContext.Source.FFProbeData.GetPGSSubtitles() {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.sup", subtitle.Id, subtitle.Id))
	}

	_, err := mkvExtractCommand.RunWithContext(ctx, command.NewAllowedCodesOption(0, 1))
	if err != nil {
		tracker.Logger().Cmdf("MKVExtract Command:%s", mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v", err)
	}

	return nil
}
