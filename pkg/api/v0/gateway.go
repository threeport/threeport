//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// Gateway is a route for requests to a workload from clients outside the
// private network of a workload kubernetes runtime.  This
type GatewayDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// TCP Port to expose to outside network.
	TCPPort *int `json:"TCPPort,omitempty" query:"tcpport" gorm:"not null" validate:"required"`

	// // Expose port 443 with TLS termination.
	// HTTPSPort *bool `json:"HTTPSPort,omitempty" query:"httpsport" gorm:"default:true" validate:"optional"`

	// // Expose port 80.
	// HTTPPort *bool `json:"HTTPPort,omitempty" query:"httpport" gorm:"default:true" validate:"optional"`

	// Redirect all requests to HTTP port to HTTPS.
	HTTPSRedirect *bool `json:"HTTPSRedirect,omitempty" query:"httpsredirect" gorm:"default:true" validate:"optional"`

	// // Allow requests from the public internet.
	// Public *bool `json:"Public,omitempty" query:"public" gorm:"default:true" validate:"optional"`

	// // Allow requests from the private network outside the workload cluster but
	// // not from the public internet.
	// Private *bool `json:"Private,omitempty" query:"private" gorm:"default:false"
	// validate:"optional"`

	// Indicates if TLS is enabled.
	TLSEnabled *bool `json:"TLSEnabled,omitempty" query:"tlsenabled" gorm:"default:false" validate:"optional"`

	// The domain name to serve requests for.
	DomainNameDefinitionID *uint `json:"DomainNameDefinitionID,omitempty" query:"domainnamedefinition" validate:"optional"`

	// The request paths to serve requests for.
	Path *string `json:"Paths,omitempty" query:"paths" gorm:"default:'/'" validate:"optional"`

	// An optional subdomain to add to the domain name.
	SubDomain *string `json:"SubDomain,omitempty" query:"subdomain" validate:"optional"`

	// The kubernetes service to route requests to.
	ServiceName *string `json:"ServiceName,omitempty" query:"servicename" validate:"optional"`

	// The workload definition that belongs to this resource.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" validate:"optional"`

	// The associated gateway instances that are deployed from this definition.
	GatewayInstances []*GatewayInstance `json:"GatewayInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// GatewayInstance is a deployed instance of a gateway.
type GatewayInstance struct {
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
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadresourceinstanceid" gorm:"not null" validate:"required"`

	//TODO: implement this in the future so we don't need to
	// query the workload instance & search for the workload resource instance
	// The workload resource instances that belong to this instance.
	// WorkloadResourceInstances *[]WorkloadResourceInstance `json:"WorkloadResourceInstances,omitempty" query:"workloadresourceinstances" validate:"optional,association"`
}

// DomainNameDefinition the definition for domain name management for a
// particular DNS zone.
type DomainNameDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The base domain upon which the subdomain will be added to give a workload
	// a unique domain name.
	Domain *string `json:"Domain,omitempty" query:"domain" gorm:"not null" validate:"required"`

	// The name of the zone in which the domain is managed.
	Zone *string `json:"Zone,omitempty" query:"zone" gorm:"not null" validate:"required"`

	// The email address of the domain administrator.
	AdminEmail *string `json:"AdminEmail,omitempty" query:"adminemail" gorm:"not null" validate:"required"`

	// Whether or not the domain name is a root domain.
	// RootDomain *bool `json:"RootDomain,omitempty" query:"rootdomain" gorm:"default:false" validate:"optional"`

	// TTL configuration for this record.
	// TTL *uint `json:"TTL,omitempty" query:"ttl" gorm:"default:300" validate:"optional"`

	// The type of DNS record to create.
	// Type *string `json:"Type,omitempty" query:"type" gorm:"default:'A'"
	// validate:"optional"`

	// // The workload definition that belongs to this resource.
	// WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" validate:"optional"`

	// The associated domain name instances that are deployed from this definition.
	DomainNameInstances []*DomainNameInstance `json:"DomainNameInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// DomainNameInstance is an instance of domain name management for a workload.
type DomainNameInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The definition used to define the instance.
	DomainNameDefinitionID *uint `json:"DomainNameDefinitionID,omitempty" query:"domainnamedefinitionid" gorm:"not null" validate:"required"`

	// The workload instance this gateway belongs to.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadresourceinstanceid" gorm:"not null" validate:"required"`

	// The cluster where the workload that is using the domain name is running.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`
}
