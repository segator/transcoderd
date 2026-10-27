package worker

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"
	"transcoder/worker/step"

	"github.com/google/uuid"
)

func TestLocalProgressSnapshotLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	render := console.NewRenderService()
	executor := &JobExecutor{console: render, client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "test"), workerConfig: &config.Config{EncodeConfig: &config.FFMPEGConfig{VideoCodec: config.AV1NVENC}}}
	id := uuid.New()
	tracker := executor.ReportTrackStep(&job.Context{JobId: id, Source: &job.VideoData{OriginalName: "film.mkv"}}, "video", model.FFMPEGSNotification)
	tracker.SetTotal(100)
	tracker.SetDisplayName("film renamed.mkv")
	tracker.UpdateValue(45)
	got := executor.progress.snapshot()
	if len(got) != 1 || got[0].JobID != id.String() || got[0].Name != "film renamed.mkv" || got[0].StepID != "video" || got[0].Percent != 45 || got[0].Codec != config.AV1NVENC || got[0].Profile != "gpu" {
		t.Fatalf("snapshot = %+v, want video at 45%%", got)
	}
	tracker.Done()
	tracker.updateSnapshot() // A late FFmpeg progress event must not resurrect it.
	if got := executor.progress.snapshot(); len(got) != 0 {
		t.Fatalf("snapshot after completion = %+v, want empty", got)
	}
}

func TestPipelineSnapshotDistinguishesPrefetchedJobs(t *testing.T) {
	id := uuid.New()
	ctx := &job.Context{JobId: id, Source: &job.VideoData{OriginalName: "film.mkv", FilePath: "downloaded.mkv"}}
	e := &JobExecutor{}
	e.jobs.codec = &config.FFMPEGConfig{VideoCodec: config.AV1SVT}
	ex := step.NewFFMPEGStepExecutor(&config.FFMPEGConfig{})
	e.trackPipeline(map[model.NotificationType]*step.Executor{model.FFMPEGSNotification: ex})
	ex.AddJob(ctx)
	got := e.jobs.snapshot()
	if len(got) != 1 || got[0].State != "queued" || !got[0].Downloaded || got[0].Name != "film.mkv" || got[0].Codec != config.AV1SVT || got[0].Profile != "cpu" {
		t.Fatalf("queued snapshot = %+v", got)
	}
	ex.OnJob(ctx)
	if got = e.jobs.snapshot(); len(got) != 1 || got[0].State != "running" {
		t.Fatalf("running snapshot = %+v", got)
	}
	e.jobs.remove(id.String())
	if got = e.jobs.snapshot(); len(got) != 0 {
		t.Fatalf("finished snapshot = %+v", got)
	}
}

func TestProgressUpdatesCanRaceWithCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	executor := &JobExecutor{console: console.NewRenderService(), client: serverclient.NewServerClient(&web.Config{Domain: server.URL}, "test")}
	tracker := executor.ReportTrackStep(&job.Context{JobId: uuid.New()}, "video", model.FFMPEGSNotification)
	tracker.SetTotal(100)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			tracker.UpdateValue(int64(i))
		}
	}()
	tracker.Done()
	wg.Wait()
	if got := executor.progress.snapshot(); len(got) != 0 {
		t.Fatalf("completed progress was resurrected: %+v", got)
	}
}
