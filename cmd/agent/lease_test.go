package main

import (
	"testing"
	"time"
)

// TestLeaderElectionTimings checks the lease, renew, and retry intervals.
func TestLeaderElectionTimings(t *testing.T) {
	// read the intervals
	leaseDuration, renewDeadline, retryPeriod := leaderElectionTimings()

	// keep the renew inside the lease, with a client timeout of at least 15s
	if leaseDuration != 60*time.Second {
		t.Fatalf("lease duration %s", leaseDuration)
	}
	if renewDeadline != 40*time.Second {
		t.Fatalf("renew deadline %s", renewDeadline)
	}
	if retryPeriod != 5*time.Second {
		t.Fatalf("retry period %s", retryPeriod)
	}
	if renewDeadline >= leaseDuration {
		t.Fatalf("renew deadline %s is not inside lease duration %s", renewDeadline, leaseDuration)
	}
	if renewDeadline/2 < 15*time.Second {
		t.Fatalf("lease client timeout %s is under 15s", renewDeadline/2)
	}
}
