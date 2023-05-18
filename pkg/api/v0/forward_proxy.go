//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

// ForwardProxy provides a managed outbound network connection from a workload
// to a destination.  It can be used to manage service dependencies
// transparently for a workload so the workload doesn't require any config
// change or config reload in order to switch between service dependencies.
type ForwardProxyDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The hostname of the upstream service.
	UpstreamHost *string `json:"UpstreamHost,omitempty" query:"upstreamhost" gorm:"not null" validate:"required"`

	// The path for the upstream service.
	UpstreamPath *string `json:"UpstreamPath,omitempty" query:"upstreampath" gorm:"not null" validate:"required"`
}

type ForwardProxyInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	// The definition used to define the instance.
	ForwardProxyDefinitionID *uint `json:"ForwardProxyDefinitionID,omitempty" validate:"optional,association"`

	// The cluster where the forward proxy is deployed.
	ClusterInstanceID *uint `json:"ClusterInstanceID,omitempty" validate:"optional,association"`
}
