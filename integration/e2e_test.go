//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/scheduler"
)

const (
	e2ePollInterval = 5 * time.Second
	e2eTimeout      = 20 * time.Minute
)

// TestE2E runs a full end-to-end verification against an already-running
// server/worker environment using only the HTTP API.
//
// Required env vars:
//   - E2E_SERVER_URL (example: http://server:8080)
//   - E2E_TOKEN
//   - E2E_SOURCE_PATH (relative path baked into the server container)
func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	serverURL := strings.TrimRight(os.Getenv("E2E_SERVER_URL"), "/")
	token := os.Getenv("E2E_TOKEN")
	sourcePath := os.Getenv("E2E_SOURCE_PATH")
	if serverURL == "" || token == "" || sourcePath == "" {
		t.Skip("E2E env vars not set")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	var jobID string
	var completedJob *model.Job

	t.Run("SubmitJob", func(t *testing.T) {
		jobReq := &model.JobRequest{SourcePath: sourcePath}
		body, err := json.Marshal(jobReq)
		if err != nil {
			t.Fatalf("Failed to marshal job request: %v", err)
		}

		respBody, statusCode := doJSONRequest(t, client, http.MethodPost, serverURL+"/api/v1/job/", token, bytes.NewReader(body))
		if statusCode != http.StatusOK {
			t.Fatalf("Job submission failed with status %d: %s", statusCode, string(respBody))
		}

		var result scheduler.ScheduleJobRequestResult
		if err := json.Unmarshal(respBody, &result); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if len(result.ScheduledJobs) == 0 {
			t.Fatalf("No jobs scheduled. Failed: %v, Skipped: %v", result.FailedJobRequest, result.SkippedFiles)
		}

		jobID = result.ScheduledJobs[0].Id.String()
		if jobID == "" {
			t.Fatal("Scheduled job ID is empty")
		}

		t.Logf("Job submitted: %s", jobID)
	})

	t.Run("WaitForCompletion", func(t *testing.T) {
		deadline := time.Now().Add(e2eTimeout)
		for time.Now().Before(deadline) {
			job := getJob(t, client, serverURL, token, jobID)
			if len(job.Events) == 0 {
				t.Fatal("Job has no events")
			}

			status := job.Events.GetStatus()
			latestEvent := job.Events.GetLatest()
			if latestEvent != nil {
				t.Logf("  Status: %s/%s — %s", latestEvent.NotificationType, latestEvent.Status, latestEvent.Message)
			}

			switch status {
			case model.CompletedNotificationStatus:
				completedJob = job
				return
			case model.FailedNotificationStatus:
				if latestEvent == nil {
					t.Fatal("Job failed without latest event details")
				}
				dumpEvents(t, job.Events)
				t.Fatalf("Job failed: %s", latestEvent.Message)
			}

			time.Sleep(e2ePollInterval)
		}

		if completedJob == nil {
			job := getJob(t, client, serverURL, token, jobID)
			dumpEvents(t, job.Events)
			t.Fatal("Job did not complete within timeout")
		}
	})

	t.Run("VerifyOutput", func(t *testing.T) {
		if completedJob == nil {
			t.Fatal("Completed job not available")
		}
		if completedJob.SourceProbe == nil {
			t.Fatal("Job source_probe is nil")
		}
		if completedJob.TargetProbe == nil {
			t.Fatal("Job target_probe is nil")
		}

		sourceProbe := completedJob.SourceProbe
		targetProbe := completedJob.TargetProbe

		var hasHEVC, hasAAC, hasSRT bool
		audioLangs := make(map[string]int)
		subtitleLangs := make(map[string]int)
		var outputVideoWidth, outputVideoHeight int
		var outputAudioTitles, outputSubTitles []string

		for i, s := range targetProbe.Streams {
			t.Logf("  %d. %s: %s (lang=%s title=%q)", i, s.CodecType, s.CodecName, s.Language, s.Title)
			switch {
			case s.CodecType == "video" && s.CodecName == "hevc":
				hasHEVC = true
				outputVideoWidth = s.Width
				outputVideoHeight = s.Height
			case s.CodecType == "audio" && s.CodecName == "aac":
				hasAAC = true
				if s.Language != "" {
					audioLangs[s.Language]++
				}
				if s.Title != "" {
					outputAudioTitles = append(outputAudioTitles, s.Title)
				}
			case s.CodecType == "subtitle" && s.CodecName == "subrip":
				hasSRT = true
				if s.Language != "" {
					subtitleLangs[s.Language]++
				}
				if s.Title != "" {
					outputSubTitles = append(outputSubTitles, s.Title)
				}
			}
		}

		if !hasHEVC {
			t.Errorf("Output missing HEVC video stream")
		}
		if !hasAAC {
			t.Errorf("Output missing AAC audio stream")
		}
		if !hasSRT {
			t.Errorf("Output missing SRT subtitle stream (PGS should have been converted to SRT)")
		}

		sourceDuration := sourceProbe.DurationSeconds
		outputDuration := targetProbe.DurationSeconds
		t.Logf("Duration: source=%.2fs output=%.2fs", sourceDuration, outputDuration)
		if sourceDuration > 0 && outputDuration > 0 {
			ratio := outputDuration / sourceDuration
			if ratio < 0.8 || ratio > 1.2 {
				t.Errorf("Duration mismatch: output %.2fs vs source %.2fs (ratio %.2f) — video may be truncated or corrupted", outputDuration, sourceDuration, ratio)
			}
		}

		var sourceVideoWidth, sourceVideoHeight int
		for _, s := range sourceProbe.Streams {
			if s.CodecType == "video" {
				sourceVideoWidth = s.Width
				sourceVideoHeight = s.Height
				break
			}
		}
		t.Logf("Resolution: source=%dx%d output=%dx%d", sourceVideoWidth, sourceVideoHeight, outputVideoWidth, outputVideoHeight)
		if sourceVideoWidth > 0 && outputVideoWidth != sourceVideoWidth {
			t.Errorf("Video width changed: source=%d output=%d", sourceVideoWidth, outputVideoWidth)
		}
		if sourceVideoHeight > 0 && outputVideoHeight != sourceVideoHeight {
			t.Errorf("Video height changed: source=%d output=%d", sourceVideoHeight, outputVideoHeight)
		}

		t.Logf("Audio languages found: %v", audioLangs)
		if audioLangs["eng"] == 0 {
			t.Errorf("Output missing English audio track")
		}
		if audioLangs["spa"] == 0 {
			t.Errorf("Output missing Spanish audio track")
		}

		t.Logf("Subtitle languages found: %v", subtitleLangs)
		if subtitleLangs["eng"] == 0 {
			t.Errorf("Output missing English subtitle track")
		}
		if subtitleLangs["spa"] == 0 {
			t.Errorf("Output missing Spanish subtitle track")
		}

		sourceAudioLangs := countStreamLanguages(sourceProbe.Streams, "audio")
		if sourceAudioLangs > 0 && len(audioLangs) < sourceAudioLangs {
			t.Errorf("Audio language count decreased: output has %d language(s) but source had %d", len(audioLangs), sourceAudioLangs)
		}

		sourceSubLangs := countStreamLanguages(sourceProbe.Streams, "subtitle")
		if sourceSubLangs > 0 && len(subtitleLangs) < sourceSubLangs {
			t.Errorf("Subtitle language count decreased: output has %d language(s) but source had %d", len(subtitleLangs), sourceSubLangs)
		}

		t.Logf("Audio titles: %v", outputAudioTitles)
		t.Logf("Subtitle titles: %v", outputSubTitles)
		if len(outputAudioTitles) == 0 {
			t.Errorf("No audio tracks have title metadata — titles were lost during transcoding")
		}
		if len(outputSubTitles) == 0 {
			t.Errorf("No subtitle tracks have title metadata — titles were lost during transcoding")
		}
	})

	t.Run("VerifyEventHistory", func(t *testing.T) {
		if completedJob == nil {
			t.Fatal("Completed job not available")
		}

		events := completedJob.Events
		dumpEvents(t, events)

		if len(events) < 5 {
			t.Errorf("Expected at least 5 events, got %d", len(events))
		}

		firstEvent := events[0]
		if firstEvent.Status != model.QueuedNotificationStatus {
			t.Errorf("First event should be Queued, got %s", firstEvent.Status)
		}

		lastEvent := events[len(events)-1]
		if lastEvent.Status != model.CompletedNotificationStatus {
			t.Errorf("Last event should be Completed, got %s", lastEvent.Status)
		}
	})
}

func doJSONRequest(t *testing.T, client *http.Client, method, requestURL, token string, body io.Reader) ([]byte, int) {
	t.Helper()

	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return respBody, resp.StatusCode
}

func getJob(t *testing.T, client *http.Client, serverURL, token, jobID string) *model.Job {
	t.Helper()

	respBody, statusCode := doJSONRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/v1/job/%s", serverURL, jobID), token, nil)
	if statusCode != http.StatusOK {
		t.Fatalf("Get job failed with status %d: %s", statusCode, string(respBody))
	}

	var job model.Job
	if err := json.Unmarshal(respBody, &job); err != nil {
		t.Fatalf("Failed to parse job response: %v", err)
	}

	return &job
}

func countStreamLanguages(streams []model.MediaStream, codecType string) int {
	langs := make(map[string]bool)
	for _, s := range streams {
		if s.CodecType == codecType && s.Language != "" {
			langs[s.Language] = true
		}
	}
	return len(langs)
}

func dumpEvents(t *testing.T, events model.TaskEvents) {
	t.Helper()
	t.Logf("Event history (%d events):", len(events))
	for i, ev := range events {
		t.Logf("  %d. [%s] %s: %s", i+1, ev.NotificationType, ev.Status, ev.Message)
	}
}
