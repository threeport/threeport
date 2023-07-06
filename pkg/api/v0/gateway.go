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
	TCPPort *int32 `json:"TCPPort,omitempty" query:"tcpport" validate:"optional"`

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

	// Indicates if object is considered to be reconciled by workload controller.
	Reconciled *bool `json:"Reconciled,omitempty" query:"reconciled" gorm:"default:false" validate:"optional"`
}

type GatewayInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition used to define the instance.
	GatewayDefinitionID *uint `json:"GatewayDefinitionID,omitempty" validate:"optional,association"`

	// The cluster where the ingress layer is installed.
	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" validate:"optional,association"`
}
