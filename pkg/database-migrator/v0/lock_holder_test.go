package v0

import (
	"os"
	"strings"
	"testing"
)

// TestOnlyAnInitContainerSharesItsLockHolder covers the name a migrator holds
// the migration lock under.  An init container uses its pod's host name, so
// one restarted in the same pod takes back its lock.  Any other run uses a
// name of its own, since a holder takes back a lock it already holds, and two
// runs by hand on one machine sharing the host name would both migrate.
func TestOnlyAnInitContainerSharesItsLockHolder(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("read host name: %v", err)
	}

	for i := 0; i < 2; i++ {
		holder, err := LockHolder(true)
		if err != nil {
			t.Fatalf("init container holder: %v", err)
		}
		if holder != host {
			t.Errorf("an init container holds the lock as %q, not as its pod %q, so a restart cannot take it back", holder, host)
		}
	}

	first, err := LockHolder(false)
	if err != nil {
		t.Fatalf("holder: %v", err)
	}
	second, err := LockHolder(false)
	if err != nil {
		t.Fatalf("holder: %v", err)
	}
	if first == second {
		t.Errorf("two runs on one machine both hold the lock as %q", first)
	}
	for _, holder := range []string{first, second} {
		if holder == host || !strings.HasPrefix(holder, host+"/") {
			t.Errorf("a run by hand holds the lock as %q, which does not say which machine it runs on", holder)
		}
	}
}

// TestAPlainUpInAPodIsTheInitContainer covers how a plain 'up' tells it is
// the API server's init container: Kubernetes sets KUBERNETES_SERVICE_HOST
// in every container it runs.
func TestAPlainUpInAPodIsTheInitContainer(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	if !InPod() {
		t.Error("a migrator in a pod does not know it")
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	if InPod() {
		t.Error("a migrator run outside Kubernetes takes itself for an init container")
	}
}
