package provider

import (
	"gorm.io/datatypes"
)

// InfraProvider defines the minimal interface any infrastructure backend must
// implement to benefit from durable lifecycle management. Providers that use
// Pulumi, Terraform, vanilla Go SDK calls, or Helm all satisfy this interface.
type InfraProvider interface {
	// DeployInfra creates the infrastructure (blocking).
	DeployInfra() error

	// DestroyInfra tears down the infrastructure (blocking).
	DestroyInfra() error

	// SetStackState restores previously persisted state for crash recovery.
	SetStackState(state *datatypes.JSON) error

	// GetStackState returns the current infrastructure state for persistence.
	GetStackState() (*datatypes.JSON, error)
}

// StreamableProvider supports real-time state streaming during operations.
// Only backends that write state to a local file (e.g. Pulumi, Terraform)
// implement this. The lifecycle handler uses type assertion to check for
// streaming support and skips it for non-streaming providers.
type StreamableProvider interface {
	InfraProvider

	// GetStateFilePath returns the path to the state file on disk.
	GetStateFilePath() (string, error)

	// ReadStateFile reads the current state directly from disk.
	// Returns nil (not error) if the file doesn't exist yet.
	ReadStateFile() (*datatypes.JSON, error)
}

// RefreshableProvider supports syncing state with cloud reality before
// operations. Providers that track resource state (Pulumi, Terraform)
// implement this to clear stale pending operations and detect drift.
type RefreshableProvider interface {
	InfraProvider

	// RefreshStack refreshes the state to match cloud reality.
	RefreshStack() error
}

// CloudVerifier supports asking the cloud whether infrastructure for an
// instance exists, independent of any recorded state. The lifecycle uses it to
// check a delete against reality, so a missing inventory cannot make the
// controller confirm a delete while the infrastructure keeps running.
type CloudVerifier interface {
	InfraProvider

	// ResourcesExist reports whether the cloud still holds the instance's
	// primary infrastructure, such as its cluster. It returns an error when
	// that cannot be determined, never false.
	ResourcesExist() (bool, error)
}
