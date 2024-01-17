//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// HelmWorkloadDefinition includes the helm repo and chart that is used to
// configure the workload.
type HelmWorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The OCI helm repo URL to pull the helm workload's chart from, e.g.
	// oci://registry-1.docker.io/bitnamicharts
	HelmRepo *string `json:"HelmRepo,omitempty" query:"helmrepo" gorm:"not null" validate:"required"`

	// The name of the helm chart to use from the helm reop, e.g. wordpress
	HelmChart *string `json:"HelmChart,omitempty" query:"helmchart" gorm:"not null" validate:"required"`

	// The helm values that override the defaults from the helm chart.  These
	// will be inherited by each helm workload instance derived from this
	// definition.  The helm values defined here can be further overridden by
	// values defined on the helm workload instance.
	HelmValuesDocument *string `json:"HelmValuesDocument,omitempty" query:"helmvaluesdocument" validate:"optional"`

	// The associated helm workload instances that are deployed from this definition.
	HelmWorkloadInstances []*HelmWorkloadInstance `json:"HelmWorkloadInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// HelmWorkloadInstance is a deployed instance of a helm chart with the runtime
// parameters as helm values.
type HelmWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// Filepath to the helm values YAML file that provides runtime parameters to the helm chart.
	HelmValuesDocument *string `json:"HelmValuesDocument,omitempty" query:"helmvaluesdocument" validate:"optional"`

	// The definition used to configure the workload instance.
	HelmWorkloadDefinitionID *uint `json:"HelmWorkloadDefinitionID,omitempty" query:"helmworkloaddefinitionid" gorm:"not null" validate:"required"`

	//// The associated workload resource definitions that are derived.
	//WorkloadResourceInstances []*WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" validate:"optional,association"`

	//// The latest status of a workload instance.
	//Status *string `json:"Status,omitempty" query:"status" validate:"optional"`

	//// All events generated for the workload instance that aren't related to a
	//// particular workload resource instance.
	//Events []*WorkloadEvent `json:"Events,omitempty" query:"events" validate:"optional"`

	//// The threeport objects that are deployed to support the workload instance.
	//AttachedObjectReferences []*AttachedObjectReference `json:"AttachedObjectReferences,omitempty" query:"attachedobjectreferences" validate:"optional,association"`
}
