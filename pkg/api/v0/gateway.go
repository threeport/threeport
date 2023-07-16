//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// Gateway is a route for requests to a workload from clients outside the
// private network of a workload cluster.  This
type GatewayDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// TCP Port to expose to outside network.
	TCPPort *int `json:"TCPPort,omitempty" query:"tcpport" validate:"optional"`

	// Expose port 443 with TLS termination.
	HTTPSPort *bool `json:"HTTPSPort,omitempty" query:"httpsport" gorm:"default:true" validate:"optional"`

	// Expose port 80.
	HTTPPort *bool `json:"HTTPPort,omitempty" query:"httpport" gorm:"default:true" validate:"optional"`

	// Redirect all requests to HTTP port to HTTPS.
	HTTPSRedirct *bool `json:"HTTPSRedirect,omitempty" query:"httpsredirect" gorm:"default:true" validate:"optional"`

	// Allow requests from the public internet.
	Public *bool `json:"Public,omitempty" query:"public" gorm:"default:true" validate:"optional"`

	// Allow requests from the private network outside the workload cluster but
	// not from the public internet.
	Private *bool `json:"Private,omitempty" query:"private" gorm:"default:false" validate:"optional"`

	// The domain name to serve requests for.
	DomainNameID *uint `json:"DomainNameID,omitempty" query:"domainname" validate:"optional"`

	// The workload definition that belongs to this resource.
	WorkloadDefinitionID *uint `json:"WorkloadDefinitionID,omitempty" query:"workloaddefinitionid" gorm:"constraint:OnDelete:CASCADE;omitempty" validate:"optional"`

	// The associated gateway instances that are deployed from this definition.
	GatewayInstances []*GatewayInstance `json:"GatewayInstances,omitempty" validate:"optional,association"`

	// Indicates if object is considered to be reconciled by gateway controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}

// +threeport-codegen:reconciler
// GatewayInstance is a deployed instance of a gateway.
type GatewayInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The cluster where the ingress layer is installed.
	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" query:"clusterinstanceid" gorm:"not null" validate:"required"`

	// GatewayDefinitionID is the definition used to configure the workload instance.
	GatewayDefinitionID *uint `json:"GatewayDefinitionID,omitempty" query:"gatewaydefinitionid" gorm:"not null" validate:"required"`

	// The workload instance this gateway belongs to.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadresourceinstanceid" gorm:"not null" validate:"optional"`

	// The workload resource instance that belongs to this instance.
	WorkloadResourceInstanceID *uint `json:"WorkloadResourceInstanceID,omitempty" query:"workloadresourceinstanceid" validate:"optional"`

	// Indicates if object is considered to be reconciled by gateway controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}
