package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// devMessageBrokerStorageSize and devDatabaseStorageSize are the volumes
	// a development control plane's message broker servers and database
	// nodes get: enough for a cluster meant to be disposable.
	devMessageBrokerStorageSize = "5Gi"
	devDatabaseStorageSize      = "20Gi"

	// prodMessageBrokerStorageSize and prodDatabaseStorageSize are the
	// volumes a production control plane's get, sized to absorb growth
	// nobody predicted, since a full message broker refuses notifications
	// and a full database refuses writes.
	prodMessageBrokerStorageSize = "250Gi"
	prodDatabaseStorageSize      = "1Ti"
)

// messageBrokerStorageSize returns the volume each message broker server
// gets: the one given, or the tier's.
func (cpi *ControlPlaneInstaller) messageBrokerStorageSize() string {
	if cpi.Opts.MessageBrokerStorageSize != "" {
		return cpi.Opts.MessageBrokerStorageSize
	}
	if string(cpi.Opts.Tier) == ControlPlaneTierProd {
		return prodMessageBrokerStorageSize
	}

	return devMessageBrokerStorageSize
}

// databaseStorageSize returns the volume each database node gets: the one
// given, or the tier's.
func (cpi *ControlPlaneInstaller) databaseStorageSize() string {
	if cpi.Opts.DatabaseStorageSize != "" {
		return cpi.Opts.DatabaseStorageSize
	}
	if string(cpi.Opts.Tier) == ControlPlaneTierProd {
		return prodDatabaseStorageSize
	}

	return devDatabaseStorageSize
}

// ValidateStorageSize refuses a volume size that isn't a positive Kubernetes
// quantity, such as 250Gi, naming the flag it was given with.  An empty size
// uses the tier's.
func ValidateStorageSize(flag, size string) error {
	if size == "" {
		return nil
	}
	quantity, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("--%s %q is not a size such as 250Gi: %w", flag, size, err)
	}
	if quantity.Sign() <= 0 {
		return fmt.Errorf("--%s %s is not a size greater than zero", flag, size)
	}

	return nil
}
