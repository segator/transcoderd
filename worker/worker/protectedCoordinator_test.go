package worker

import (
	"errors"
	"testing"
)

func TestProtectedPollingChecksUpdatesWhenQueueIsStopped(t *testing.T) {
	worker := testAdmissionWorker(0)
	checks := 0
	check := func() (bool, error) { checks++; return true, nil }
	required, reserved, err := protectedPollDecision(check, worker.ReserveJobSlot)
	if err != nil || !required || reserved || checks != 1 || worker.reservedJobs != 0 {
		t.Fatalf("stopped update decision = required %t, reserved %t, checks %d, error %v", required, reserved, checks, err)
	}
	check = func() (bool, error) { checks++; return false, nil }
	required, reserved, err = protectedPollDecision(check, worker.ReserveJobSlot)
	if err != nil || required || reserved || checks != 2 {
		t.Fatalf("stopped normal poll = required %t, reserved %t, checks %d, error %v", required, reserved, checks, err)
	}
	want := errors.New("update check failed")
	required, reserved, err = protectedPollDecision(func() (bool, error) { return false, want }, worker.ReserveJobSlot)
	if required || reserved || !errors.Is(err, want) {
		t.Fatalf("failed update check = required %t, reserved %t, error %v", required, reserved, err)
	}
}
