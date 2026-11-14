package v0

import (
	"errors"
	"strings"
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

// TestResumeDropsTheSavedRestoreSoTheRetryBacksUpAgain records a source,
// resumes the writers the way a failed restore does, and checks the retry.
func TestResumeDropsTheSavedRestoreSoTheRetryBacksUpAgain(t *testing.T) {
	// record a finished restore, then resume the source writers
	_, client, cpi := newBrokerTestClient()
	cpi.Opts.FromNamespace = "primary-02"
	source := &ControlPlaneInstaller{Opts: Options{Namespace: "primary-02"}}
	if err := cpi.recordRestore(client); err != nil {
		t.Fatal(err)
	}
	// resume panics without a mapper after the record is dropped
	func() {
		defer func() { _ = recover() }()
		err := resumeSourceWriters(cpi, source, client, nil, nil, "primary-02", errors.New("restore failed"))
		if err == nil || !strings.Contains(err.Error(), "restore failed") {
			t.Fatal(err)
		}
	}()
	recorded, err := cpi.restoreRecorded(client)
	if err != nil {
		t.Fatal(err)
	}
	if recorded {
		t.Fatal("saved restore stayed after the source writers resumed")
	}

	// the retry starts a new backup and does not return the recorded shortcut
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_, err = cpi.StopSourceAndRestore(client, "bucket")
	}()
	if err == nil && !panicked {
		t.Fatal("retry reused the saved restore")
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
