package v0

import "gorm.io/datatypes"

// MachineRuntimeDefinition is the configuration for a machine runtime.  It
// serves as a template for provisioning machine runtime instances.
type MachineRuntimeDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The infrastructure provider that provisions machines from this definition
	InfraProvider *string `json:",omitempty" validate:"optional"`

	// The provider account name that selects which account the machine is
	// provisioned on, empty falling back to the default provider account
	InfraProviderAccountName *string `json:",omitempty" validate:"optional"`

	// The compute capacity of the machine
	MachineSize *string `json:",omitempty" validate:"optional" gorm:"default:Medium"`

	// The CPU-to-memory ratio of the machine
	MachineProfile *string `json:",omitempty" validate:"optional" gorm:"default:Balanced"`

	// The provider-specific machine type
	MachineType *string `json:",omitempty" validate:"optional"`

	// The provider image identifier used to boot the machine
	ImageID *string `json:",omitempty" validate:"optional"`

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
	Hostname *string `validate:"optional"`

	// The SSH username for authenticating to the machine. Optional at create
	// for the same reason as the hostname; populated once the machine is
	// provisioned.
	SSHUser *string `validate:"optional"`

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
	Region *string `json:",omitempty" validate:"optional"`

	// The abstract threeport location for the machine
	Location *string `json:",omitempty" validate:"optional"`

	// The provider network identifier the machine attaches to.
	NetworkID *string `json:",omitempty" validate:"optional"`

	// The provider subnet identifier the machine attaches to
	SubnetID *string `json:",omitempty" gorm:"type:text" validate:"optional"`

	// IngressRules are the firewall ingress rules applied to the machine.
	// Rules are provider-agnostic; each provider reconciler translates them
	// to its native firewall shape. Callers who need SSH must include a
	// tcp/22 rule here; no rule is added by default.
	IngressRules *[]IngressRule `json:",omitempty" validate:"optional" gorm:"type:jsonb;serializer:json"`

	// NetworkCIDR is the CIDR block for the VPC network the machine is
	// placed in. Optional; when unset the reconciler falls back to a
	// provider-specific default.
	NetworkCIDR *string `json:",omitempty" validate:"optional" gorm:"column:network_cidr"`

	// SubnetCIDR is the CIDR block for the subnet the machine's primary
	// interface is placed in. Optional; when unset the reconciler falls
	// back to a provider-specific default.
	SubnetCIDR *string `json:",omitempty" validate:"optional" gorm:"column:subnet_cidr"`

	// AssignPublicIP controls whether the primary network interface gets
	// an external IP address. Defaults false; the reconciler reads back
	// the assigned address into Hostname after provisioning when true.
	AssignPublicIP *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// An inventory of all provider resources backing this machine
	ResourceInventory *datatypes.JSON `json:",omitempty" validate:"optional"`

	// The machine runtime definition for this instance.  Optional because
	// imported machines may not have an associated definition.
	MachineRuntimeDefinitionID *uint `validate:"optional" relationship:"requires"`

	// The associated machine workload instances running on this machine runtime.
	MachineWorkloadInstances []*MachineWorkloadInstance `validate:"optional,association"`
}
