//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

// DomainNameDefinition the definition for domain name management for a
// particular DNS zone.
type DomainNameDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	//// The name of the definition.
	//Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The base domain upon which the subdomain will be added to give a workload
	// a unique domain name.
	Domain *string `json:"Domain,omitempty" query:"domain" gorm:"not null" validate:"required"`

	// The name of the zone in which the domain is managed.
	Zone *string `json:"Zone,omitempty" query:"zone" gorm:"not null" validate:"required"`
}

// DomainNameInstance is an instance of domain name management for a workload.
type DomainNameInstance struct {
	Common   `swaggerignore:"true" mapstructure:",squash"`
	Instance `mapstructure:",squash"`

	//// The subdomain added to the domain to provide a unique domain name.
	//Subdomain *string `json:"Subdomain,omitempty" query:"subdomain" gorm:"not null" validate:"required"`

	//// The name to use as a subdomain pre-pended to the domain in the definition.
	//Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The definition used to define the instance.
	DomainNameDefinitionID *uint `json:"DomainNameDefinitionID,omitempty" query:"domainnamedefinitionid" gorm:"not null" validate:"required"`

	// The cluster where the workload that is using the domain name is running.
	ClusterInstanceID *uint `json:"ClusterInstnaceID,omitempty" query:"clusterinstanceid" gorm:"not null" validate:"required"`
}
