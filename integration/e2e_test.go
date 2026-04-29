//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"transcoder/model"
	"transcoder/server/repository"
	"transcoder/server/scheduler"
)

type probeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Tags      struct {
		Language string `json:"language"`
		Title    string `json:"title"`
	} `json:"tags"`
}

type probeFormat struct {
	Duration string `json:"duration"`
}

func (f probeFormat) DurationSeconds() float64 {
	v, _ := strconv.ParseFloat(f.Duration, 64)
	return v
}

type probeData struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

const (
	e2eDBName   = "transcoderd_e2e"
	e2eDBUser   = "test"
	e2eDBPass   = "test"
	e2eToken    = "e2e-test-token"
	e2eTestFile = "testdata/e2e_fixture.mkv"

	defaultServerImage = "transcoderd:server-test"
	defaultWorkerImage = "transcoderd:worker-test"
)

// TestDockerE2E runs a full end-to-end test using real Docker containers
// for both server and worker, with a synthetic MKV containing dual-language
// audio (eng+spa) and PGS bitmap subtitles (2 eng + 2 spa).
//
// Prerequisites:
//   - Docker running
//   - Pre-built Docker images (or set E2E_SERVER_IMAGE / E2E_WORKER_IMAGE env vars):
//     make buildcontainer-server buildcontainer-worker
//   - Test fixture present: integration/testdata/e2e_fixture.mkv (507KB, committed to git)
//
// Run with: go test -tags=integration -v -timeout 30m -run TestDockerE2E ./integration/...
func TestDockerE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker e2e test in short mode")
	}

	serverImage := envOrDefault("E2E_SERVER_IMAGE", defaultServerImage)
	workerImage := envOrDefault("E2E_WORKER_IMAGE", defaultWorkerImage)
	t.Logf("Using images: server=%s worker=%s", serverImage, workerImage)

	fixtureAbs, err := filepath.Abs(e2eTestFile)
	if err != nil {
		t.Fatalf("Failed to resolve fixture path: %v", err)
	}
	if _, err := os.Stat(fixtureAbs); os.IsNotExist(err) {
		t.Fatalf("Test fixture not found at %s — it must be committed to git", fixtureAbs)
	}

	ctx := context.Background()

	net, err := network.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create Docker network: %v", err)
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Failed to remove network: %v", err)
		}
	}()

	t.Log("Starting PostgreSQL container...")
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(e2eDBName),
		postgres.WithUsername(e2eDBUser),
		postgres.WithPassword(e2eDBPass),
		network.WithNetwork([]string{"postgres"}, net),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Failed to start postgres: %v", err)
	}
	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate postgres: %v", err)
		}
	}()

	pgPort := mustMappedPortInt(t, ctx, pgContainer, "5432")
	t.Logf("PostgreSQL running on port %d", pgPort)

	dbConfig := &repository.SQLServerConfig{
		Host:     "localhost",
		Port:     pgPort,
		User:     e2eDBUser,
		Password: e2eDBPass,
		Scheme:   e2eDBName,
		Driver:   "postgres",
	}
	repo, err := repository.NewSQLRepository(dbConfig)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	if err := repo.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize DB schema: %v", err)
	}
	t.Log("DB schema initialized")

	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	testVideoRelPath := "test-video.mkv"
	destPath := filepath.Join(sourceDir, testVideoRelPath)
	if err := copyFile(fixtureAbs, destPath); err != nil {
		t.Fatalf("Failed to copy test fixture: %v", err)
	}
	fi, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Failed to stat copied fixture: %v", err)
	}
	t.Logf("Test video: %s (%d bytes)", destPath, fi.Size())

	configContent := fmt.Sprintf(`server:
  database:
    host: postgres
    port: 5432
    user: %s
    password: %s
    scheme: %s
    driver: postgres
  scheduler:
    sourcePath: /source
    deleteOnComplete: false
    minFileSize: 100
    scheduleTime: 5s
    jobTimeout: 10m
web:
  token: %s
  port: 8080
`, e2eDBUser, e2eDBPass, e2eDBName, e2eToken)

	configFile := filepath.Join(sourceDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	t.Log("Starting server container...")
	serverContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        serverImage,
			ExposedPorts: []string{"8080/tcp"},
			Cmd:          []string{"--noUpdates"},
			Mounts: testcontainers.ContainerMounts{
				testcontainers.BindMount(configFile, "/etc/transcoderd/config.yml"),
				testcontainers.BindMount(sourceDir, "/source"),
				testcontainers.BindMount(targetDir, "/target"),
			},
			Networks: []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {"server"},
			},
			WaitingFor: wait.ForHTTP("/api/v1/job/request").
				WithPort("8080/tcp").
				WithHeaders(map[string]string{
					"Authorization": "Bearer " + e2eToken,
					"workerName":    "healthcheck",
				}).
				WithStatusCodeMatcher(func(status int) bool {
					return status == 204 || status == 200
				}).
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start server container: %v", err)
	}
	defer func() {
		if err := serverContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate server: %v", err)
		}
	}()

	serverPortNum := mustMappedPortInt(t, ctx, serverContainer, "8080")
	serverURL := fmt.Sprintf("http://localhost:%d", serverPortNum)
	t.Logf("Server running at %s", serverURL)

	t.Log("Starting worker container...")
	workerContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: workerImage,
			Cmd: []string{
				"--noUpdates",
				"--web.token", e2eToken,
				"--web.domain", "http://server:8080",
				"--worker.name", "e2e-worker",
				"--worker.ffmpegConfig.videoPreset", "ultrafast",
				"--worker.ffmpegConfig.videoCRF", "35",
				"--worker.ffmpegConfig.videoCodec", "libx265",
				"--worker.ffmpegConfig.audioCodec", "aac",
				"--worker.ffmpegConfig.videoProfile", "main10",
				"--worker.verifyDeltaTime", "5",
				"--worker.threads", "2",
			},
			Networks: []string{net.Name},
			WaitingFor: wait.ForLog("Starting Worker").
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("Failed to start worker container: %v", err)
	}
	defer func() {
		if err := workerContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate worker: %v", err)
		}
	}()
	t.Log("Worker container started")

	t.Run("SubmitJob", func(t *testing.T) {
		jobReq := &model.JobRequest{
			SourcePath: testVideoRelPath,
		}
		body, err := json.Marshal(jobReq)
		if err != nil {
			t.Fatalf("Failed to marshal job request: %v", err)
		}

		req, err := http.NewRequest("POST", serverURL+"/api/v1/job/", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+e2eToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to submit job: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("Job submission failed with status %d: %s", resp.StatusCode, respBody)
		}

		var result scheduler.ScheduleJobRequestResult
		if err := json.Unmarshal(respBody, &result); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if len(result.ScheduledJobs) == 0 {
			t.Fatalf("No jobs scheduled. Failed: %v, Skipped: %v", result.FailedJobRequest, result.SkippedFiles)
		}
		t.Logf("Job submitted: %s", result.ScheduledJobs[0].Id)
	})

	jobs, err := repo.GetJobs(ctx)
	if err != nil || jobs == nil || len(*jobs) == 0 {
		t.Fatalf("Failed to find submitted job: %v", err)
	}
	jobID := (*jobs)[0].Id.String()

	t.Run("WaitForCompletion", func(t *testing.T) {
		deadline := time.Now().Add(20 * time.Minute)
		for time.Now().Before(deadline) {
			job, err := repo.GetJob(ctx, jobID)
			if err != nil {
				t.Fatalf("Failed to get job: %v", err)
			}

			status := job.Events.GetStatus()

			if status == model.CompletedNotificationStatus {
				t.Logf("Job completed successfully!")
				t.Logf("  Source: %s (%d bytes)", job.SourcePath, job.SourceSize)
				t.Logf("  Target: %s (%d bytes)", job.TargetPath, job.TargetSize)
				if job.SourceSize > 0 {
					ratio := float64(job.TargetSize) / float64(job.SourceSize) * 100
					t.Logf("  Size ratio: %.1f%%", ratio)
				}
				return
			}

			if status == model.FailedNotificationStatus {
				lastEvent := job.Events.GetLatest()
				dumpContainerLogs(t, ctx, workerContainer, "Worker")
				t.Fatalf("Job failed: %s", lastEvent.Message)
			}

			latestEvent := job.Events.GetLatest()
			t.Logf("  Status: %s/%s — %s", latestEvent.NotificationType, latestEvent.Status, latestEvent.Message)
			time.Sleep(5 * time.Second)
		}
		dumpContainerLogs(t, ctx, workerContainer, "Worker")
		dumpContainerLogs(t, ctx, serverContainer, "Server")
		t.Fatal("Job did not complete within timeout")
	})

	t.Run("VerifyOutput", func(t *testing.T) {
		job, err := repo.GetJob(ctx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		targetPath := filepath.Join(sourceDir, job.TargetPath)
		tfi, err := os.Stat(targetPath)
		if os.IsNotExist(err) {
			targetPath = filepath.Join(targetDir, job.TargetPath)
			tfi, err = os.Stat(targetPath)
		}
		if err != nil {
			t.Fatalf("Output file not found: %v (checked both source and target dirs)", err)
		}
		if tfi.Size() == 0 {
			t.Fatal("Output file is empty")
		}
		t.Logf("Output file: %s (%d bytes)", targetPath, tfi.Size())

		if dest := os.Getenv("E2E_KEEP_OUTPUT"); dest != "" {
			_ = copyFile(targetPath, dest)
		}

		if job.TargetSize > 0 && job.SourceSize > 0 && job.TargetSize >= job.SourceSize {
			t.Logf("Warning: target (%d) is not smaller than source (%d)", job.TargetSize, job.SourceSize)
		}

		containerOutputPath := "/source/" + job.TargetPath
		containerSourcePath := "/source/" + testVideoRelPath
		probeOutputFile := "/source/ffprobe_result.json"
		probeSourceFile := "/source/ffprobe_source.json"

		exitCode, execOutput, err := serverContainer.Exec(ctx, []string{
			"sh", "-c",
			fmt.Sprintf("ffprobe -v quiet -print_format json -show_streams -show_format '%s' > '%s' 2>&1", containerOutputPath, probeOutputFile),
		})
		if err != nil {
			t.Fatalf("Failed to exec ffprobe in server container: %v", err)
		}
		if exitCode != 0 {
			execBytes, _ := io.ReadAll(execOutput)
			t.Fatalf("ffprobe failed (exit %d): %s", exitCode, string(execBytes))
		}

		probeBytes, err := os.ReadFile(filepath.Join(sourceDir, "ffprobe_result.json"))
		if err != nil {
			t.Fatalf("Failed to read ffprobe output file: %v", err)
		}

		var probeResult probeData
		if err := json.Unmarshal(probeBytes, &probeResult); err != nil {
			t.Fatalf("Failed to parse ffprobe output: %v\nRaw: %s", err, string(probeBytes))
		}

		exitCode, execOutput, err = serverContainer.Exec(ctx, []string{
			"sh", "-c",
			fmt.Sprintf("ffprobe -v quiet -print_format json -show_streams -show_format '%s' > '%s' 2>&1", containerSourcePath, probeSourceFile),
		})
		if err != nil {
			t.Fatalf("Failed to exec ffprobe on source: %v", err)
		}
		if exitCode != 0 {
			execBytes, _ := io.ReadAll(execOutput)
			t.Fatalf("ffprobe on source failed (exit %d): %s", exitCode, string(execBytes))
		}

		sourceProbeBytes, err := os.ReadFile(filepath.Join(sourceDir, "ffprobe_source.json"))
		if err != nil {
			t.Fatalf("Failed to read source ffprobe file: %v", err)
		}
		var sourceProbeResult probeData
		if err := json.Unmarshal(sourceProbeBytes, &sourceProbeResult); err != nil {
			t.Fatalf("Failed to parse source ffprobe: %v", err)
		}

		t.Logf("Output streams (%d):", len(probeResult.Streams))
		var hasHEVC, hasAAC, hasSRT bool
		audioLangs := make(map[string]int)
		subtitleLangs := make(map[string]int)
		var outputVideoWidth, outputVideoHeight int
		var outputAudioTitles, outputSubTitles []string
		for i, s := range probeResult.Streams {
			t.Logf("  %d. %s: %s (lang=%s title=%q)", i, s.CodecType, s.CodecName, s.Tags.Language, s.Tags.Title)
			switch {
			case s.CodecType == "video" && s.CodecName == "hevc":
				hasHEVC = true
				outputVideoWidth = s.Width
				outputVideoHeight = s.Height
			case s.CodecType == "audio" && s.CodecName == "aac":
				hasAAC = true
				if lang := s.Tags.Language; lang != "" {
					audioLangs[lang]++
				}
				if s.Tags.Title != "" {
					outputAudioTitles = append(outputAudioTitles, s.Tags.Title)
				}
			case s.CodecType == "subtitle" && s.CodecName == "subrip":
				hasSRT = true
				if lang := s.Tags.Language; lang != "" {
					subtitleLangs[lang]++
				}
				if s.Tags.Title != "" {
					outputSubTitles = append(outputSubTitles, s.Tags.Title)
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

		// Duration check: output duration must be within 20% of source
		sourceDuration := sourceProbeResult.Format.DurationSeconds()
		outputDuration := probeResult.Format.DurationSeconds()
		t.Logf("Duration: source=%.2fs output=%.2fs", sourceDuration, outputDuration)
		if sourceDuration > 0 && outputDuration > 0 {
			ratio := outputDuration / sourceDuration
			if ratio < 0.8 || ratio > 1.2 {
				t.Errorf("Duration mismatch: output %.2fs vs source %.2fs (ratio %.2f) — video may be truncated or corrupted", outputDuration, sourceDuration, ratio)
			}
		}

		// Video resolution check: must match source
		var sourceVideoWidth, sourceVideoHeight int
		for _, s := range sourceProbeResult.Streams {
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

		// Per-language assertion: both eng and spa must exist in audio
		t.Logf("Audio languages found: %v", audioLangs)
		if audioLangs["eng"] == 0 {
			t.Errorf("Output missing English audio track")
		}
		if audioLangs["spa"] == 0 {
			t.Errorf("Output missing Spanish audio track")
		}

		// Per-language assertion: both eng and spa must exist in subtitles
		t.Logf("Subtitle languages found: %v", subtitleLangs)
		if subtitleLangs["eng"] == 0 {
			t.Errorf("Output missing English subtitle track")
		}
		if subtitleLangs["spa"] == 0 {
			t.Errorf("Output missing Spanish subtitle track")
		}

		// Language count vs source
		sourceAudioLangs := countStreamLanguages(sourceProbeResult.Streams, "audio")
		if sourceAudioLangs > 0 && len(audioLangs) < sourceAudioLangs {
			t.Errorf("Audio language count decreased: output has %d language(s) but source had %d", len(audioLangs), sourceAudioLangs)
		}
		sourceSubLangs := countStreamLanguages(sourceProbeResult.Streams, "subtitle")
		if sourceSubLangs > 0 && len(subtitleLangs) < sourceSubLangs {
			t.Errorf("Subtitle language count decreased: output has %d language(s) but source had %d", len(subtitleLangs), sourceSubLangs)
		}

		// Metadata title preservation: audio and subtitle tracks must retain titles
		t.Logf("Audio titles: %v", outputAudioTitles)
		t.Logf("Subtitle titles: %v", outputSubTitles)
		if len(outputAudioTitles) == 0 {
			t.Errorf("No audio tracks have title metadata — titles were lost during transcoding")
		}
		if len(outputSubTitles) == 0 {
			t.Errorf("No subtitle tracks have title metadata — titles were lost during transcoding")
		}

		// File playability: decode the entire output to detect corrupt frames
		exitCode, execOutput, err = serverContainer.Exec(ctx, []string{
			"sh", "-c",
			fmt.Sprintf("ffmpeg -v error -i '%s' -f null - 2>&1", containerOutputPath),
		})
		if err != nil {
			t.Fatalf("Failed to run playability check: %v", err)
		}
		if exitCode != 0 {
			execBytes, _ := io.ReadAll(execOutput)
			t.Errorf("File playability check failed (exit %d) — output has corrupt frames or demux errors:\n%s", exitCode, string(execBytes))
		} else {
			t.Log("File playability check passed (full decode, no errors)")
		}
	})

	t.Run("VerifyEventHistory", func(t *testing.T) {
		job, err := repo.GetJob(ctx, jobID)
		if err != nil {
			t.Fatalf("Failed to get job: %v", err)
		}

		events := job.Events

		t.Logf("Event history (%d events):", len(events))
		for i, ev := range events {
			t.Logf("  %d. [%s] %s: %s", i+1, ev.NotificationType, ev.Status, ev.Message)
		}

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

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func mustMappedPortInt(t *testing.T, ctx context.Context, c testcontainers.Container, port string) int {
	t.Helper()
	p, err := c.MappedPort(ctx, port)
	if err != nil {
		t.Fatalf("Failed to get mapped port %s: %v", port, err)
	}
	portNum, err := strconv.Atoi(p.Port())
	if err != nil {
		t.Fatalf("Failed to parse port %q: %v", p.Port(), err)
	}
	return portNum
}

func countStreamLanguages(streams []probeStream, codecType string) int {
	langs := make(map[string]bool)
	for _, s := range streams {
		if s.CodecType == codecType && s.Tags.Language != "" {
			langs[s.Tags.Language] = true
		}
	}
	return len(langs)
}

func dumpContainerLogs(t *testing.T, ctx context.Context, c testcontainers.Container, name string) {
	t.Helper()
	logs, err := c.Logs(ctx)
	if err != nil {
		t.Logf("Failed to get %s logs: %v", name, err)
		return
	}
	defer logs.Close()
	logBytes, _ := io.ReadAll(logs)
	t.Logf("%s logs:\n%s", name, string(logBytes))
}
