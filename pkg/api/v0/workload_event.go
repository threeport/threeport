package v0

import (
	"time"

	"gorm.io/gorm"
)

// WorkloadEvent is a summary of a Kubernetes Event that is associated with a
// kubernetes workload resource instance or a helm workload instance.
type WorkloadEvent struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// A unique ID for de-duplicating purposes.  It is one of two thing:
	// * The Kubernetes Event resource UID: when the WorkloadEvent is derived
	// directly from a Kubernetes Event.
	// * The workload controller ID: when the WorkloadEvent is emitted by the
	// workload controller.
	RuntimeEventUID *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The type of event that occurred in Kubernetes.
	Type *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The reason for the event.
	Reason *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The message associated with the event.
	Message *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The timestamp for the event in the kubernetes runtime.
	Timestamp *time.Time `json:",omitempty" validate:"required" gorm:"not null"`

	// The related kubernetes workload instance.
	KubernetesWorkloadInstanceID *uint `json:",omitempty" validate:"optional"`

	// The related kubernetes workload resource instance.
	KubernetesWorkloadResourceInstanceID *uint `json:",omitempty" validate:"optional"`

	// The related helm workload instance.
	HelmWorkloadInstanceID *uint `json:",omitempty" validate:"optional"`

	// The related machine workload instance.
	MachineWorkloadInstanceID *uint `json:",omitempty" validate:"optional"`
}

// beforeCreate is the pre-create validation hook for WorkloadEvent.
func (w *WorkloadEvent) beforeCreate(tx *gorm.DB) error {
	return nil
}

// beforeUpdate is the pre-update validation hook for WorkloadEvent.
func (w *WorkloadEvent) beforeUpdate(tx *gorm.DB) error {
	return nil
}

// beforeDelete is the pre-delete validation hook for WorkloadEvent.
func (w *WorkloadEvent) beforeDelete(tx *gorm.DB) error {
	return nil
}

// afterCreate is the post-create hook for WorkloadEvent.
func (w *WorkloadEvent) afterCreate(tx *gorm.DB) error {
	return nil
}

// afterUpdate is the post-update hook for WorkloadEvent.
func (w *WorkloadEvent) afterUpdate(tx *gorm.DB) error {
	return nil
}

// afterDelete is the post-delete hook for WorkloadEvent.
func (w *WorkloadEvent) afterDelete(tx *gorm.DB) error {
	return nil
}
