package v0

import "testing"

// TestRestoreFromNamespaceSkipsWhenUnset checks that a fresh install does not restore.
func TestRestoreFromNamespaceSkipsWhenUnset(t *testing.T) {
	// skip when no source namespace is set
	cpi := &ControlPlaneInstaller{}
	if err := cpi.RestoreFromNamespace(nil, ""); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreFromNamespaceRequiresBucket checks that a source namespace needs a bucket.
func TestRestoreFromNamespaceRequiresBucket(t *testing.T) {
	// refuse a restore that names a source and no bucket
	cpi := &ControlPlaneInstaller{Opts: Options{FromNamespace: "threeport-primary-01"}}
	if err := cpi.RestoreFromNamespace(nil, ""); err == nil {
		t.Fatal("expected an error")
	}
}

// TestRestoreFromNamespaceSkipsWhenRecorded checks that a recorded source is not restored again.
func TestRestoreFromNamespaceSkipsWhenRecorded(t *testing.T) {
	// record the source, then skip the restore
	_, kubeClient, cpi := newBrokerTestClient()
	cpi.Opts.FromNamespace = "threeport-primary-01"
	if err := cpi.recordRestore(kubeClient); err != nil {
		t.Fatal(err)
	}
	if err := cpi.RestoreFromNamespace(kubeClient, "backup-bucket"); err != nil {
		t.Fatal(err)
	}
}

// TestRestoreRecordedIsFalseForAnotherSource checks that a different source is not treated as done.
func TestRestoreRecordedIsFalseForAnotherSource(t *testing.T) {
	// record one source
	_, kubeClient, cpi := newBrokerTestClient()
	cpi.Opts.FromNamespace = "threeport-primary-01"
	if err := cpi.recordRestore(kubeClient); err != nil {
		t.Fatal(err)
	}

	// a different source is not recorded
	cpi.Opts.FromNamespace = "threeport-primary-02"
	recorded, err := cpi.restoreRecorded(kubeClient)
	if err != nil {
		t.Fatal(err)
	}
	if recorded {
		t.Fatal("expected the other source to be unrecorded")
	}
}
