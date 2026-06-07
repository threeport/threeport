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
	YAMLDocument *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The associated kubernetes workload resource definitions that are derived.
	KubernetesWorkloadResourceDefinitions []*KubernetesWorkloadResourceDefinition `json:",omitempty" validate:"optional,association"`

	// The associated kubernetes workload instances that are deployed from this definition.
	KubernetesWorkloadInstances []*KubernetesWorkloadInstance `json:",omitempty" validate:"optional,association"`
}

// KubernetesWorkloadResourceDefinition is an individual Kubernetes resource manifest.
type KubernetesWorkloadResourceDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.
	JSONDefinition *datatypes.JSON `json:",omitempty" validate:"required" gorm:"not null"`

	// The kubernetes workload definition this resource belongs to.
	KubernetesWorkloadDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null"`
}

// KubernetesWorkloadInstance is a deployed instance of a kubernetes workload.
type KubernetesWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The definition used to configure the kubernetes workload instance.
	KubernetesWorkloadDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The associated kubernetes workload resource instances that are derived.
	KubernetesWorkloadResourceInstances []*KubernetesWorkloadResourceInstance `json:",omitempty" validate:"optional,association"`

	// The latest status of a kubernetes workload instance.
	Status *string `json:",omitempty" validate:"optional"`
}

// KubernetesWorkloadResourceInstance is a Kubernetes resource instance.
type KubernetesWorkloadResourceInstance struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.  This field is a superset of
	// KubernetesWorkloadResourceDefinition.JSONDefinition in that it has
	// namespace management and other configuration — such as resource
	// allocation management — added.
	JSONDefinition *datatypes.JSON `json:",omitempty" validate:"required" gorm:"not null"`

	// The kubernetes workload instance this resource belongs to.
	KubernetesWorkloadInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null"`

	// The most recent operation performed on a Kubernetes resource in the
	// kubernetes runtime.
	LastOperation *string `json:",omitempty" validate:"optional"`

	// Indicates if object is considered to be reconciled by kubernetes
	// kubernetes workload controller.
	Reconciled *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// The JSON definition of a Kubernetes resource as stored in etcd in the
	// kubernetes runtime.
	RuntimeDefinition *datatypes.JSON `json:",omitempty" validate:"optional"`

	// Whether another controller has scheduled this resource for deletion
	ScheduledForDeletion *time.Time `json:",omitempty" validate:"optional"`
}
