package v0

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// Reconciliation carries the markers a saved object embeds.
type Reconciliation struct {
	Reconciled           *bool
	CreationAcknowledged *time.Time
	CreationConfirmed    *time.Time
	CreationFailed       *bool
	DeletionScheduled    *time.Time
	DeletionAcknowledged *time.Time
	DeletionConfirmed    *time.Time
}

// AlreadyReconciled reports whether Reconciled is set to true.
func (r Reconciliation) AlreadyReconciled() bool {
	// treat a nil pointer as not reconciled
	return r.Reconciled != nil && *r.Reconciled
}

// notifiableObject is a saved object with one field outside reconciliation state.
type notifiableObject struct {
	UpdatedAt *time.Time
	Reconciliation
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
		Reconciliation: Reconciliation{
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
		Reconciliation: Reconciliation{Reconciled: &reconciled},
		ValuesDocument: &values,
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
		Reconciliation: Reconciliation{Reconciled: &stampReconciled},
		ValuesDocument: &stampValues,
	}
	stampSnapshot := NotifySnapshot(stampSaved)
	stampReconciled = true
	assertUpdate(t, stampSnapshot, stampSaved, false)
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
	t.Logf("byte-slice pair with differing ciphertexts: no helper on Reconciliation; naive DeepEqual = false")

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
	noConfirm := Reconciliation{
		Reconciled:           util.Ptr(true),
		CreationAcknowledged: util.Ptr(instant),
	}
	withConfirm := Reconciliation{
		Reconciled:           util.Ptr(true),
		CreationAcknowledged: util.Ptr(instant),
		CreationConfirmed:    util.Ptr(instant),
	}
	if got := ReconciliationStateChanged(noConfirm, withConfirm); !got {
		t.Errorf("ReconciliationStateChanged(unset -> set CreationConfirmed) = false, want true")
	}

	// check a Reconciled flip
	unreconciled := Reconciliation{Reconciled: util.Ptr(false)}
	reconciled := Reconciliation{Reconciled: util.Ptr(true)}
	if got := ReconciliationStateChanged(unreconciled, reconciled); !got {
		t.Errorf("ReconciliationStateChanged(Reconciled flip) = false, want true")
	}
}

// makeReconciliation sets every field ReconciliationStateChanged compares.
func makeReconciliation(
	reconciled, creationFailed bool,
	creationAck, creationConfirmed, deletionScheduled, deletionAck, deletionConfirmed time.Time,
) Reconciliation {
	return Reconciliation{
		Reconciled:           util.Ptr(reconciled),
		CreationAcknowledged: util.Ptr(creationAck),
		CreationConfirmed:    util.Ptr(creationConfirmed),
		CreationFailed:       util.Ptr(creationFailed),
		DeletionScheduled:    util.Ptr(deletionScheduled),
		DeletionAcknowledged: util.Ptr(deletionAck),
		DeletionConfirmed:    util.Ptr(deletionConfirmed),
	}
}

// savedWithEmbed embeds CommonLike and Reconciliation. changedStructFields descends into each one.
type savedWithEmbed struct {
	UpdatedAt  *time.Time
	Reconciled *bool
	CommonLike
	Reconciliation
	ValuesDocument *string
}

// CommonLike has one named field and no embed, which is the shape of Common.
type CommonLike struct {
	DeletedAt *string
}

// namedBox is a struct field that must be reported by its own name.
type namedBox struct {
	Name *string
}

// savedWithNamed holds a named struct beside the reconciliation embed.
type savedWithNamed struct {
	UpdatedAt *time.Time
	Nested    namedBox
	Reconciliation
}

// TestNotificationHelpers covers each helper branch the notify decision calls.
func TestNotificationHelpers(t *testing.T) {
	check := func(name string, got, want bool) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	instant := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	later := instant.Add(time.Hour)

	// name an embed field, and skip UpdatedAt and Reconciled
	left := savedWithEmbed{
		UpdatedAt:      util.Ptr(instant),
		Reconciled:     util.Ptr(false),
		CommonLike:     CommonLike{DeletedAt: util.Ptr("old")},
		Reconciliation: Reconciliation{CreationFailed: util.Ptr(false)},
		ValuesDocument: util.Ptr("old"),
	}
	right := left
	right.UpdatedAt = util.Ptr(later)
	right.Reconciled = util.Ptr(true)
	right.CreationFailed = util.Ptr(true)
	right.DeletedAt = util.Ptr("new")
	names := changedStructFields(reflect.ValueOf(left), reflect.ValueOf(right))
	if got, want := fmt.Sprint(names), "[DeletedAt CreationFailed]"; got != want {
		t.Errorf("changedStructFields(embed) = %s, want %s", got, want)
	}

	// report a named struct as one field
	namedLeft := savedWithNamed{Nested: namedBox{Name: util.Ptr("old")}}
	namedRight := savedWithNamed{Nested: namedBox{Name: util.Ptr("new")}}
	named := changedStructFields(reflect.ValueOf(namedLeft), reflect.ValueOf(namedRight))
	if got, want := fmt.Sprint(named), "[Nested]"; got != want {
		t.Errorf("changedStructFields(named struct) = %s, want %s", got, want)
	}

	// UpdatedAt alone is not a spec change, and a skipped embed hides its marker
	updatedOnly := right
	updatedOnly.CreationFailed = left.CreationFailed
	updatedOnly.DeletedAt = left.DeletedAt
	updatedOnly.Reconciled = left.Reconciled
	check("structFieldsChanged(UpdatedAt)", structFieldsChanged(reflect.ValueOf(left), reflect.ValueOf(updatedOnly), true), false)
	failed := left
	failed.CreationFailed = util.Ptr(true)
	check("structFieldsChanged(skip embed)", structFieldsChanged(reflect.ValueOf(left), reflect.ValueOf(failed), true), false)
	check("structFieldsChanged(read embed)", structFieldsChanged(reflect.ValueOf(left), reflect.ValueOf(failed), false), true)

	// compare pointers, instants, and a slice through reflect.DeepEqual
	check("fieldValuesEqual(*bool)", fieldValuesEqual(reflect.ValueOf(util.Ptr(true)), reflect.ValueOf(util.Ptr(true))), true)
	check("fieldValuesEqual(nil *bool)", fieldValuesEqual(reflect.ValueOf((*bool)(nil)), reflect.ValueOf(util.Ptr(true))), false)
	withMono := time.Now().UTC()
	check("fieldValuesEqual(*time.Time)", fieldValuesEqual(reflect.ValueOf(&withMono), reflect.ValueOf(util.Ptr(withMono.Round(0)))), true)
	check("fieldValuesEqual([]string)", fieldValuesEqual(reflect.ValueOf([]string{"a"}), reflect.ValueOf([]string{"a"})), true)
	check("fieldValuesEqual(different len)", fieldValuesEqual(reflect.ValueOf([]string{"a"}), reflect.ValueOf([]string{"a", "b"})), false)

	// peel a pointer, and reject nil, a non-struct, and two types
	same := notifiableObject{ValuesDocument: util.Ptr("old")}
	_, _, sameOK := persistedStructs(&same, same)
	check("persistedStructs(pointer and value)", sameOK, true)
	_, _, nilOK := persistedStructs(nil, nil)
	check("persistedStructs(nil)", nilOK, false)
	_, _, textOK := persistedStructs("old", "new")
	check("persistedStructs(string)", textOK, false)
	_, _, typeOK := persistedStructs(same, struct{ N int }{})
	check("persistedStructs(two types)", typeOK, false)
	notify, changed := ObjectUpdateNotifiable(nil, nil)
	check("ObjectUpdateNotifiable(nil)", notify, true)
	if changed != nil {
		t.Errorf("ObjectUpdateNotifiable(nil) names = %v, want nil", changed)
	}

	// copy a pointer, a nil pointer, and a time.Time by value
	text := "old"
	copiedText := deepCopy(reflect.ValueOf(&text))
	text = "new"
	if copiedText.Elem().String() != "old" {
		t.Errorf("deepCopy(*string) = %s, want old", copiedText.Elem().String())
	}
	var nilText *string
	if !deepCopy(reflect.ValueOf(nilText)).IsNil() {
		t.Errorf("deepCopy(nil *string) is non-nil")
	}
	zone := time.FixedZone("Test", 3600)
	copiedTime := deepCopy(reflect.ValueOf(instant.In(zone))).Interface().(time.Time)
	if copiedTime.Location().String() != zone.String() {
		t.Errorf("deepCopy(time.Time) location = %s, want %s", copiedTime.Location(), zone)
	}
	snapshot := NotifySnapshot(&same)
	*same.ValuesDocument = "new"
	if got := *snapshot.ValuesDocument; got != "old" {
		t.Errorf("NotifySnapshot(*object) = %s, want old", got)
	}

	// read reconciliation state, including a missing embed and a pointer embed
	_, haveState := reconciliationFrom(reflect.ValueOf(same))
	check("reconciliationFrom(embed)", haveState, true)
	_, missingState := reconciliationFrom(reflect.ValueOf(struct{ N int }{}))
	check("reconciliationFrom(missing)", missingState, false)
	_, pointerState := reconciliationFrom(reflect.ValueOf(struct{ Reconciliation *Reconciliation }{}))
	check("reconciliationFrom(pointer)", pointerState, false)
	check("AlreadyReconciled(nil)", AlreadyReconciled(nil), false)
	check("AlreadyReconciled(false)", AlreadyReconciled(Reconciliation{Reconciled: util.Ptr(false)}), false)
	check("AlreadyReconciled(true)", AlreadyReconciled(Reconciliation{Reconciled: util.Ptr(true)}), true)

	// copy a set field, and treat a missing or non-pointer field as unset
	if got := boolField(Reconciliation{Reconciled: util.Ptr(true)}, "Reconciled"); got == nil || !*got {
		t.Errorf("boolField(true) = %v, want true", got)
	}
	if got := boolField(Reconciliation{}, "Reconciled"); got != nil {
		t.Errorf("boolField(unset) = %v, want nil", got)
	}
	if got := boolField(struct{ Reconciled bool }{}, "Reconciled"); got != nil {
		t.Errorf("boolField(non-pointer) = %v, want nil", got)
	}
	if got := timeField(Reconciliation{CreationAcknowledged: util.Ptr(instant)}, "CreationAcknowledged"); got == nil || !got.Equal(instant) {
		t.Errorf("timeField(set) = %v, want %s", got, instant)
	}
	if got := timeField(Reconciliation{}, "CreationAcknowledged"); got != nil {
		t.Errorf("timeField(unset) = %v, want nil", got)
	}

	// compare presence and instants for the pointer helpers
	check("boolPtrEqual(nil, set)", boolPtrEqual(nil, util.Ptr(true)), false)
	check("boolPtrEqual(false, false)", boolPtrEqual(util.Ptr(false), util.Ptr(false)), true)
	check("boolPtrEqual(false, true)", boolPtrEqual(util.Ptr(false), util.Ptr(true)), false)
	check("timePtrSet(nil, nil)", timePtrSet(nil, nil), true)
	check("timePtrSet(nil, set)", timePtrSet(nil, &instant), false)
	check("timePtrSet(different instants)", timePtrSet(&instant, &later), true)
	check("timePtrEqual(nil, set)", timePtrEqual(nil, &instant), false)
	check("timePtrRefreshed(nil)", timePtrRefreshed(nil, nil), false)
	check("timePtrRefreshed(one nil)", timePtrRefreshed(nil, &instant), false)
	check("timePtrRefreshed(same)", timePtrRefreshed(&instant, util.Ptr(instant)), false)
	check("timePtrRefreshed(different)", timePtrRefreshed(&instant, &later), true)

	// acknowledgementRefreshed reports a moved creation or deletion acknowledgement.
	// ReconciliationStateChanged ignores InterruptReconciliation.
	check("acknowledgementRefreshed(same)", acknowledgementRefreshed(
		Reconciliation{CreationAcknowledged: util.Ptr(instant)},
		Reconciliation{CreationAcknowledged: util.Ptr(instant)},
	), false)
	check("acknowledgementRefreshed(creation)", acknowledgementRefreshed(
		Reconciliation{CreationAcknowledged: util.Ptr(instant)},
		Reconciliation{CreationAcknowledged: util.Ptr(later)},
	), true)
	check("acknowledgementRefreshed(deletion)", acknowledgementRefreshed(
		Reconciliation{DeletionAcknowledged: util.Ptr(instant)},
		Reconciliation{DeletionAcknowledged: util.Ptr(later)},
	), true)
	type gated struct {
		Reconciled              *bool
		InterruptReconciliation *bool
	}
	check("ReconciliationStateChanged(interrupt)", ReconciliationStateChanged(
		gated{Reconciled: util.Ptr(true), InterruptReconciliation: util.Ptr(false)},
		gated{Reconciled: util.Ptr(true), InterruptReconciliation: util.Ptr(true)},
	), false)
}
