package main

import (
	"testing"

	"transcoder/worker/control"
)

func TestCombinedProfileSafeIdleRequiresBothConsistentProfiles(t *testing.T) {
	yes := true
	ready := control.AdmissionStatus{SafeIdle: &yes, RestartBarrier: true, ActiveJobs: 1, ParkedJobs: 1}
	profiles := map[string]control.AdmissionStatus{"cpu": ready, "gpu": ready}
	if safe, barrier := combinedProfileSafeIdle(profiles); safe == nil || !*safe || !barrier {
		t.Fatal("two safely parked profiles were not reported idle")
	}
	broken := ready
	broken.ActiveEncodes = 1
	profiles["gpu"] = broken
	if safe, _ := combinedProfileSafeIdle(profiles); safe == nil || *safe {
		t.Fatal("active encode was reported safely idle")
	}
	broken = ready
	broken.SafeIdle = nil
	profiles["gpu"] = broken
	if safe, _ := combinedProfileSafeIdle(profiles); safe != nil {
		t.Fatal("unknown profile was reported safely idle")
	}
	delete(profiles, "gpu")
	if safe, barrier := combinedProfileSafeIdle(profiles); safe != nil || barrier {
		t.Fatal("missing profile was reported safely idle")
	}
}
