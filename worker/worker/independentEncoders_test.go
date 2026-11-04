package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"transcoder/model"
	"transcoder/server/web"
	"transcoder/worker/config"
	"transcoder/worker/job"
	"transcoder/worker/serverclient"

	"github.com/google/uuid"
)

func TestIndependentEncoderQueueRunsTwoCPUAndThreeGPU(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pool := newEncodeConcurrency(2)
	pool.setLimits(2, 3)
	queue := make(chan *job.Context, 7)
	type running struct {
		id   uuid.UUID
		kind string
	}
	started := make(chan running, 7)
	releases := make(map[uuid.UUID]chan struct{})
	for i := 0; i < 7; i++ {
		j := &job.Context{JobId: uuid.New()}
		releases[j.JobId] = make(chan struct{}, 1)
		queue <- j
	}
	go runEncoderQueue(ctx, queue, pool, func(j *job.Context, kind string) {
		started <- running{j.JobId, kind}
		select {
		case <-ctx.Done():
		case <-releases[j.JobId]:
		}
	})
	var cpu, gpu []running
	for i := 0; i < 5; i++ {
		select {
		case got := <-started:
			if got.kind == config.CPUEncoderKind {
				cpu = append(cpu, got)
			} else {
				gpu = append(gpu, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("independent queue did not fill five slots")
		}
	}
	if len(cpu) != 2 || len(gpu) != 3 {
		t.Fatalf("simultaneous encodes = CPU %d GPU %d, want 2+3", len(cpu), len(gpu))
	}
	select {
	case extra := <-started:
		t.Fatalf("extra encode started without a slot: %+v", extra)
	case <-time.After(20 * time.Millisecond):
	}
	// CPU drain must not assign the waiting jobs to the newly disabled class.
	pool.setLimits(0, 3)
	releases[cpu[0].id] <- struct{}{}
	select {
	case extra := <-started:
		t.Fatalf("disabled CPU accepted unassigned work: %+v", extra)
	case <-time.After(20 * time.Millisecond):
	}
	releases[gpu[0].id] <- struct{}{}
	select {
	case next := <-started:
		if next.kind != config.GPUEncoderKind {
			t.Errorf("next kind = %s, want GPU", next.kind)
		}
	case <-time.After(time.Second):
		t.Fatal("GPU did not fill a released GPU slot")
	}
	// Both zero stops admission in JobExecutor, but already accepted queued work
	// must still be able to drain using the prior positive per-class capacity.
	pool.setLimits(0, 0)
	select {
	case next := <-started:
		if next.kind != config.CPUEncoderKind {
			t.Errorf("accepted drain kind = %s, want free CPU slot", next.kind)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted job was stranded by a global pause")
	}
}

func TestIndependentEncoderLimitsResizeAndRestore(t *testing.T) {
	pool := newEncodeConcurrency(2)
	pool.setLimits(2, 3)
	for i := 0; i < 2; i++ {
		if _, ok := pool.tryStartEncoder("cpu"); !ok {
			t.Fatal("CPU slot unavailable")
		}
	}
	pool.setLimits(1, 3)
	if _, ok := pool.tryStartEncoder("cpu"); ok {
		t.Error("lowered CPU target admitted an extra encode")
	}
	pool.finishEncoder("cpu")
	if _, ok := pool.tryStartEncoder("cpu"); ok {
		t.Error("CPU target admitted while at its limit")
	}
	pool.finishEncoder("cpu")
	if _, ok := pool.tryStartEncoder("cpu"); !ok {
		t.Error("CPU slot did not reopen after drain")
	}
	pool.finishEncoder("cpu")
	pool.setLimits(0, 3)
	if kind, ok := pool.tryStartEncoder(""); !ok || kind != "gpu" {
		t.Errorf("unassigned kind = %s/%t, want GPU", kind, ok)
	}
	if _, ok := pool.tryStartEncoder("cpu"); !ok {
		t.Error("pinned CPU work could not resume while CPU admission was disabled")
	}
	pool.finishEncoder("cpu")
	// A restored class absent from the startup profile gets one drain slot.
	restored := newEncodeConcurrency(2)
	restored.setLimits(0, 0)
	if _, ok := restored.tryStartEncoder("cpu"); !ok {
		t.Error("restored CPU job was stranded")
	}
	if _, ok := restored.tryStartEncoder("cpu"); ok {
		t.Error("restored CPU drain exceeded its fallback limit")
	}
}

func TestIndependentAdmissionAndProfilePersistence(t *testing.T) {
	e := testAdmissionWorker(2)
	e.workerConfig.EncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1NVENC, VideoCQ: 39, ExtraArgs: "-hwaccel cuda", AudioCodec: "aac", AudioVBR: 5}
	if err := e.SetEncoderLimits(2, 3); err != nil {
		t.Fatal(err)
	}
	s := e.AdmissionStatus()
	if s.MaxActiveJobs != 5 || s.MaxAcceptedJobs != 6 || s.CPUJobs != 2 || s.GPUJobs != 3 {
		t.Fatalf("dual status = %+v, want 5 slots plus 1 pipeline job", s)
	}
	e.activeJobs = 5
	if !e.ReserveJobSlot() {
		t.Fatal("sixth pipeline slot unavailable")
	}
	if e.ReserveJobSlot() {
		t.Fatal("admission exceeded CPU+GPU+1")
	}
	e.ReleaseJobSlot()
	if err := e.SetEncoderLimits(0, 0); err != nil {
		t.Fatal(err)
	}
	if e.ReserveJobSlot() || e.AdmissionStatus().ActiveJobs != 5 {
		t.Error("global pause did not preserve accepted work while closing admission")
	}
	cpu := job.NewContext(uuid.New(), 0, filepath.Join(t.TempDir(), "cpu"))
	if err := cpu.Init(); err != nil {
		t.Fatal(err)
	}
	if err := e.pinEncoderProfile(cpu, "cpu"); err != nil {
		t.Fatal(err)
	}
	if cpu.EncodeConfig.VideoCodec != config.AV1SVT || cpu.EncodeConfig.VideoCQ != 0 || cpu.EncodeConfig.ExtraArgs != "" {
		t.Fatalf("CPU profile inherited GPU options: %+v", cpu.EncodeConfig)
	}
	if err := cpu.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cpu.WorkingDir, cpu.JobId.String()+".json")
	loaded, err := job.ReadContextFromDiskByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	e.workerConfig.CPUEncodeConfig = &config.FFMPEGConfig{VideoCodec: config.HEVCX265}
	if err := e.pinEncoderProfile(loaded, "cpu"); err != nil {
		t.Fatal(err)
	}
	if loaded.EncodeKind != "cpu" || loaded.EncodeConfig.VideoCodec != config.AV1SVT {
		t.Errorf("restored profile changed to %+v", loaded.EncodeConfig)
	}
	gpu := &job.Context{}
	if err := e.pinEncoderProfile(gpu, "gpu"); err != nil {
		t.Fatal(err)
	}
	e.workerConfig.EncodeConfig.VideoCQ = 40
	if gpu.EncodeConfig.VideoCQ != 39 {
		t.Error("in-flight GPU profile shared mutable startup settings")
	}
}

func TestCPUOnlyAdmissionSurvivesGlobalPauseAndRestart(t *testing.T) {
	e := testAdmissionWorker(2)
	e.workerConfig.EncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1NVENC}
	if err := e.SetEncoderLimits(2, 0); err != nil {
		t.Fatal(err)
	}
	// This also covers an in-flight assignment arriving after a global pause.
	if err := e.SetEncoderLimits(0, 0); err != nil {
		t.Fatal(err)
	}
	j := job.NewContext(uuid.New(), 0, t.TempDir())
	if err := e.pinAdmissionProfile(j); err != nil {
		t.Fatal(err)
	}
	if j.EncodeKind != "cpu" || j.EncodeConfig.VideoCodec != config.AV1SVT {
		t.Fatalf("CPU-only accepted profile = %s %+v", j.EncodeKind, j.EncodeConfig)
	}
	if err := j.PersistJobContext(); err != nil {
		t.Fatal(err)
	}
	loaded, err := job.ReadContextFromDiskByPath(filepath.Join(j.WorkingDir, j.JobId.String()+".json"))
	if err != nil {
		t.Fatal(err)
	}
	newPool := newEncodeConcurrency(2)
	newPool.setLimits(0, 0)
	kind, ok := newPool.tryStartEncoder(loaded.EncodeKind)
	if !ok || kind != "cpu" {
		t.Fatalf("restored accepted CPU job selected %s/%t", kind, ok)
	}
}

func TestSynchronizeIndependentLimitsAndTelemetry(t *testing.T) {
	cpu, gpu := 2, 3
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("workerCapabilities") != model.IndependentEncoderCapability {
			t.Error("independent capability not declared")
		}
		_ = json.NewEncoder(w).Encode(model.WorkerSettings{CPUJobs: &cpu, GPUJobs: &gpu})
	}))
	defer server.Close()
	e := testAdmissionWorker(2)
	e.workerConfig.EncodeConfig = &config.FFMPEGConfig{VideoCodec: config.AV1NVENC}
	c := NewServerCoordinator(serverclient.NewServerClient(&web.Config{Domain: server.URL}, "mixed"), e, nil, testConsoleLogger{})
	for _, pair := range [][2]int{{2, 3}, {0, 3}, {0, 0}, {4, 1}} {
		cpu, gpu = pair[0], pair[1]
		if err := c.syncWorkerSettings(t.Context()); err != nil {
			t.Fatal(err)
		}
		rt := e.RuntimeTelemetry()
		if rt.CapacityVersion != 2 || rt.CPUJobs != cpu || rt.GPUJobs != gpu || rt.CPUEncoder != config.AV1SVT || rt.GPUEncoder != config.AV1NVENC {
			t.Errorf("dual telemetry = %+v, want CPU %d GPU %d", rt, cpu, gpu)
		}
	}
}
