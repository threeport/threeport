package v0

// MachineRuntimeDefinition is the configuration for a machine runtime.  It
// serves as a template for provisioning machine runtime instances.
type MachineRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The associated machine runtime instances that are deployed from this
	// definition.
	MachineRuntimeInstances []*MachineRuntimeInstance `json:",omitempty" validate:"optional,association"`
}

// MachineRuntimeInstance is a machine that serves as a runtime for workloads.
type MachineRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The hostname or IP address used to reach the machine.
	Hostname *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The SSH username for authenticating to the machine.
	SSHUser *string `json:",omitempty" validate:"required" gorm:"not null"`

	// The SSH private key for authenticating to the machine.
	SSHKey *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// The SSH password for authenticating to the machine.
	SSHPassword *string `json:",omitempty" validate:"optional" encrypt:"true"`

	// The SSH port on the machine.
	Port *int `json:",omitempty" validate:"optional" gorm:"default:22"`

	// The remote machine's SSH public host key, used to verify identity on
	// connection. If not provided, captured on first connection.
	HostKey *string `json:",omitempty" validate:"optional"`

	// The machine runtime definition for this instance.  Optional because
	// imported machines may not have an associated definition.
	MachineRuntimeDefinitionID *uint `json:",omitempty" validate:"optional" relationship:"requires"`

	// The associated machine workload instances running on this machine runtime.
	MachineWorkloadInstances []*MachineWorkloadInstance `json:",omitempty" validate:"optional,association"`
}
