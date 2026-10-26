package worker

import (
	"sync/atomic"
	"testing"
	"time"

	"transcoder/worker/config"
)

func testAdmissionWorker(max int) *JobExecutor {
	workerConfig := &config.Config{MaxActiveJobs: max}
	return &JobExecutor{
		workerConfig:  workerConfig,
		maxActiveJobs: max,
		encodePool:    newEncodeConcurrency(workerConfig.EncodeParallelJobs()),
	}
}

func TestInitialMaximumDefaultsToTwo(t *testing.T) {
	if got := config.DefaultMaxActiveJobs; got != 2 {
		t.Fatalf("DefaultMaxActiveJobs = %d, want 2", got)
	}
	if got := testAdmissionWorker(0).AdmissionStatus().MaxActiveJobs; got != 0 {
		t.Fatalf("initial maximum with zero configured = %d, want 0", got)
	}
}

func TestZeroMaximumStopsAdmissionAndPositiveMaximumRestoresIt(t *testing.T) {
	worker := testAdmissionWorker(2)
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	if worker.ReserveJobSlot() {
		t.Fatal("ReserveJobSlot() succeeded with a zero maximum")
	}
	if err := worker.SetMaxActiveJobs(2); err != nil {
		t.Fatal(err)
	}
	if !worker.ReserveJobSlot() {
		t.Fatal("ReserveJobSlot() failed after admission was re-enabled")
	}
	worker.ReleaseJobSlot()
}

func TestAdmissionAllowsExactlyTwoPipelineBufferJobs(t *testing.T) {
	worker := testAdmissionWorker(2)
	for i := 0; i < 4; i++ {
		if !worker.ReserveJobSlot() {
			t.Fatalf("ReserveJobSlot() failed for accepted job %d of 4", i+1)
		}
	}
	if worker.ReserveJobSlot() {
		t.Fatal("ReserveJobSlot() accepted a fifth job beyond two encode slots and two pipeline buffer jobs")
	}
	status := worker.AdmissionStatus()
	if status.MaxActiveJobs != 2 || status.MaxAcceptedJobs != 4 {
		t.Fatalf("status = encode target %d, admission ceiling %d; want 2 and 4", status.MaxActiveJobs, status.MaxAcceptedJobs)
	}
	for i := 0; i < 4; i++ {
		worker.ReleaseJobSlot()
	}
}

func TestAdmissionCeilingIsZeroWhenDisabled(t *testing.T) {
	worker := testAdmissionWorker(0)
	if got := worker.AdmissionStatus().MaxAcceptedJobs; got != 0 {
		t.Fatalf("MaxAcceptedJobs with zero target = %d, want 0", got)
	}
	if worker.ReserveJobSlot() {
		t.Fatal("ReserveJobSlot() succeeded with a zero maximum")
	}
}

func TestAdmissionCeilingUsesEffectiveEncodeOverride(t *testing.T) {
	worker := testAdmissionWorker(4)
	worker.encodePool.SetTarget(2)
	if got := worker.AdmissionStatus().MaxAcceptedJobs; got != 4 {
		t.Fatalf("MaxAcceptedJobs with two encodes and a two-job buffer = %d, want 4", got)
	}
	for i := 0; i < 4; i++ {
		if !worker.ReserveJobSlot() {
			t.Fatalf("reservation %d failed below the effective admission ceiling", i+1)
		}
	}
	if worker.ReserveJobSlot() {
		t.Fatal("worker admitted more than two jobs beyond the effective encode target")
	}
	for i := 0; i < 4; i++ {
		worker.ReleaseJobSlot()
	}
}

func TestZeroMaximumRetainsEncodeTargetForAcceptedJobs(t *testing.T) {
	worker := testAdmissionWorker(2)
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	status := worker.AdmissionStatus()
	if status.Accepting || status.MaxEncodes != 2 {
		t.Errorf("status after disabling admission = accepting %t, encodes %d, want false and 2", status.Accepting, status.MaxEncodes)
	}
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	if got := worker.AdmissionStatus().MaxEncodes; got != 1 {
		t.Errorf("encode target after setting max to one = %d, want 1", got)
	}
	if err := worker.SetMaxActiveJobs(0); err != nil {
		t.Fatal(err)
	}
	if got := worker.AdmissionStatus().MaxEncodes; got != 1 {
		t.Errorf("zero max changed the encode target to %d, want 1", got)
	}
}

func TestLoweredMaximumWaitsForActiveJobs(t *testing.T) {
	worker := testAdmissionWorker(4)
	atomic.StoreUint32(&worker.activeJobs, 4)
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	if worker.ReserveJobSlot() {
		t.Fatal("worker accepted a job above the lowered maximum")
	}
	atomic.StoreUint32(&worker.activeJobs, 0)
	if !worker.ReserveJobSlot() {
		t.Fatal("worker did not accept a job after active jobs fell below the maximum")
	}
	worker.ReleaseJobSlot()
}

func TestReservationCountsAgainstConcurrentLimitChange(t *testing.T) {
	worker := testAdmissionWorker(2)
	for i := 0; i < 3; i++ {
		if !worker.ReserveJobSlot() {
			t.Fatalf("reservation %d failed before lowering the maximum", i+1)
		}
	}
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	if worker.ReserveJobSlot() {
		t.Fatal("worker made another reservation above the lowered admission ceiling")
	}
	for i := 0; i < 3; i++ {
		worker.ReleaseJobSlot()
	}
	if !worker.ReserveJobSlot() {
		t.Fatal("worker did not reserve after an in-flight reservation was released")
	}
	worker.ReleaseJobSlot()
}

func TestScheduleAndMaximumBothGateAdmission(t *testing.T) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	elapsed := now.Sub(midnight)
	start := elapsed + time.Hour
	stop := start + time.Hour
	worker := testAdmissionWorker(1)
	worker.workerConfig.StartAfter = &start
	worker.workerConfig.StopAfter = &stop
	if worker.ReserveJobSlot() {
		t.Fatal("worker accepted a job outside its configured window")
	}

	start = elapsed - time.Minute
	stop = start + 2*time.Hour
	atomic.StoreUint32(&worker.activeJobs, 3)
	if worker.ReserveJobSlot() {
		t.Fatal("worker accepted a job above its admission ceiling while inside the schedule")
	}
}

func TestSetMaximumRejectsNegativeValues(t *testing.T) {
	worker := testAdmissionWorker(2)
	for _, value := range []int{-1} {
		if err := worker.SetMaxActiveJobs(value); err == nil {
			t.Errorf("SetMaxActiveJobs(%d) returned no error", value)
		}
	}
	if got := worker.AdmissionStatus().MaxActiveJobs; got != 2 {
		t.Fatalf("maximum after invalid updates = %d, want 2", got)
	}
}
