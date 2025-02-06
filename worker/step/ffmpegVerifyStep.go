package step

import (
	"fmt"
	"golang.org/x/net/context"
	"transcoder/model"
	"transcoder/worker/job"
)

type FFMPEGVerifyStep struct {
	verifyDeltaTimeSeconds float64
}

func NewFFMPEGVerifyStepExecutor(verifyDeltaTimeSeconds float64, options ...ExecutorOption) *Executor {
	verifyStep := &FFMPEGVerifyStep{
		verifyDeltaTimeSeconds: verifyDeltaTimeSeconds,
	}
	return NewStepExecutor(model.JobVerify, verifyStep.actions, options...)
}

func (f *FFMPEGVerifyStep) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(_ context.Context, _ Tracker) error {
				return f.verifyJob(jobContext)
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (f *FFMPEGVerifyStep) verifyJob(jobContext *job.Context) error {
	sourceData := jobContext.Source.FFProbeData
	targetData := jobContext.Target.FFProbeData

	diffDuration := sourceData.Video.Duration.Seconds() - targetData.Video.Duration.Seconds()
	if diffDuration > f.verifyDeltaTimeSeconds || diffDuration < (-1*f.verifyDeltaTimeSeconds) {
		err := fmt.Errorf("source File duration %f is diferent than encoded %f", sourceData.Video.Duration.Seconds(), targetData.Video.Duration.Seconds())
		return err
	}
	if jobContext.Target.Size > jobContext.Source.Size {
		return fmt.Errorf("source File size %d bytes is less than encoded %d bytes", jobContext.Source.Size, jobContext.Target.Size)
	}
	return nil
}
