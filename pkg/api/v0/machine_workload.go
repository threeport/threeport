package v0

// MachineWorkloadDefinition is the configuration for a workload that runs on
// a machine runtime.
type MachineWorkloadDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The shell script to run when a machine workload instance is created.
	CreateScript *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The shell script to run when a machine workload instance is updated.
	UpdateScript *string `json:",omitempty" validate:"optional"`

	// The shell script to run when a machine workload instance is deleted.
	DeleteScript *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The shell to use for script execution.
	Shell *string `json:",omitempty" validate:"optional" gorm:"default:/bin/bash"`

	// The working directory for script execution.
	WorkingDir *string `json:",omitempty" validate:"optional"`

	// The timeout in seconds for script execution.
	Timeout *int `json:",omitempty" validate:"optional"`

	// The environment variables to set for the workload as KEY=VALUE entries.
	Env *[]string `json:",omitempty" validate:"optional" gorm:"type:jsonb;serializer:json" encrypt:"true"`

	// The associated machine workload instances that are deployed from this
	// definition.
	MachineWorkloadInstances []*MachineWorkloadInstance `json:",omitempty" validate:"optional,association"`
}

// MachineWorkloadInstance is a deployed instance of a workload running on a
// machine runtime.
type MachineWorkloadInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The machine runtime on which the workload is deployed.
	MachineRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// The definition used to configure the machine workload instance.
	MachineWorkloadDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null"`

	// The latest status of the workload instance as observed by the reconciler
	// when the most recent script execution completed.
	Status *string `json:",omitempty" validate:"optional"`

	// The environment variables set for the workload as KEY=VALUE entries.
	Env *[]string `json:",omitempty" validate:"optional" gorm:"type:jsonb;serializer:json" encrypt:"true"`
}
