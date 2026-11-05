package worker

import (
	"context"
	"errors"
	"fmt"

	"transcoder/model"
	"transcoder/worker/config"
)

// Called under settingsMu: local capacity writes and remote command delivery
// share the same serialization boundary. A lost acknowledgement never replays.
func (q *ServerCoordinator) applyRemoteControl(ctx context.Context, c *model.WorkerControl) error {
	if q.appliedControlID != c.ID {
		err := q.executeRemoteControl(ctx, c)
		q.appliedControlID = c.ID
		q.appliedControlError = ""
		if err != nil {
			q.appliedControlError = "Worker could not apply control: " + err.Error()
			if len(q.appliedControlError) > 512 {
				q.appliedControlError = q.appliedControlError[:512]
			}
		}
	}
	if q.appliedControlError == "" {
		q.worker.MarkWorkerSettingsResolved()
	}
	if err := q.serverClient.PublishPingEventContext(ctx, q.worker.RuntimeTelemetry()); err != nil {
		return err
	}
	ack := model.WorkerControlAck{ID: c.ID, Error: q.appliedControlError}
	if err := q.serverClient.AckWorkerControl(ctx, ack); err != nil {
		return err
	}
	if q.appliedControlError != "" {
		return errors.New(q.appliedControlError)
	}
	q.worker.MarkWorkerSettingsResolved()
	return nil
}
func (q *ServerCoordinator) executeRemoteControl(ctx context.Context, c *model.WorkerControl) error {
	if err := model.ValidateWorkerAction(c.Action); err != nil {
		return err
	}
	status := q.worker.AdmissionStatus()
	if status.Draining || (status.Finishing && c.Action != "resume-queue") {
		return errors.New("worker is draining accepted work")
	}
	switch c.Action {
	case "pause-encodes":
		return q.worker.SetRAMPaused(true)
	case "resume-encodes":
		return q.worker.SetRAMPaused(false)
	case "stop-after-current", "resume-queue":
		settings := c.Settings()
		if c.Action == "stop-after-current" {
			zeroCPU, zeroGPU := 0, 0
			settings = model.WorkerSettings{CPUJobs: &zeroCPU, GPUJobs: &zeroGPU}
		}
		if err := settings.ValidateWrite(); err != nil {
			return fmt.Errorf("invalid queue limits: %w", err)
		}
		if err := q.serverClient.SetWorkerSettings(ctx, settings); err != nil {
			return err
		}
		if err := q.worker.SetPauseMode(config.PauseFinish); err != nil {
			return err
		}
		if settings.Independent() {
			return q.worker.SetEncoderLimits(*settings.CPUJobs, *settings.GPUJobs)
		}
		return q.worker.SetMaxActiveJobs(*settings.MaxActiveJobs)
	}
	return nil
}
