// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

import (
	"time"

	"gorm.io/gorm"
)

// Common includes standard fields included in most objects.
type Common struct {
	ID        *uint           `json:"ID,omitempty" gorm:"primarykey"`
	CreatedAt *time.Time      `json:"CreatedAt,omitempty"`
	UpdatedAt *time.Time      `json:"UpdatedAt,omitempty"`
	DeletedAt *gorm.DeletedAt `json:"DeletedAt,omitempty" gorm:"index"`
}

// Reconciliation includes the fields for reconciled objects.
type Reconciliation struct {
	// Indicates if object is considered to be reconciled by the object's controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`

	// Used to inform reconcilers that an object is being deleted so they may
	// complete delete reconciliation before actually deleting the object from the database.
	DeletionScheduled *time.Time `json:"DeletionScheduled,omitempty" query:"deletionscheduled" validate:"optional"`

	// Used by controllers to acknowledge deletion and indicate that deletion
	// reconciliation has begun so that subsequent reconciliation attempts can
	// act accordingly.
	DeletionAcknowledged *time.Time `json:"DeletionAcknowledged,omitempty" query:"deletionacknowledged" validate:"optional"`

	// Used by controllers to confirm deletion of an object.
	DeletionConfirmed *time.Time `json:"DeletionConfirmed,omitempty" query:"deletionconfirmed" validate:"optional"`

	// InterruptReconciliation is used by the controller to indicated that future
	// reconcilation should be interrupted.  Useful in cases where there is a
	// situation where future reconciliation could be descructive such as
	// spinning up more infrastructure when there is a unresolved problem.
	InterruptReconciliation *bool `json:"InterruptReconciliation,omitempty" query:"interruptreconciliation" gorm:"default:false" validate:"optional"`
}
