//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

import (
	pq "github.com/lib/pq"
)

// +threeport-codegen:reconciler
// MetricsDefinition defines a metrics aggregation layer for a workload.
type MetricsDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The Helm workload definitions that belongs to this resource.
	HelmWorkloadDefinitionIDs pq.Int64Array `json:"HelmWorkloadDefinitionIDs,omitempty" query:"helmworkloaddefinitionid" validate:"optional" gorm:"type:integer[]"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying charts.
	HelmWorkloadDefinitionValues *string `json:"HelmWorkloadDefinitionValues,omitempty" query:"helmworkloaddefinitionvalues" validate:"optional"`
}

// +threeport-codegen:reconciler
// MetricsInstances defines an instance of a metrics aggregation layer for a workload.
type MetricsInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The helm workload instance ids this belongs to.
	HelmWorkloadInstanceIDs pq.Int64Array `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceids" gorm:"type:integer[]" validate:"required"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying charts.
	HelmWorkloadInstanceValues *string `json:"HelmWorkloadInstanceValues,omitempty" query:"helmworkloadinstancevalues" validate:"optional"`
}
