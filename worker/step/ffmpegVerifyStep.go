package step

import (
	"context"
	"fmt"
	"time"

	"transcoder/model"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

type FFMPEGVerifyStep struct {
	verifyDeltaTimeSeconds float64
	probeVideoDuration     func(context.Context, string) (time.Duration, error)
}

func NewFFMPEGVerifyStepExecutor(verifyDeltaTimeSeconds float64, options ...ExecutorOption) *Executor {
	verifyStep := &FFMPEGVerifyStep{
		verifyDeltaTimeSeconds: verifyDeltaTimeSeconds,
		probeVideoDuration:     ffmpeg.ProbeVideoDuration,
	}
	return NewStepExecutor(model.JobVerify, verifyStep.actions, options...)
}

func (f *FFMPEGVerifyStep) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(ctx context.Context, _ Tracker) error {
				return f.verifyJob(ctx, jobContext)
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (f *FFMPEGVerifyStep) verifyJob(ctx context.Context, jobContext *job.Context) error {
	sourceData := jobContext.Source.FFProbeData
	targetData := jobContext.Target.FFProbeData

	diffDuration := sourceData.Video.Duration.Seconds() - targetData.Video.Duration.Seconds()
	if diffDuration > f.verifyDeltaTimeSeconds || diffDuration < (-1*f.verifyDeltaTimeSeconds) {
		probeVideoDuration := f.probeVideoDuration
		if probeVideoDuration == nil {
			probeVideoDuration = ffmpeg.ProbeVideoDuration
		}
		sourceDuration, err := probeVideoDuration(ctx, jobContext.Source.FilePath)
		if err != nil {
			return fmt.Errorf("measure source video duration: %w", err)
		}
		targetDuration, err := probeVideoDuration(ctx, jobContext.Target.FilePath)
		if err != nil {
			return fmt.Errorf("measure encoded video duration: %w", err)
		}
		videoDiff := sourceDuration.Seconds() - targetDuration.Seconds()
		if videoDiff > f.verifyDeltaTimeSeconds || videoDiff < (-1*f.verifyDeltaTimeSeconds) {
			return fmt.Errorf("source video duration %f differs from encoded video duration %f", sourceDuration.Seconds(), targetDuration.Seconds())
		}
	}
	if jobContext.Target.Size > jobContext.Source.Size {
		return fmt.Errorf("source File size %d bytes is less than encoded %d bytes", jobContext.Source.Size, jobContext.Target.Size)
	}
	return nil
}
