package v0

import "gorm.io/datatypes"

// MachineRuntimeDefinition is the configuration for a machine runtime.  It
// serves as a template for provisioning machine runtime instances.
type MachineRuntimeDefinition struct {
	Common     `swaggerignore:"true" mapstructure:",squash"`
	Definition `mapstructure:",squash"`

	// The infrastructure provider that provisions machines from this definition
	InfraProvider *string `validate:"optional"`

	// The provider-specific machine type to provision
	MachineType *string `validate:"optional"`

	// The provider image identifier used to boot the machine
	ImageID *string `validate:"optional"`

	// The associated machine runtime instances that are deployed from this
	// definition.
	MachineRuntimeInstances []*MachineRuntimeInstance `validate:"optional,association"`
}

// MachineRuntimeInstance is a machine that serves as a runtime for workloads.
type MachineRuntimeInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The hostname or IP address used to reach the machine. Optional at
	// create so the abstract instance can exist before the machine is
	// provisioned; populated once the machine is reachable.
	//
	// idx_machine_runtime_instance_hostname is a partial unique index that
	// allows at most one live instance per hostname, so a single machine
	// cannot be represented by two records that each drive their own
	// reconciliation against it. The deleted_at predicate keeps
	// soft-deleted rows out of the unique slot, so the hostname of a
	// deleted instance is available to a new one right away. CockroachDB
	// treats every NULL as distinct in a unique index, so any number of
	// instances may hold no hostname while they wait on provisioning.
	// An empty string is excluded the same way, because it is not a
	// hostname either.
	Hostname *string `json:",omitempty" validate:"optional" gorm:"uniqueIndex:idx_machine_runtime_instance_hostname,where:deleted_at IS NULL AND hostname IS NOT NULL AND hostname <> ''"`

	// The SSH username for authenticating to the machine. Optional at create
	// for the same reason as the hostname; populated once the machine is
	// provisioned.
	SSHUser *string `json:",omitempty" validate:"optional"`

	// The SSH private key for authenticating to the machine.
	SSHKey *string `validate:"optional" encrypt:"true"`

	// The SSH password for authenticating to the machine.
	SSHPassword *string `validate:"optional" encrypt:"true"`

	// The SSH port on the machine.
	Port *int `validate:"optional" gorm:"default:22"`

	// The remote machine's SSH public host key, used to verify identity on
	// connection. If not provided, captured on first connection.
	HostKey *string `validate:"optional"`

	// The provider region in which the machine is provisioned
	Region *string `validate:"optional"`

	// The provider network identifier the machine attaches to
	NetworkID *string `validate:"optional"`

	// The provider subnet identifier the machine attaches to
	SubnetID *string `validate:"optional" gorm:"type:text"`

	// An inventory of all provider resources backing this machine
	ResourceInventory *datatypes.JSON `validate:"optional"`

	// The machine runtime definition for this instance.  Optional because
	// imported machines may not have an associated definition.
	MachineRuntimeDefinitionID *uint `validate:"optional" relationship:"requires"`

	// The associated machine workload instances running on this machine runtime.
	MachineWorkloadInstances []*MachineWorkloadInstance `validate:"optional,association"`
}
