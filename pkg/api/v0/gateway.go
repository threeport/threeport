package v0

// GatewayDefinition is the definition of a gateway that provides routing of requests to a
// collection of workloads.
type GatewayDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// // Allow requests from the public internet.
	// Public *bool `json:",omitempty" gorm:"default:true" validate:"optional"`

	// // Allow requests from the private network outside the workload cluster but
	// // not from the public internet.
	// Private *bool `json:",omitempty" gorm:"default:false"
	// validate:"optional"`

	// HttpPorts is a list of HTTP ports to expose to the outside network.
	HttpPorts []*GatewayHttpPort `json:",omitempty" validate:"optional"`

	// TcpPorts is a list of TCP ports to expose to the outside network.
	TcpPorts []*GatewayTcpPort `json:",omitempty" validate:"optional"`

	// The domain name to serve requests for.
	DomainNameDefinitionID *uint `json:",omitempty" validate:"optional"`

	// An optional subdomain to add to the domain name.
	SubDomain *string `json:",omitempty" validate:"optional"`

	// The kubernetes service to route requests to.
	ServiceName *string `json:",omitempty" validate:"optional"`

	// The kubernetes workload definition that belongs to this resource.
	KubernetesWorkloadDefinitionID *uint `json:",omitempty" validate:"optional"`

	// The associated gateway instances that are deployed from this definition.
	GatewayInstances []*GatewayInstance `json:",omitempty" validate:"optional,association"`
}

// GatewayInstance is a deployed instance of a gateway.
type GatewayInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime where the ingress layer is installed.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The domain name instance to serve requests for.
	// DomainNameInstanceID *uint `json:",omitempty" validate:"optional"`

	// GatewayDefinitionID is the definition used to configure the gateway instance.
	GatewayDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// TODO: implement this in the future so we don't need to query the
	// kubernetes workload instance & search for the kubernetes workload resource
	// instance.
	// The kubernetes workload resource instances that belong to this instance.
	// KubernetesWorkloadResourceInstances *[]KubernetesWorkloadResourceInstance `validate:"optional,association"`

	// The kubernetes workload instance this gateway belongs to.
	KubernetesWorkloadInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`
}

// GatewayHttpPort is an HTTP port to expose to the outside network.
type GatewayHttpPort struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// GatewayDefinitionID is the definition used to configure the gateway http port.
	GatewayDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null"`

	// The HTTP port to expose.
	Port *int `json:",omitempty" validate:"required" gorm:"not null"`

	// The request path to serve requests for.
	Path *string `json:",omitempty" validate:"optional" gorm:"default:'/'"`

	// Indicates if TLS is enabled.
	TLSEnabled *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Redirect all requests to HTTP port to HTTPS.
	HTTPSRedirect *bool `json:",omitempty" validate:"optional" gorm:"default:false"`
}

// GatewayTcpPort is a TCP port to expose to the outside network.
type GatewayTcpPort struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// GatewayDefinitionID is the definition used to configure the gateway tcp port.
	GatewayDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null"`

	// The TCP port to expose.
	Port *int `json:",omitempty" validate:"required" gorm:"not null"`

	// Indicates if TLS is enabled.
	TLSEnabled *bool `json:",omitempty" validate:"optional" gorm:"default:false"`
}

// DomainNameDefinition is the definition for domain name management for a
// particular DNS zone.
type DomainNameDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The base domain upon which the subdomain will be added to give a workload
	// a unique domain name.
	Domain *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The name of the zone in which the domain is managed.
	Zone *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The email address of the domain administrator.
	AdminEmail *string `json:",omitempty" validate:"required" gorm:"not null"`

	// Whether or not the domain name is a root domain.
	// RootDomain *bool `json:",omitempty" gorm:"default:false" validate:"optional"`

	// TTL configuration for this record.
	// TTL *uint `json:",omitempty" gorm:"default:300" validate:"optional"`

	// The type of DNS record to create.
	// Type *string `json:",omitempty" gorm:"default:'A'"
	// validate:"optional"`

	// // The kubernetes workload definition that belongs to this resource.
	// KubernetesWorkloadDefinitionID *uint `json:",omitempty" validate:"optional"`

	// The associated domain name instances that are deployed from this definition.
	DomainNameInstances []*DomainNameInstance `json:",omitempty" validate:"optional,association"`
}

// DomainNameInstance is an instance of domain name management for a workload.
type DomainNameInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The definition used to define the instance.
	DomainNameDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The kubernetes workload instance this domain name belongs to.
	KubernetesWorkloadInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The cluster where the kubernetes workload that is using the domain name is running.
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`
}
