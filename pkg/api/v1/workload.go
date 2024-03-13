//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
package v1

import (
	v0 "github.com/threeport/threeport/pkg/api/v0"
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
	WorkloadResourceInstances []*v0.WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" validate:"optional,association"`

	// The latest status of a workload instance.
	Status *string `json:"Status,omitempty" query:"status" validate:"optional"`

	// All events generated for the workload instance that aren't related to a
	// particular workload resource instance.
	Events []*v0.WorkloadEvent `json:"Events,omitempty" query:"events" validate:"optional"`
}
