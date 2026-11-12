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
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
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
//
// A child control plane allocates only from the pool its parent gave it.
// With none, it returns "" and allocates nothing, and the cluster gets the
// layout every cluster used to get: the default pool is its parent's, whose
// ranges this database doesn't hold.
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

		block, allocates, err := nextNetworkCidr(apiClient, apiServer)
		if err != nil {
			return "", err
		}
		if !allocates {
			return "", nil
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
// no runtime and no child control plane's pool uses, and whether this control
// plane allocates at all: a child with no pool doesn't.
func nextNetworkCidr(apiClient *http.Client, apiServer string) (netip.Prefix, bool, error) {
	self, err := client.GetSelfControlPlaneInstance(apiClient, apiServer)
	if err != nil {
		return netip.Prefix{}, false, fmt.Errorf("failed to get this control plane's instance: %w", err)
	}
	ownPool, err := v0.ParseNetworkCidrPool(self.NetworkCidrPool)
	if err != nil {
		return netip.Prefix{}, false, fmt.Errorf("failed to read this control plane's network range pool: %w", err)
	}
	if self.ParentControlPlaneInstanceID != nil && len(ownPool) == 0 {
		return netip.Prefix{}, false, nil
	}
	pool, err := v0.NetworkCidrPoolOrDefault(self.NetworkCidrPool)
	if err != nil {
		return netip.Prefix{}, false, fmt.Errorf("failed to read this control plane's network range pool: %w", err)
	}

	inUse, err := networkCidrsInUse(apiClient, apiServer, self)
	if err != nil {
		return netip.Prefix{}, false, err
	}
	block, err := v0.AllocateNetworkCidr(pool, inUse)

	return block, true, err
}

// networkCidrsInUse returns the ranges a new runtime's range must stay clear
// of: every runtime's range, every child control plane's pool, the pools of
// deleted children this control plane has reserved for their clusters, and
// the networks of clusters still being destroyed.  The
// control planes this one keeps rows for that aren't itself are its children;
// the rows for its parent and the genesis carry no pool.
func networkCidrsInUse(apiClient *http.Client, apiServer string, self *v0.ControlPlaneInstance) ([]netip.Prefix, error) {
	inUse, err := v0.ParseReservedNetworkCidrs(self.ReservedNetworkCidrs)
	if err != nil {
		return nil, fmt.Errorf("failed to read this control plane's reserved network ranges: %w", err)
	}

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

	clusters, err := clusterNetworksInUse(apiClient, apiServer)
	if err != nil {
		return nil, err
	}

	return append(inUse, clusters...), nil
}

// clusterNetworksInUse returns the networks the EKS, GKE and OKE clusters
// still hold, from their provider instances' recorded state.  The runtime
// controller removes a runtime once it schedules its provider's deletion,
// while the cluster's network lives until the destroy removes it, so a
// range is in use as long as a cluster's state holds it.
func clusterNetworksInUse(apiClient *http.Client, apiServer string) ([]netip.Prefix, error) {
	var inUse []netip.Prefix

	eksInstances, err := client.GetAwsEksKubernetesRuntimeInstances(apiClient, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get EKS kubernetes runtime instances: %w", err)
	}
	for _, instance := range *eksInstances {
		inUse = append(inUse, provider.NetworkCidrsInInventory(instance.ResourceInventory)...)
	}

	gkeInstances, err := client.GetGcpGkeKubernetesRuntimeInstances(apiClient, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get GKE kubernetes runtime instances: %w", err)
	}
	for _, instance := range *gkeInstances {
		inUse = append(inUse, provider.NetworkCidrsInInventory(instance.ResourceInventory)...)
	}

	okeInstances, err := client.GetOciOkeKubernetesRuntimeInstances(apiClient, apiServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get OKE kubernetes runtime instances: %w", err)
	}
	for _, instance := range *okeInstances {
		inUse = append(inUse, provider.NetworkCidrsInInventory(instance.ResourceInventory)...)
	}

	return inUse, nil
}

// ClusterNetworkCidr returns the network range a provider instance's cluster
// is laid out in, from its kubernetes runtime instance.  A cluster yet to be
// provisioned, with no recorded resources, its creation not confirmed and no
// deletion scheduled, is allocated a range when its runtime has none.  One
// already provisioned keeps what its runtime records: none for a cluster
// created before ranges were allocated, whose network was laid out without
// one and would be replaced by a new layout.  A cluster being deleted whose
// runtime is already gone has none.
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
		// the runtime controller may remove the runtime before the provider's
		// deletion has destroyed the cluster.  The destroy doesn't lay the
		// network out, and a create resumed on the stack keeps its range, so
		// a cluster being deleted needs none
		if reconciliation.DeletionScheduled != nil && errors.Is(err, client_lib.ErrObjectNotFound) {
			return "", nil
		}
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
