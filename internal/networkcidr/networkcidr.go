// Package networkcidr allocates kubernetes runtimes their network ranges.
package networkcidr

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"

	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// allocationAttempts bounds the allocations of a range another runtime took
// first.  Each attempt reads the ranges in use again, so it skips the block
// that runtime took.
const allocationAttempts = 5

// EnsureRuntimeNetworkCidr returns the network range of a kubernetes runtime
// instance, allocating one when it has none: the first /16 of this control
// plane's pool that no runtime has, outside the pools of its child control
// planes.  The API refuses a range another runtime took at the same time,
// and the allocation is then made again.
func EnsureRuntimeNetworkCidr(apiClient *http.Client, apiServer string, runtimeID uint) (string, error) {
	var lastErr error
	for attempt := 0; attempt < allocationAttempts; attempt++ {
		runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiServer, runtimeID)
		if err != nil {
			return "", fmt.Errorf("failed to get kubernetes runtime instance %d: %w", runtimeID, err)
		}
		if runtime.NetworkCidr != nil && *runtime.NetworkCidr != "" {
			return *runtime.NetworkCidr, nil
		}

		block, err := nextNetworkCidr(apiClient, apiServer)
		if err != nil {
			return "", err
		}
		cidr := block.String()
		if _, err := client.UpdateKubernetesRuntimeInstance(apiClient, apiServer, &v0.KubernetesRuntimeInstance{
			Common:      v0.Common{ID: &runtimeID},
			NetworkCidr: &cidr,
		}); err != nil {
			lastErr = err
			continue
		}

		return cidr, nil
	}

	return "", fmt.Errorf("failed to allocate a network range to kubernetes runtime instance %d after %d attempts: %w",
		runtimeID, allocationAttempts, lastErr)
}

// nextNetworkCidr returns the next block of this control plane's pool that
// no runtime and no child control plane's pool uses.
func nextNetworkCidr(apiClient *http.Client, apiServer string) (netip.Prefix, error) {
	self, err := client.GetSelfControlPlaneInstance(apiClient, apiServer)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed to get this control plane's instance: %w", err)
	}
	pool, err := v0.NetworkCidrPoolOrDefault(self.NetworkCidrPool)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed to read this control plane's network range pool: %w", err)
	}

	inUse, err := networkCidrsInUse(apiClient, apiServer, self)
	if err != nil {
		return netip.Prefix{}, err
	}

	return v0.AllocateNetworkCidr(pool, inUse)
}

// networkCidrsInUse returns the ranges a new runtime's range must stay clear
// of: every runtime's range, and every child control plane's pool.  The
// control planes this one keeps rows for that aren't itself are its children;
// the rows for its parent and the genesis carry no pool.
func networkCidrsInUse(apiClient *http.Client, apiServer string, self *v0.ControlPlaneInstance) ([]netip.Prefix, error) {
	var inUse []netip.Prefix

	runtimes, err := client.GetKubernetesRuntimeInstances(apiClient, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes runtime instances: %w", err)
	}
	for _, runtime := range *runtimes {
		if runtime.NetworkCidr == nil || *runtime.NetworkCidr == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(*runtime.NetworkCidr)
		if err != nil {
			return nil, fmt.Errorf("kubernetes runtime instance %s has a network range that can't be read: %w",
				deref(runtime.Name), err)
		}
		inUse = append(inUse, prefix)
	}

	controlPlanes, err := client.GetControlPlaneInstances(apiClient, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get control plane instances: %w", err)
	}
	for _, controlPlane := range *controlPlanes {
		if self.ID != nil && controlPlane.ID != nil && *controlPlane.ID == *self.ID {
			continue
		}
		if controlPlane.NetworkCidrPool == nil {
			continue
		}
		pool, err := v0.ParseNetworkCidrPool(controlPlane.NetworkCidrPool)
		if err != nil {
			return nil, fmt.Errorf("control plane instance %s has a network range pool that can't be read: %w",
				deref(controlPlane.Name), err)
		}
		inUse = append(inUse, pool...)
	}

	return inUse, nil
}

// ClusterNetworkCidr returns the network range a provider instance's cluster
// is laid out in, from its kubernetes runtime instance.  A cluster yet to be
// provisioned, with no recorded resources, its creation not confirmed and no
// deletion scheduled, is allocated a range when its runtime has none.  One
// already provisioned keeps what its runtime records: none for a cluster
// created before ranges were allocated, whose network was laid out without
// one and would be replaced by a new layout.
func ClusterNetworkCidr(
	apiClient *http.Client,
	apiServer string,
	runtimeID *uint,
	reconciliation v0.Reconciliation,
	inventory *datatypes.JSON,
) (string, error) {
	if runtimeID == nil {
		return "", errors.New("the provider instance names no kubernetes runtime instance")
	}

	provisioned := reconciliation.CreationConfirmed != nil || provider.HasInventory(inventory)
	if provisioned || reconciliation.DeletionScheduled != nil {
		runtime, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiServer, *runtimeID)
		if err != nil {
			return "", fmt.Errorf("failed to get kubernetes runtime instance %d: %w", *runtimeID, err)
		}
		return deref(runtime.NetworkCidr), nil
	}

	return EnsureRuntimeNetworkCidr(apiClient, apiServer, *runtimeID)
}

// deref returns a string pointer's value, or "" for nil.
func deref(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
