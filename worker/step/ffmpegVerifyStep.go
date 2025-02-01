package step

import (
	"fmt"
	"transcoder/worker/job"
)

type FFMPEGVerifyStep struct {
	verifyDeltaTimeSeconds float64
}

func NewFFMPEGVerifyStepExecutor(verifyDeltaTimeSeconds float64) *FFMPEGVerifyStep {
	return &FFMPEGVerifyStep{
		verifyDeltaTimeSeconds: verifyDeltaTimeSeconds,
	}
}

func (f *FFMPEGVerifyStep) Execute(jobContext *job.Context) error {
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
