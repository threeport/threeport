//go:generate threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate threeport-codegen controller --filename $GOFILE
package v0

import "gorm.io/datatypes"

// +threeport-codegen:reconciler
// RadiusWorkloadDefinition is the configuration for a workload deployed with
// radius.
type RadiusWorkloadDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The bicep config that defines the radius workload.
	BicepDocument *string `json:"BicepDocument,omitempty" gorm:"not null" validate:"required"`

	// The associated workload instances that are deployed from this definition.
	RadiusWorkloadInstances []*RadiusWorkloadInstance `json:"RadiusWorkloadInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// RadiusWorkloadInstance is a deployed instance of a radius workload.  It
// includes the values that are passed to the params required by the bicep
// config.
type RadiusWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The kubernetes runtime to which the workload is deployed.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// The definition used to configure the workload instance.
	RadiusWorkloadDefinitionID *uint `json:"RadiusWorkloadDefinitionID,omitempty" query:"radiusworkloaddefinitionid" gorm:"not null" validate:"required"`

	// The latest status of a workload instance.
	Status *string `json:"Status,omitempty" query:"status" validate:"optional"`

	// The parameters to pass in when deploying an instance of the radius
	// workload.
	//RuntimeParameters []*RadiusRuntimeParameter `json:"RuntimeParameters,omitempty" query:"runtimeparameters" validate:"optional,association"`
	RuntimeParameters *datatypes.JSON `json:"RuntimeParameters,omitempty" query:"runtimeparameters" validate:"optional,association"`

	//// All events generated for the workload instance that aren't related to a
	//// particular workload resource instance.
	//Events []*WorkloadEvent `json:"Events,omitempty" query:"events" validate:"optional"`

	//// The threeport objects that are deployed to support the workload instance.
	//AttachedObjectReferences []*AttachedObjectReference `json:"AttachedObjectReferences,omitempty" query:"attachedobjectreferences" validate:"optional,association"`
}

//// RadiusRuntimeParameter is a key-value pair that is used to set runtime values
//// that are passed to radius when deploying a workload.
//type RadiusRuntimeParameter struct {
//	Common `swaggerignore:"true" mapstructure:",squash"`
//
//	// The instance that the runtime paramter key value pair belongs to.
//	RadiusWorkloadInstanceID *uint `json:"RadiusWorkloadInstanceID,omitempty" query:"radiusworkloadinstanceid" gorm:"not null" validate:"required"`
//
//	// The runtime parameter key.
//	Key *string `json:"Key,omitempty" query:"key" gorm:"not null" validate:"required"`
//
//	// The runtime parameter value.
//	Value *string `json:"Value,omitempty" query:"value" gorm:"not null" validate:"required"`
//}
