package v0

import (
	"time"

	"gorm.io/gorm"
)

// Common includes standard fields included in most objects.
type Common struct {
	ID        *uint           `json:",omitempty" gorm:"primarykey"`
	CreatedAt *time.Time      `json:",omitempty"`
	UpdatedAt *time.Time      `json:",omitempty"`
	DeletedAt *gorm.DeletedAt `json:",omitempty" gorm:"index"`
}

// Reconciliation includes the fields for reconciled objects.  These are
// leveraged by controllers to persist information related to the reconciliation
// of system state for objects.
type Reconciliation struct {
	// Indicates if object is considered to be reconciled by the object's controller.
	Reconciled *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	CreationAcknowledged *time.Time `json:",omitempty" validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	CreationConfirmed *time.Time `json:",omitempty" validate:"optional"`

	// Gets set to true if creation process fails.
	CreationFailed *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Used to inform reconcilers that an object is being deleted so they may
	// complete delete reconciliation before actually deleting the object from the database.
	DeletionScheduled *time.Time `json:",omitempty" validate:"optional"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	DeletionAcknowledged *time.Time `json:",omitempty" validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	DeletionConfirmed *time.Time `json:",omitempty" validate:"optional"`

	// InterruptReconciliation is used by the controller to indicated that future
	// reconcilation should be interrupted.  Useful in cases where there is a
	// situation where future reconciliation could be descructive such as
	// spinning up more infrastructure when there is a unresolved problem.
	InterruptReconciliation *bool `json:",omitempty" validate:"optional" gorm:"default:false"`
}

// ReconciliationStateChanged reports whether a reconciliation progress marker
// differs between a and b.  Acknowledgement timestamps count only as set or
// unset, so re-stamping one is not a change.  InterruptReconciliation gates
// whether a controller acts rather than marking progress, so it is not compared.
func ReconciliationStateChanged(a, b Reconciliation) bool {
	return !boolPtrEqual(a.Reconciled, b.Reconciled) ||
		!timePtrSet(a.CreationAcknowledged, b.CreationAcknowledged) ||
		!timePtrEqual(a.CreationConfirmed, b.CreationConfirmed) ||
		!boolPtrEqual(a.CreationFailed, b.CreationFailed) ||
		!timePtrEqual(a.DeletionScheduled, b.DeletionScheduled) ||
		!timePtrSet(a.DeletionAcknowledged, b.DeletionAcknowledged) ||
		!timePtrEqual(a.DeletionConfirmed, b.DeletionConfirmed)
}

// ReconciliationUpdateNotifiable reports whether an update from a to b warrants
// notifying the object's controller.  An update moving no reconciliation state
// still notifies, since a spec edit moves none.  A refreshed acknowledgement
// does not: a controller re-stamps its own every 60 seconds during an
// infrastructure operation, and that notification would wake its author.
func ReconciliationUpdateNotifiable(a, b Reconciliation) bool {
	if ReconciliationStateChanged(a, b) {
		return true
	}
	return !acknowledgementRefreshed(a, b)
}

// acknowledgementRefreshed reports whether a creation or deletion
// acknowledgement moved to a new instant between a and b.
func acknowledgementRefreshed(a, b Reconciliation) bool {
	return timePtrRefreshed(a.CreationAcknowledged, b.CreationAcknowledged) ||
		timePtrRefreshed(a.DeletionAcknowledged, b.DeletionAcknowledged)
}

// timePtrRefreshed reports whether a and b are both set and hold different
// instants.
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

// timePtrEqual reports whether a and b are both unset or hold the same instant.
// It compares instants because == on time.Time also compares the location and
// the monotonic clock reading, and a value read back from the database arrives
// in the local zone while the API writes UTC.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// timePtrSet reports whether a and b are both set or both unset, ignoring the
// instants they hold.
func timePtrSet(a, b *time.Time) bool {
	return (a == nil) == (b == nil)
}
