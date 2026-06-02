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
	RuntimeEventUID *string `json:"RuntimeEventUID,omitempty" query:"runtimeeventuid" gorm:"not null" validate:"required"`

	// The type of event that occurred in Kubernetes.
	Type *string `json:"Type,omitempty" query:"type" gorm:"not null" validate:"required"`

	// The reason for the event.
	Reason *string `json:"Reason,omitempty" query:"reason" gorm:"not null" validate:"required"`

	// The message associated with the event.
	Message *string `json:"Message,omitempty" query:"message" gorm:"not null" validate:"required"`

	// The timestamp for the event in the kubernetes runtime.
	Timestamp *time.Time `json:"Timestamp,omitempty" query:"timestamp" gorm:"not null" validate:"required"`

	// The related kubernetes workload instance.
	KubernetesWorkloadInstanceID *uint `json:"KubernetesWorkloadInstanceID,omitempty" query:"kubernetesworkloadinstanceid" validate:"optional"`

	// The related kubernetes workload resource instance.
	KubernetesWorkloadResourceInstanceID *uint `json:"KubernetesWorkloadResourceInstanceID,omitempty" query:"kubernetesworkloadresourceinstanceid" validate:"optional"`

	// The related helm workload instance.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional"`

	// The related machine workload instance.
	MachineWorkloadInstanceID *uint `json:"MachineWorkloadInstanceID,omitempty" query:"machineworkloadinstanceid" validate:"optional"`
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
