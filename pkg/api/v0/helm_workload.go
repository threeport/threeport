//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

import "gorm.io/datatypes"

// +threeport-codegen:reconciler
// HelmWorkloadDefinition includes the helm repo and chart that is used to
// configure the workload.
type HelmWorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The helm repo URL to pull the helm workload's chart from
	// e.g. oci://registry-1.docker.io/bitnamicharts
	// e.g. https://grafana.github.io/helm-charts
	Repo *string `json:"Repo,omitempty" query:"repo" gorm:"not null" validate:"required"`

	// The name of the helm chart to use from the helm repo, e.g. wordpress
	Chart *string `json:"Chart,omitempty" query:"chart" gorm:"not null" validate:"required"`

	// The version of the helm chart to use from the helm repo, e.g. 1.2.3
	ChartVersion *string `json:"ChartVersion,omitempty" query:"chartversion" validate:"optional"`

	// The helm values that override the defaults from the helm chart.  These
	// will be inherited by each helm workload instance derived from this
	// definition.  The helm values defined here can be further overridden by
	// values defined on the helm workload instance.
	ValuesDocument *string `json:"ValuesDocument,omitempty" validate:"optional"`

	// The associated helm workload instances that are deployed from this definition.
	HelmWorkloadInstances []*HelmWorkloadInstance `json:"HelmWorkloadInstances,omitempty" validate:"optional,association"`

	// Complete kubernetes resources that will be appended to the provided
	// helm chart.
	AdditionalResources *datatypes.JSON `json:"AdditionalResources,omitempty" validate:"optional"`
}

// +threeport-codegen:reconciler
// HelmWorkloadInstance is a deployed instance of a helm chart with the runtime
// parameters as helm values.
type HelmWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Filepath to the helm values YAML file that provides runtime parameters to
	// the helm chart.
	ValuesDocument *string `json:"ValuesDocument,omitempty" validate:"optional"`

	// The kubernetes runtime to which the helm workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// Namespace to deploy the helm chart to.
	ReleaseNamespace *string `json:"ReleaseNamespace,omitempty" query:"releasenamespace" validate:"optional"`

	// The definition used to configure the workload instance.
	HelmWorkloadDefinitionID *uint `json:"HelmWorkloadDefinitionID,omitempty" query:"helmworkloaddefinitionid" gorm:"not null" validate:"required"`

	// Complete kubernetes resources that will be appended to the provided
	// helm chart.
	AdditionalResources *datatypes.JSON `json:"AdditionalResources,omitempty" validate:"optional"`
}
