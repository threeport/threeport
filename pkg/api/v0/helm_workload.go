//go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-sdk codegen controller --filename $GOFILE
package v0

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl:config-path
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

	// The version of the helm chart to use from the helm repo, e.g. 1.2.3
	HelmChartVersion *string `json:"HelmChartVersion,omitempty" query:"helmchartversion" gorm:"not null" validate:"optional"`

	// The helm values that override the defaults from the helm chart.  These
	// will be inherited by each helm workload instance derived from this
	// definition.  The helm values defined here can be further overridden by
	// values defined on the helm workload instance.
	HelmValuesDocument *string `json:"HelmValuesDocument,omitempty" query:"helmvaluesdocument" validate:"optional"`

	// The associated helm workload instances that are deployed from this definition.
	HelmWorkloadInstances []*HelmWorkloadInstance `json:"HelmWorkloadInstances,omitempty" validate:"optional,association"`
}

// +threeport-sdk:reconciler
// +threeport-sdk:tptctl:config-path
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

	// Filepath to the helm values YAML file that provides runtime parameters to the helm chart.
	HelmValuesDocument *string `json:"HelmValuesDocument,omitempty" query:"helmvaluesdocument" validate:"optional"`

	// Namespace to deploy the helm chart to.
	HelmReleaseNamespace *string `json:"HelmReleaseNamespace,omitempty" query:"helmreleasenamespace" validate:"optional"`

	// The definition used to configure the workload instance.
	HelmWorkloadDefinitionID *uint `json:"HelmWorkloadDefinitionID,omitempty" query:"helmworkloaddefinitionid" gorm:"not null" validate:"required"`
}
