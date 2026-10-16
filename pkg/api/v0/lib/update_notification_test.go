package lib

import (
	"reflect"
	"testing"
	"time"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// notifiableObject is a saved object with one field outside reconciliation state.
type notifiableObject struct {
	api.Common
	api.Reconciliation
	ValuesDocument *string
}

// TestObjectUpdateNotifiable covers when a saved object notifies the controller.
func TestObjectUpdateNotifiable(t *testing.T) {
	yes := true
	no := false
	earlier := time.Now().Add(-time.Minute)
	later := time.Now()
	oldValues := "old"
	newValues := "new"

	before := notifiableObject{
		Reconciliation: api.Reconciliation{
			Reconciled:           &no,
			CreationAcknowledged: &earlier,
		},
		ValuesDocument: &oldValues,
	}

	// a spec change in the same write as the stamp must not notify
	stampedWithValues := before
	stampedWithValues.Reconciled = &yes
	stampedWithValues.ValuesDocument = &newValues
	assertUpdate(t, before, stampedWithValues, false, "ValuesDocument")

	// the controller's stamp of Reconciled to true must not notify
	stamped := before
	stamped.Reconciled = &yes
	assertUpdate(t, before, stamped, false)

	// UpdatedAt moves on every write and must not turn a stamp into a notification
	stampedAt := stamped
	stampedAt.UpdatedAt = &later
	assertUpdate(t, before, stampedAt, false)

	// a marker that moves with the stamp is named and still does not notify
	failed := true
	stampedAndFailed := stampedWithValues
	stampedAndFailed.CreationFailed = &failed
	assertUpdate(t, before, stampedAndFailed, false, "CreationFailed", "ValuesDocument")

	// an unset Reconciled that becomes true is the same transition
	unset := before
	unset.Reconciled = nil
	assertUpdate(t, unset, stampedWithValues, false, "ValuesDocument")

	// notify when both snapshots are equal
	unchanged := before
	assertUpdate(t, before, unchanged, true)

	// a restamped acknowledgement is not a state change
	restamp := unchanged
	restamp.CreationAcknowledged = &later
	assertUpdate(t, unchanged, restamp, false)

	// moving another marker is a state change
	moved := unchanged
	moved.CreationFailed = &failed
	assertUpdate(t, unchanged, moved, true)

	// clearing Reconciled is a state change
	cleared := stamped
	cleared.Reconciled = &no
	assertUpdate(t, stamped, cleared, true)

	// a spec edit while Reconciled stays true still notifies
	edited := stamped
	edited.ValuesDocument = &newValues
	assertUpdate(t, stamped, edited, true)
}

// assertUpdate checks the notify decision and the fields named for a warning.
func assertUpdate(t *testing.T, before, after any, wantNotify bool, wantChanged ...string) {
	t.Helper()

	// compare the decision and the warning names
	notify, changed := ObjectUpdateNotifiable(before, after)
	if notify != wantNotify {
		t.Errorf("notify = %v, want %v", notify, wantNotify)
	}
	if len(wantChanged) == 0 {
		wantChanged = nil
	}
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Errorf("changed = %v, want %v", changed, wantChanged)
	}
}

// TestNotifySnapshotIsolatesPointerFields covers a write that mutates the saved object's pointers.
func TestNotifySnapshotIsolatesPointerFields(t *testing.T) {
	values := "old"
	reconciled := false
	saved := notifiableObject{
		Reconciliation: api.Reconciliation{Reconciled: &reconciled},
		ValuesDocument:     &values,
	}
	snapshot := NotifySnapshot(saved)

	// write through the pointers the way a database scan does
	values = "new"
	reconciled = true
	if *snapshot.ValuesDocument != "old" || snapshot.Reconciled == nil || *snapshot.Reconciled {
		t.Errorf("snapshot must keep the values from before the write")
	}
	assertUpdate(t, snapshot, saved, false, "ValuesDocument")

	// a stamp that only flips the shared bool must not notify
	stampValues := "old"
	stampReconciled := false
	stampSaved := notifiableObject{
		Reconciliation: api.Reconciliation{Reconciled: &stampReconciled},
		ValuesDocument:     &stampValues,
	}
	stampSnapshot := NotifySnapshot(stampSaved)
	stampReconciled = true
	assertUpdate(t, stampSnapshot, stampSaved, false)
}

// TestReconciliationUpdateNotifiable covers when an update notifies the controller.
func TestReconciliationUpdateNotifiable(t *testing.T) {
	earlier := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Minute)
	no := false

	// a restamped acknowledgement must not notify
	restamped := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	after := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &later}
	if ReconciliationUpdateNotifiable(restamped, after) {
		t.Errorf("a refreshed acknowledgement alone must not notify; that is the publish loop")
	}

	// notify when both snapshots are equal
	unchanged := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	if !ReconciliationUpdateNotifiable(unchanged, unchanged) {
		t.Errorf("a spec edit leaves reconciliation state equal and must still notify")
	}

	// a moved state marker must notify
	failed := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	yes := true
	nowFailed := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier, CreationFailed: &yes}
	if !ReconciliationUpdateNotifiable(failed, nowFailed) {
		t.Errorf("a moved state marker must notify")
	}

	// the controller's stamp of Reconciled to true must not notify
	unstamped := api.Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	stamped := api.Reconciliation{Reconciled: &yes, CreationAcknowledged: &earlier}
	if ReconciliationUpdateNotifiable(unstamped, stamped) {
		t.Errorf("a Reconciled stamp must not schedule another reconcile")
	}

	// clearing Reconciled must still notify
	if !ReconciliationUpdateNotifiable(stamped, unstamped) {
		t.Errorf("clearing Reconciled must notify")
	}

	// a stamp that also moves another marker must notify
	stampedAndFailed := api.Reconciliation{Reconciled: &yes, CreationAcknowledged: &earlier, CreationFailed: &yes}
	if !ReconciliationUpdateNotifiable(unstamped, stampedAndFailed) {
		t.Errorf("a stamp that also moves another marker must notify")
	}
}

// TestChangeDetection covers the pointer helpers under ReconciliationStateChanged.
func TestChangeDetection(t *testing.T) {
	// check nil against nil
	var nilBoolA, nilBoolB *bool
	if got := boolPtrEqual(nilBoolA, nilBoolB); !got {
		t.Errorf("boolPtrEqual(nil, nil) = false, want true")
	}

	// check equal values behind distinct pointers
	trueA := true
	trueB := true
	if got := boolPtrEqual(&trueA, &trueB); !got {
		t.Errorf("boolPtrEqual(&true, &true) with distinct backing = false, want true")
	}
	if &trueA == &trueB {
		t.Fatalf("test setup: expected distinct pointer identity")
	}

	// check a time against its Round(0) form
	withMono := time.Now().UTC()
	stripped := withMono.Round(0)
	if got := timePtrEqual(&withMono, &stripped); !got {
		t.Errorf("timePtrEqual(withMono, stripped) = false, want true (same instant)")
	}
	if reflect.DeepEqual(withMono, stripped) {
		t.Logf("note: reflect.DeepEqual returned true here; monotonic reading may already be absent")
	}

	// check the same instant in two locations
	instant := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	sameInstantLocal := instant.In(time.Local)
	if got := timePtrEqual(&instant, &sameInstantLocal); !got {
		t.Errorf("timePtrEqual across loc = false, want true")
	}

	// check set against unset
	if got := timePtrSet(&withMono, &stripped); !got {
		t.Errorf("timePtrSet(both set) = false, want true")
	}
	var nilTime *time.Time
	if got := timePtrSet(nilTime, nilTime); !got {
		t.Errorf("timePtrSet(nil, nil) = false, want true")
	}
	if got := timePtrSet(&withMono, nilTime); got {
		t.Errorf("timePtrSet(set, nil) = true, want false")
	}

	cipherA := []byte{0x01, 0x02, 0x03, 0x04}
	cipherB := []byte{0x0a, 0x0b, 0x0c, 0x0d}
	if reflect.DeepEqual(cipherA, cipherB) {
		t.Fatalf("test setup: expected differing ciphertexts")
	}
	t.Logf("byte-slice pair with differing ciphertexts: no helper on api.Reconciliation; naive DeepEqual = false")

	// check identical Reconciliations
	base := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	copyOf := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	if got := ReconciliationStateChanged(base, copyOf); got {
		t.Errorf("ReconciliationStateChanged(identical copies) = true, want false")
	}

	// check fresh vs Round(0) instants
	fresh := makeReconciliation(true, false, withMono, withMono, withMono, withMono, withMono)
	fromDB := makeReconciliation(true, false, stripped, stripped, stripped, stripped, stripped)
	if got := ReconciliationStateChanged(fresh, fromDB); got {
		t.Errorf("ReconciliationStateChanged(fresh vs DB-round-trip) = true, want false")
	}

	// check UTC vs Local at the same instant
	utc := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	local := makeReconciliation(true, false, sameInstantLocal, sameInstantLocal, sameInstantLocal, sameInstantLocal, sameInstantLocal)
	if got := ReconciliationStateChanged(utc, local); got {
		t.Errorf("ReconciliationStateChanged(UTC vs Local same instant) = true, want false")
	}

	// check an acknowledgement re-stamp
	later := instant.Add(1 * time.Second)
	prev := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	restamped := makeReconciliation(true, false, later, instant, instant, later, instant)
	if got := ReconciliationStateChanged(prev, restamped); got {
		t.Errorf("ReconciliationStateChanged(ack re-stamp) = true, want false")
	}

	// check unset to set CreationConfirmed
	noConfirm := api.Reconciliation{
		Reconciled:           util.Ptr(true),
		CreationAcknowledged: util.Ptr(instant),
	}
	withConfirm := api.Reconciliation{
		Reconciled:           util.Ptr(true),
		CreationAcknowledged: util.Ptr(instant),
		CreationConfirmed:    util.Ptr(instant),
	}
	if got := ReconciliationStateChanged(noConfirm, withConfirm); !got {
		t.Errorf("ReconciliationStateChanged(unset -> set CreationConfirmed) = false, want true")
	}

	// check a Reconciled flip
	unreconciled := api.Reconciliation{Reconciled: util.Ptr(false)}
	reconciled := api.Reconciliation{Reconciled: util.Ptr(true)}
	if got := ReconciliationStateChanged(unreconciled, reconciled); !got {
		t.Errorf("ReconciliationStateChanged(Reconciled flip) = false, want true")
	}
}

// makeReconciliation sets every field ReconciliationStateChanged compares.
func makeReconciliation(
	reconciled, creationFailed bool,
	creationAck, creationConfirmed, deletionScheduled, deletionAck, deletionConfirmed time.Time,
) api.Reconciliation {
	return api.Reconciliation{
		Reconciled:           util.Ptr(reconciled),
		CreationAcknowledged: util.Ptr(creationAck),
		CreationConfirmed:    util.Ptr(creationConfirmed),
		CreationFailed:       util.Ptr(creationFailed),
		DeletionScheduled:    util.Ptr(deletionScheduled),
		DeletionAcknowledged: util.Ptr(deletionAck),
		DeletionConfirmed:    util.Ptr(deletionConfirmed),
	}
}
