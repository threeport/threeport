//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
package v0

import (
	"gorm.io/datatypes"
)

const PathWorkloadResourceDefinitionSets = "/v0/workload-resource-definition-sets"

// WorkloadDefinition is the collection of Kubernetes manifests that define a
// distinct workload.
// +threeport-codegen:reconciler
type WorkloadDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The yaml manifests that define the workload configuration.
	YAMLDocument *string `json:"YAMLDocument,omitempty" gorm:"not null" validate:"required"`

	// The associated workload resource definitions that are derived.
	WorkloadResourceDefinitions []*WorkloadResourceDefinition `json:"WorkloadResourceDefinitions,omitempty" validate:"optional,association"`

	// The associated workload instances that are deployed from this definition.
	WorkloadInstances []*WorkloadInstance `json:"WorkloadInstances,omitempty" validate:"optional,association"`

	// Indicates if object is considered to be reconciled by workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}

// WorkloadResourceDefinition is an individual Kubernetes resource manifest.
type WorkloadResourceDefinition struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The individual manifest in JSON format.
	JSONDefinition *datatypes.JSON `json:"JSONDefinition,omitempty" gorm:"not null" validate:"required"`

	// The workload definition this resource belongs to.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`
}

// +threeport-codegen:reconciler
// WorkloadInstance is a deployed instance of a workload.
type WorkloadInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`
	// ClusterID is the cluster to which the workload is deployed.
	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" query:"clusterinstanceid" gorm:"not null" validate:"required"`

	// WorkloadDefinitionID is the definition used to configure the workload
	// instance.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"not null" validate:"required"`

	// The associated workload resource definitions that are derived.
	WorkloadResourceInstances []*WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" validate:"optional,association"`

	// Indicates if object is considered to be reconciled by workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
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

	// The Kubernetes status of the deployed resource.
	// One of:
	// * Pending
	// * Running
	// * Succeeded
	// * Failed
	// * Unknown
	Status *string `json:"Status,omitempty" query:"status" validate:"optional"`
}
