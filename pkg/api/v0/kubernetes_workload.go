package v0

import (
	"time"

	"gorm.io/datatypes"
)

const (
	PathKubernetesWorkloadResourceDefinitionSets = "/v0/kubernetes-workload-resource-definition-sets"
)

// KubernetesWorkloadDefinition is a collection of Kubernetes manifests that
// define a distinct workload.
type KubernetesWorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The yaml manifests that define the workload configuration.
	YAMLDocument *string `json:"YAMLDocument,omitempty" gorm:"not null" validate:"required"`

	// The associated kubernetes workload resource definitions that are derived.
	KubernetesWorkloadResourceDefinitions []*KubernetesWorkloadResourceDefinition `json:"KubernetesWorkloadResourceDefinitions,omitempty" validate:"optional,association"`

	// The associated kubernetes workload instances that are deployed from this definition.
	KubernetesWorkloadInstances []*KubernetesWorkloadInstance `json:"KubernetesWorkloadInstances,omitempty" validate:"optional,association"`
}

// KubernetesWorkloadResourceDefinition is an individual Kubernetes resource manifest.
type KubernetesWorkloadResourceDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.
	JSONDefinition *datatypes.JSON `json:"JSONDefinition,omitempty" gorm:"not null" validate:"required"`

	// The kubernetes workload definition this resource belongs to.
	KubernetesWorkloadDefinitionID *uint `json:"KubernetesWorkloadDefinitionID,omitempty" query:"kubernetesworkloaddefinitionid" gorm:"not null" validate:"required"`
}

// KubernetesWorkloadInstance is a deployed instance of a kubernetes workload.
type KubernetesWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required" relationship:"requires"`

	// The definition used to configure the kubernetes workload instance.
	KubernetesWorkloadDefinitionID *uint `json:"KubernetesWorkloadDefinitionID,omitempty" query:"kubernetesworkloaddefinitionid" gorm:"not null" validate:"required" relationship:"requires"`

	// The associated kubernetes workload resource instances that are derived.
	KubernetesWorkloadResourceInstances []*KubernetesWorkloadResourceInstance `json:"KubernetesWorkloadResourceInstances,omitempty" validate:"optional,association"`

	// The latest status of a kubernetes workload instance.
	Status *string `json:"Status,omitempty" validate:"optional"`
}

// KubernetesWorkloadResourceInstance is a Kubernetes resource instance.
type KubernetesWorkloadResourceInstance struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.  This field is a superset of
	// KubernetesWorkloadResourceDefinition.JSONDefinition in that it has
	// namespace management and other configuration — such as resource
	// allocation management — added.
	JSONDefinition *datatypes.JSON `json:"JSONDefinition,omitempty" gorm:"not null" validate:"required"`

	// The kubernetes workload instance this resource belongs to.
	KubernetesWorkloadInstanceID *uint `json:"KubernetesWorkloadInstanceID,omitempty" query:"kubernetesworkloadinstanceid" gorm:"not null" validate:"required"`

	// The most recent operation performed on a Kubernetes resource in the
	// kubernetes runtime.
	LastOperation *string `json:"LastOperation,omitempty" validate:"optional"`

	// Indicates if object is considered to be reconciled by kubernetes
	// kubernetes workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" gorm:"default:false" validate:"optional"`

	// The JSON definition of a Kubernetes resource as stored in etcd in the
	// kubernetes runtime.
	RuntimeDefinition *datatypes.JSON `json:"RuntimeDefinition,omitempty" validate:"optional"`

	// Whether another controller has scheduled this resource for deletion
	ScheduledForDeletion *time.Time `json:"ScheduledForDeletion,omitempty" validate:"optional"`
}
