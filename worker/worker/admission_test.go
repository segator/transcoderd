package worker

import (
	"sync/atomic"
	"testing"
	"time"

	"transcoder/worker/config"
)

func testAdmissionWorker(max int) *JobExecutor {
	return &JobExecutor{
		workerConfig:  &config.Config{},
		maxActiveJobs: max,
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
	if !worker.ReserveJobSlot() {
		t.Fatal("first reservation failed")
	}
	if err := worker.SetMaxActiveJobs(1); err != nil {
		t.Fatal(err)
	}
	if worker.ReserveJobSlot() {
		t.Fatal("worker made another reservation after lowering the maximum")
	}
	worker.ReleaseJobSlot()
	if !worker.ReserveJobSlot() {
		t.Fatal("worker did not reserve after the in-flight reservation was released")
	}
	worker.ReleaseJobSlot()
}

func TestScheduleAndMaximumBothGateAdmission(t *testing.T) {
	now := time.Now()
	elapsed := time.Duration(now.Hour())*time.Hour + time.Duration(now.Minute())*time.Minute
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
	atomic.StoreUint32(&worker.activeJobs, 1)
	if worker.ReserveJobSlot() {
		t.Fatal("worker accepted a job above its maximum while inside the schedule")
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
