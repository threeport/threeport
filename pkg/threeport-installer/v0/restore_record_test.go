package v0

import (
	"testing"
)

// TestClearRestoreRecordLetsTheNextRestoreStart records a source and then
// drops it the way the resume closure does.
func TestClearRestoreRecordLetsTheNextRestoreStart(t *testing.T) {
	// record the source the way a finished restore does
	_, client, cpi := newBrokerTestClient()
	cpi.Opts.FromNamespace = "primary-02"
	if err := cpi.recordRestore(client); err != nil {
		t.Fatal(err)
	}
	recorded, err := cpi.restoreRecorded(client)
	if err != nil || !recorded {
		t.Fatalf("recorded %v err %v", recorded, err)
	}

	// drop the record before the source writers resume
	if err := cpi.clearRestoreRecord(client); err != nil {
		t.Fatal(err)
	}
	recorded, err = cpi.restoreRecorded(client)
	if err != nil {
		t.Fatal(err)
	}
	if recorded {
		t.Fatal("restore stayed recorded after the source resumed")
	}
}

// TestKeepSourceNamespaceSkipsTheSharedOperator reports the namespace the
// support-services operator lives in.
func TestKeepSourceNamespaceSkipsTheSharedOperator(t *testing.T) {
	// keep the shared operator namespace and delete a dedicated one
	if !KeepSourceNamespace(ControlPlaneNamespace) {
		t.Fatal("shared operator namespace would be deleted")
	}
	if KeepSourceNamespace("primary-02") {
		t.Fatal("dedicated namespace would be kept")
	}
}
