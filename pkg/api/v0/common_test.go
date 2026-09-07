package v0

import (
	"reflect"
	"testing"
	"time"
)

// The update handlers call ReconciliationUpdateNotifiable only when an object's
// Reconciled is false, and controllers re-stamp CreationAcknowledged and
// DeletionAcknowledged every 60 seconds while an infrastructure operation runs.
// Publishing for one of those writes wakes the controller that made it, so
// ReconciliationStateChanged reads both acknowledgement fields as set or unset
// and a re-stamp of one already set is neither a change nor a notification.
// timePtrEqual compares instants with Equal, since == on time.Time also
// compares the location and the monotonic clock reading, and a time read back
// from the database carries neither.

// TestReconciliationUpdateNotifiable covers the predicate the update handlers
// call to decide whether to publish a change notification.
func TestReconciliationUpdateNotifiable(t *testing.T) {
	earlier := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	later := earlier.Add(time.Minute)
	no := false

	// check a refreshed acknowledgement on its own
	restamped := Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	after := Reconciliation{Reconciled: &no, CreationAcknowledged: &later}
	if ReconciliationUpdateNotifiable(restamped, after) {
		t.Errorf("a refreshed acknowledgement alone must not notify; that is the publish loop")
	}

	// check unchanged reconciliation state
	unchanged := Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	if !ReconciliationUpdateNotifiable(unchanged, unchanged) {
		t.Errorf("a spec edit leaves reconciliation state equal and must still notify")
	}

	// check a newly set state marker
	failed := Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier}
	yes := true
	nowFailed := Reconciliation{Reconciled: &no, CreationAcknowledged: &earlier, CreationFailed: &yes}
	if !ReconciliationUpdateNotifiable(failed, nowFailed) {
		t.Errorf("a moved state marker must notify")
	}
}

// TestChangeDetection covers the pointer comparison helpers and the
// ReconciliationStateChanged predicate built on them.
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

	// check the same instant carried in two locations
	instant := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	sameInstantLocal := instant.In(time.Local)
	if got := timePtrEqual(&instant, &sameInstantLocal); !got {
		t.Errorf("timePtrEqual across loc = false, want true")
	}

	// check set against unset, which is all timePtrSet compares
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

	// Reconciliation holds no byte-slice field, so no helper compares one
	cipherA := []byte{0x01, 0x02, 0x03, 0x04}
	cipherB := []byte{0x0a, 0x0b, 0x0c, 0x0d}
	if reflect.DeepEqual(cipherA, cipherB) {
		t.Fatalf("test setup: expected differing ciphertexts")
	}
	t.Logf("byte-slice pair with differing ciphertexts: no helper on Reconciliation; naive DeepEqual = false")

	// check two identically built Reconciliations
	base := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	copyOf := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	if got := ReconciliationStateChanged(base, copyOf); got {
		t.Errorf("ReconciliationStateChanged(identical copies) = true, want false")
	}

	// check the same instants in fresh and Round(0) form
	fresh := makeReconciliation(true, false, withMono, withMono, withMono, withMono, withMono)
	fromDB := makeReconciliation(true, false, stripped, stripped, stripped, stripped, stripped)
	if got := ReconciliationStateChanged(fresh, fromDB); got {
		t.Errorf("ReconciliationStateChanged(fresh vs DB-round-trip) = true, want false")
	}

	// check the same instants carried in two locations
	utc := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	local := makeReconciliation(true, false, sameInstantLocal, sameInstantLocal, sameInstantLocal, sameInstantLocal, sameInstantLocal)
	if got := ReconciliationStateChanged(utc, local); got {
		t.Errorf("ReconciliationStateChanged(UTC vs Local same instant) = true, want false")
	}

	// check an acknowledgement re-stamp on both acknowledgement fields
	later := instant.Add(1 * time.Second)
	prev := makeReconciliation(true, false, instant, instant, instant, instant, instant)
	restamped := makeReconciliation(true, false, later, instant, instant, later, instant)
	if got := ReconciliationStateChanged(prev, restamped); got {
		t.Errorf("ReconciliationStateChanged(ack re-stamp) = true, want false")
	}

	// check an unset to set transition on CreationConfirmed
	noConfirm := Reconciliation{
		Reconciled:           ptrBool(true),
		CreationAcknowledged: ptrTime(instant),
	}
	withConfirm := Reconciliation{
		Reconciled:           ptrBool(true),
		CreationAcknowledged: ptrTime(instant),
		CreationConfirmed:    ptrTime(instant),
	}
	if got := ReconciliationStateChanged(noConfirm, withConfirm); !got {
		t.Errorf("ReconciliationStateChanged(unset -> set CreationConfirmed) = false, want true")
	}

	// check a Reconciled flip
	unreconciled := Reconciliation{Reconciled: ptrBool(false)}
	reconciled := Reconciliation{Reconciled: ptrBool(true)}
	if got := ReconciliationStateChanged(unreconciled, reconciled); !got {
		t.Errorf("ReconciliationStateChanged(Reconciled flip) = false, want true")
	}
}

// makeReconciliation sets every field ReconciliationStateChanged compares, so
// no case differs by a field being left unset.
func makeReconciliation(
	reconciled, creationFailed bool,
	creationAck, creationConfirmed, deletionScheduled, deletionAck, deletionConfirmed time.Time,
) Reconciliation {
	return Reconciliation{
		Reconciled:           ptrBool(reconciled),
		CreationAcknowledged: ptrTime(creationAck),
		CreationConfirmed:    ptrTime(creationConfirmed),
		CreationFailed:       ptrBool(creationFailed),
		DeletionScheduled:    ptrTime(deletionScheduled),
		DeletionAcknowledged: ptrTime(deletionAck),
		DeletionConfirmed:    ptrTime(deletionConfirmed),
	}
}

func ptrBool(b bool) *bool { return &b }

func ptrTime(t time.Time) *time.Time { return &t }
