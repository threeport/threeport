package v0

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/datatypes"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// ResolveNetworkCidrFlags checks --network-cidr and --network-cidr-pool and
// returns the range the control plane's cluster gets and the pool the control
// plane allocates its runtimes' ranges from.
//
//   - The pool is a comma-separated list of blocks, each a /16 or larger, none
//     overlapping another.  Empty is the default pool.  It applies on every
//     provider, kind included, since a control plane on kind can still create
//     clusters elsewhere.
//   - The range is a /16 or larger, outside the ranges clusters use
//     internally.  It doesn't have to be in the pool.  A cluster tptctl creates
//     gets the first block of the pool when none is given.  With
//     --control-plane-only, a range given records the existing cluster's, so
//     no runtime is allocated it later; none records nothing.  A kind cluster's
//     network isn't laid out by Threeport, so it takes none.
func ResolveNetworkCidrFlags(
	networkCidr string,
	networkCidrPool string,
	infraProvider string,
	controlPlaneOnly bool,
) (string, []string, error) {
	var pool []string
	for _, block := range strings.Split(networkCidrPool, ",") {
		if block = strings.TrimSpace(block); block != "" {
			pool = append(pool, block)
		}
	}
	poolJSON, err := NetworkCidrPoolJSON(pool)
	if err != nil {
		return "", nil, err
	}
	poolBlocks, err := v0.NetworkCidrPoolOrDefault(poolJSON)
	if err != nil {
		return "", nil, fmt.Errorf("--network-cidr-pool: %w", err)
	}

	if infraProvider == v0.KubernetesRuntimeInfraProviderKind {
		if networkCidr != "" {
			return "", nil, fmt.Errorf("--network-cidr does not apply to a %s cluster, whose network Threeport doesn't lay out", infraProvider)
		}
		return "", pool, nil
	}

	if networkCidr == "" {
		if controlPlaneOnly {
			return "", pool, nil
		}
		first, err := v0.AllocateNetworkCidr(poolBlocks, nil)
		if err != nil {
			return "", nil, fmt.Errorf("--network-cidr-pool: %w", err)
		}
		return first.String(), pool, nil
	}

	prefix, err := v0.ParseNetworkCidr("--network-cidr", networkCidr)
	if err != nil {
		return "", nil, err
	}

	return prefix.String(), pool, nil
}

// NetworkCidrPoolJSON returns a pool as the JSON array a ControlPlaneInstance
// stores, or nil for an empty pool, which is the default one.
func NetworkCidrPoolJSON(pool []string) (*datatypes.JSON, error) {
	if len(pool) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the network range pool: %w", err)
	}
	raw := datatypes.JSON(encoded)

	return &raw, nil
}

// genesisNetworkCidr returns the network range recorded on the runtime of the
// control plane's cluster: the one the infra laid the cluster out in, which a
// create resumed on an older stack keeps from it, and with
// --control-plane-only the one given for the existing cluster.
func genesisNetworkCidr(opts threeport.Options, infra provider.KubernetesRuntimeInfra) string {
	if opts.ControlPlaneOnly {
		return opts.NetworkCidr
	}

	switch created := infra.(type) {
	case *provider.KubernetesRuntimeInfraEKS:
		return created.NetworkCidr
	case *provider.KubernetesRuntimeInfraGKE:
		return created.NetworkCidr
	case *provider.KubernetesRuntimeInfraOKE:
		return created.NetworkCidr
	}

	return ""
}
