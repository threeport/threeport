//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
package v1

import (
	"time"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"gorm.io/datatypes"
)

// +threeport-sdk:reconciler
// WorkloadInstance is a deployed instance of a workload.
type WorkloadInstance struct {
	v0.Common         `swaggerignore:"true" mapstructure:",squash"`
	v0.Instance       `mapstructure:",squash"`
	v0.Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The definition used to configure the workload instance.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`

	// The associated workload resource definitions that are derived.
	WorkloadResourceInstances []*WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" validate:"optional,association"`

	// The latest status of a workload instance.
	Status *string `json:"Status,omitempty" query:"status" validate:"optional"`

	// All events generated for the workload instance that aren't related to a
	// particular workload resource instance.
	Events []*WorkloadEvent `json:"Events,omitempty" query:"events" validate:"optional"`
}

// WorkloadResourceInstance is a Kubernetes resource instance.
type WorkloadResourceInstance struct {
	v0.Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.  This field is a superset of
	// WorkloadResourceDefinition.JSONDefinition in that it has namespace
	// management and other configuration - such as resource allocation
	// management - added.
	JSONDefinition *datatypes.JSON `json:"JSONDefinition,omitempty" gorm:"not null" validate:"required"`

	// The workload definition this resource belongs to.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" gorm:"not null" validate:"required"`

	// The most recent operation performed on a Kubernete resource in the
	// kubernetes runtime.
	LastOperation *string `json:"LastOperation,omitempty" query:"lastoperation" validate:"optional"`

	// Indicates if object is considered to be reconciled by workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`

	// The JSON definition of a Kubernetes resource as stored in etcd in the
	// kubernetes runtime.
	RuntimeDefinition *datatypes.JSON `json:"RuntimeDefinition,omitempty" query:"runtimedefinition" validate:"optional"`

	// All events that have occured related to this object.
	Events []*WorkloadEvent `json:"Events,omitempty" query:"events" validate:"optional"`

	// Whether another controller has scheduled this resource for deletion
	ScheduledForDeletion *time.Time `json:"ScheduledForDeletion,omitempty" query:"scheduledfordeletion" validate:"optional"`
}

// WorkloadEvent is a summary of a Kubernetes Event that is associated with a
// WorkloadResourceInstance.
type WorkloadEvent struct {
	v0.Common `swaggerignore:"true" mapstructure:",squash"`

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

	// The related workload instance.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" validate:"optional"`

	// The related workload resource instance.
	WorkloadResourceInstanceID *uint `json:"WorkloadResourceInstanceID,omitempty" query:"workloadresourceinstanceid" validate:"optional"`

	// The related helm workload instance.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional"`
}
