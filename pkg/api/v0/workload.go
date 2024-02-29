//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-sdk codegen controller --filename $GOFILE
package v0

import (
	"time"

	"gorm.io/datatypes"
)

const (
	PathWorkloadResourceDefinitionSets = "/v0/workload-resource-definition-sets"
)

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl:config-path
// WorkloadDefinition is the collection of Kubernetes manifests that define a
// distinct workload.
type WorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The yaml manifests that define the workload configuration.
	YAMLDocument *string `json:"YAMLDocument,omitempty" gorm:"not null" validate:"required"`

	// The associated workload resource definitions that are derived.
	WorkloadResourceDefinitions []*WorkloadResourceDefinition `json:"WorkloadResourceDefinitions,omitempty" validate:"optional,association"`

	// The associated workload instances that are deployed from this definition.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstances,omitempty" validate:"optional,association"`
}

// WorkloadResourceDefinition is an individual Kubernetes resource manifest.
type WorkloadResourceDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.
	JSONDefinition *datatypes.JSON `json:"JSONDefinition,omitempty" gorm:"not null" validate:"required"`

	// The workload definition this resource belongs to.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`
}

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl
// WorkloadInstance is a deployed instance of a workload.
type WorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

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

	// The threeport objects that are deployed to support the workload instance.
	AttachedObjectReferences []*AttachedObjectReference `json:"AttachedObjectReferences,omitempty" query:"attachedobjectreferences" validate:"optional,association"`
}

// AttachedObjectReference is a reference to an attached object.
type AttachedObjectReference struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	ObjectID *uint   `json:"ObjectID,omitempty" query:"objectid" gorm:"not null" validate:"optional"`
	Type     *string `json:"Type,omitempty" query:"type" gorm:"not null" validate:"optional"`

	// The workload definition this resource belongs to.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" gorm:"not null" validate:"required"`
}

// WorkloadResourceInstance is a Kubernetes resource instance.
type WorkloadResourceInstance struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

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

	// The related workload instance.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" validate:"optional"`

	// The related workload resource instance.
	WorkloadResourceInstanceID *uint `json:"WorkloadResourceInstanceID,omitempty" query:"workloadresourceinstanceid" validate:"optional"`

	// The related helm workload instance.
	HelmWorkloadInstanceID *uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" validate:"optional"`
}
