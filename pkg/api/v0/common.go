package v0

import (
	"time"

	"gorm.io/gorm"
)

// Common includes standard fields included in most objects.
type Common struct {
	ID        *uint `gorm:"primarykey"`
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *gorm.DeletedAt `gorm:"index"`
}

// Reconciliation includes the fields for reconciled objects.  These are
// leveraged by controllers to persist information related to the reconciliation
// of system state for objects.
type Reconciliation struct {
	// Indicates if object is considered to be reconciled by the object's controller.
	Reconciled *bool `validate:"optional" gorm:"default:false"`

	// The last time creation was acknowledged as begun
	CreationAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm creation of an object.
	CreationConfirmed *time.Time `json:",omitempty" validate:"optional"`

	// Gets set to true if creation process fails.
	CreationFailed *bool `validate:"optional" gorm:"default:false"`

	// Used to inform reconcilers that an object is being deleted so they may
	// complete delete reconciliation before actually deleting the object from the database.
	DeletionScheduled *time.Time `validate:"optional"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun.
	DeletionAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	DeletionConfirmed *time.Time `validate:"optional"`

	// A flag set to true if deletion of the object fails
	DeletionFailed *bool `validate:"optional" gorm:"default:false"`

	// InterruptReconciliation is used by the controller to indicate that future
	// reconciliation should be interrupted. Useful in cases where there is a
	// situation where future reconciliation could be destructive such as
	// spinning up more infrastructure when there is a unresolved problem.
	InterruptReconciliation *bool `validate:"optional" gorm:"default:false"`
}

// ReconciliationStateChanged reports whether progress markers differ.
// Acknowledgement timestamps count as set or unset only. InterruptReconciliation
// is ignored; it is a gate, not progress. CreationFailed and DeletionFailed
// compare by value.
func ReconciliationStateChanged(a, b Reconciliation) bool {
	return !boolPtrEqual(a.Reconciled, b.Reconciled) ||
		!timePtrSet(a.CreationAcknowledged, b.CreationAcknowledged) ||
		!timePtrEqual(a.CreationConfirmed, b.CreationConfirmed) ||
		!boolPtrEqual(a.CreationFailed, b.CreationFailed) ||
		!timePtrEqual(a.DeletionScheduled, b.DeletionScheduled) ||
		!timePtrSet(a.DeletionAcknowledged, b.DeletionAcknowledged) ||
		!timePtrEqual(a.DeletionConfirmed, b.DeletionConfirmed) ||
		!boolPtrEqual(a.DeletionFailed, b.DeletionFailed)
}

// ReconciliationUpdateNotifiable reports whether an update should notify the
// controller. Spec edits notify even when markers are unchanged. A restamped
// acknowledgement does not.
func ReconciliationUpdateNotifiable(a, b Reconciliation) bool {
	if ReconciliationStateChanged(a, b) {
		return true
	}
	return !acknowledgementRefreshed(a, b)
}

// acknowledgementRefreshed reports a restamped creation or deletion acknowledgement.
func acknowledgementRefreshed(a, b Reconciliation) bool {
	return timePtrRefreshed(a.CreationAcknowledged, b.CreationAcknowledged) ||
		timePtrRefreshed(a.DeletionAcknowledged, b.DeletionAcknowledged)
}

// timePtrRefreshed reports two set pointers holding different instants.
func timePtrRefreshed(a, b *time.Time) bool {
	return a != nil && b != nil && !a.Equal(*b)
}

// boolPtrEqual reports whether a and b are both unset or hold the same value.
func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// timePtrEqual compares instants. == on time.Time also compares location and
// the monotonic reading, which a database round trip drops.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// timePtrSet reports whether a and b are both set or both unset.
// A liveness re-stamp is not a state change.
func timePtrSet(a, b *time.Time) bool {
	return (a == nil) == (b == nil)
}
