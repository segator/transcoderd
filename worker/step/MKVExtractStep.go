package step

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/worker/job"
)

type MKVExtractStepExecutor struct {
}

func NewMKVExtractStepExecutor() *MKVExtractStepExecutor {
	return &MKVExtractStepExecutor{}
}

func (e *MKVExtractStepExecutor) Execute(ctx context.Context, stepTracker Tracker, jobContext *job.Context) error {
	mkvExtractCommand := command.NewCommand(helper.GetMKVExtractPath(), "tracks", jobContext.Source.FilePath).
		SetWorkDir(jobContext.WorkingDir)
	if runtime.GOOS == "linux" {
		mkvExtractCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetMKVExtractPath())))
	}
	for _, subtitle := range jobContext.Source.FFProbeData.GetPGSSubtitles() {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.sup", subtitle.Id, subtitle.Id))
	}

	_, err := mkvExtractCommand.RunWithContext(ctx, command.NewAllowedCodesOption(0, 1))
	if err != nil {
		stepTracker.Logger().Cmdf("MKVExtract Command:%s", mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v", err)
	}

	return nil
}
