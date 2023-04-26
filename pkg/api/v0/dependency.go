//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
package v0

type WorkloadDependency struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The type of dependency describes the nature of the dependency being
	// managed for the workload.
	DependencyType *string `json:"DependencyType,omitempty" query:"dependencytype" gorm:"not null" validate:"required"`

	// Providers are the workload definitions that satisfy the dependency.  Only
	// applicable if the dependency is of type SupportService or Workload.
	Providers []*WorkloadDefinition `json:"Providers,omitempty" query:"providers" gorm:"many2many:provider_definitions;" validate:"optional"`

	// ConsumingDefinitions are the workload definitions that declared this
	// dependency and rely upon it.
	ConsumingDefinitions []*WorkloadDefinition `json:"ConsumingDefinitions,omitempty" query:"consumingdefinitions" gorm:"many2many:consumer_definitions;" validate:"optional"`

	// ConsumingInstances are the workload instances that are consuming this
	// dependency.  When this array is empty for a non-persistent dependency,
	// the dependency should be removed.
	ConsumingInstances []*WorkloadInstance `json:"ConsumingInstances,omitempty" query:"consuminginstances" gorm:"many2many:consumer_instances;" validate:"optional"`

	// If true, this dependency should not be shared with other workload
	// instances.
	Dedicated *bool `json:"Dedicated,omitempty" query:"dedicated" gorm:"default:false" validate:"optional"`

	// If persistent, this dependency should remain running even when no
	// instances are consuming it.
	Persistent *bool `json:"Persistent,omitempty" query:"persistent" gorm:"default:false" validate:"optional"`

	// The status of the dependency to expose whether it is ready and healthy.
	Status *string `json:"Status,omitempty" query:"Status" validate:"optional"`
}
