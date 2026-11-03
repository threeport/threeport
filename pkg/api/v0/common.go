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

	// The object that owns the row being created, and the field on that object that stores this row's id.
	// The value is read during create and is not stored on this row.
	Owner *OwnerRef `validate:"optional" gorm:"-"`
}

// OwnerRef names an existing object that owns the row a create request is inserting.
// The create writes the owns reference and sets Field on that object in the same transaction.
type OwnerRef struct {
	// The fully qualified type of the owning object.
	Type *string `validate:"required"`

	// The id of the owning object.
	ID *uint `validate:"required"`

	// The field on the owning object that stores this row's id.
	Field *string `validate:"required"`
}

// GetOwner returns the owner named by a create request.
func (c Common) GetOwner() *OwnerRef {
	return c.Owner
}

// Reconciliation includes the fields for reconciled objects.  These are
// leveraged by controllers to persist information related to the reconciliation
// of system state for objects.
type Reconciliation struct {
	// Indicates if object is considered to be reconciled by the object's controller.
	Reconciled *bool `validate:"optional" gorm:"default:false"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	CreationAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	CreationConfirmed *time.Time `validate:"optional"`

	// Gets set to true if creation process fails.
	CreationFailed *bool `validate:"optional" gorm:"default:false"`

	// Used to inform reconcilers that an object is being deleted so they may
	// complete delete reconciliation before actually deleting the object from the database.
	DeletionScheduled *time.Time `validate:"optional"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	DeletionAcknowledged *time.Time `validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	DeletionConfirmed *time.Time `validate:"optional"`

	// InterruptReconciliation is used by the controller to indicated that future
	// reconcilation should be interrupted.  Useful in cases where there is a
	// situation where future reconciliation could be descructive such as
	// spinning up more infrastructure when there is a unresolved problem.
	InterruptReconciliation *bool `validate:"optional" gorm:"default:false"`
}

// AlreadyReconciled reports whether Reconciled is set to true.
func (r Reconciliation) AlreadyReconciled() bool {
	// treat a nil pointer as not reconciled
	return r.Reconciled != nil && *r.Reconciled
}
