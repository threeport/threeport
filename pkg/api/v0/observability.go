//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// MetricsDefinition defines a metrics aggregation layer for a workload.
type MetricsDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The domain name to serve requests for.
	DomainNameDefinitionID *uint `json:"DomainNameDefinitionID,omitempty" query:"domainnamedefinition" validate:"optional"`

	// An optional subdomain to add to the domain name.
	SubDomain *string `json:"SubDomain,omitempty" query:"subdomain" validate:"optional" gorm:"default:'metrics'"`

	// The Helm workload definitions that belongs to this resource.
	HelmWorkloadDefinitionIds []*uint `json:"HelmWorkloadDefinitionId,omitempty" query:"helmworkloaddefinitionid" validate:"optional"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying charts.
	HelmWorkloadDefinitionValues *string `json:"HelmWorkloadDefinitionValues,omitempty" query:"helmworkloaddefinitionvalues" validate:"optional"`

	// The associated gateway instance that are deployed from this definition.
	GatewayInstance *GatewayInstance `json:"GatewayInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// MetricsInstances defines an instance of a metrics aggregation layer for a workload.
type MetricsInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The domain name instance to serve requests for.
	// DomainNameInstanceID *uint `json:"DomainNameInstanceID,omitempty" query:"domainnameinstanceid" validate:"optional"`

	// GatewayDefinitionID is the definition used to configure the workload instance.
	GatewayDefinitionID *uint `json:"GatewayDefinitionID,omitempty" query:"gatewaydefinitionid" gorm:"not null" validate:"required"`

	// The workload instance this gateway belongs to.
	HelmWorkloadInstanceIDs []*uint `json:"HelmWorkloadInstanceID,omitempty" query:"helmworkloadinstanceid" gorm:"not null" validate:"required"`

	// Optional Helm workload definition values that can be provided to configure the
	// underlying charts.
	HelmWorkloadInstanceValues *string `json:"HelmWorkloadInstanceValues,omitempty" query:"helmworkloadinstancevalues" validate:"optional"`

	//TODO: implement this in the future so we don't need to
	// query the workload instance & search for the workload resource instance
	// The workload resource instances that belong to this instance.
	// WorkloadResourceInstances *[]WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" query:"workloadresourceinstances" validate:"optional,association"`
}
