package v0

import "gorm.io/datatypes"

// HelmWorkloadDefinition includes the helm repo and chart that is used to
// configure the workload.
type HelmWorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The helm repo URL to pull the helm workload's chart from
	// e.g. oci://registry-1.docker.io/bitnamicharts
	// e.g. https://grafana.github.io/helm-charts
	Repo *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The name of the helm chart to use from the helm repo, e.g. wordpress
	Chart *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The version of the helm chart to use from the helm repo, e.g. 1.2.3
	ChartVersion *string `json:",omitempty" validate:"optional"`

	// The helm values that override the defaults from the helm chart.  These
	// will be inherited by each helm workload instance derived from this
	// definition.  The helm values defined here can be further overridden by
	// values defined on the helm workload instance.
	ValuesDocument *string `json:",omitempty" validate:"optional"`

	// The associated helm workload instances that are deployed from this definition.
	HelmWorkloadInstances []*HelmWorkloadInstance `json:",omitempty" validate:"optional,association"`

	// Complete kubernetes resources that will be appended to the provided
	// helm chart.
	AdditionalResources *datatypes.JSONSlice[datatypes.JSON] `json:",omitempty" validate:"optional"`
}

// HelmWorkloadInstance is a deployed instance of a helm chart with the runtime
// parameters as helm values.
type HelmWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Filepath to the helm values YAML file that provides runtime parameters to
	// the helm chart.
	ValuesDocument *string `json:",omitempty" validate:"optional"`

	// The kubernetes runtime to which the helm workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// Namespace to deploy the helm chart to.
	ReleaseNamespace *string `json:",omitempty" validate:"optional"`

	// The definition used to configure the kubernetes workload instance.
	HelmWorkloadDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// Complete kubernetes resources that will be appended to the provided
	// helm chart.
	AdditionalResources *datatypes.JSONSlice[datatypes.JSON] `json:",omitempty" validate:"optional"`
}
