package v0

import (
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// kubernetesRuntimeInitialNodes is the number of nodes the definition
// reconciler gives a kubernetes runtime definition's default node group to
// start with.
const kubernetesRuntimeInitialNodes = 2

// nodeCounts are the sizes of a definition's default node group, and the
// number of zones its nodes run in, zero where the maximum needn't cover them.
// instanceType and nodeGroupPerZone are an EKS node group's, which the
// number of nodes its network holds depends on, and networkLimit returns that
// number for the counts, zero where it isn't known, and why.
type nodeCounts struct {
	minimum, initial, maximum *int
	zones                     int
	instanceType              *string
	nodeGroupPerZone          *bool
	networkLimit              func(nodeCounts) (int, string)
}

// validateNodeCounts checks a default node group's sizes: the initial size
// between the minimum and the maximum, the nodes it starts across the zones
// the nodes run in within the maximum, and a maximum that allows a node in
// each of those zones.  A size that isn't set isn't checked.
func validateNodeCounts(counts nodeCounts) error {
	if counts.minimum != nil && counts.initial != nil && *counts.minimum > *counts.initial {
		return util.NewBadRequestError(fmt.Sprintf(
			"DefaultNodeGroupMinimumSize %d is more than DefaultNodeGroupInitialSize %d",
			*counts.minimum, *counts.initial,
		))
	}
	if counts.initial != nil && counts.maximum != nil && *counts.initial > *counts.maximum {
		return util.NewBadRequestError(fmt.Sprintf(
			"DefaultNodeGroupInitialSize %d is more than DefaultNodeGroupMaximumSize %d",
			*counts.initial, *counts.maximum,
		))
	}
	if counts.minimum != nil && counts.maximum != nil && *counts.minimum > *counts.maximum {
		return util.NewBadRequestError(fmt.Sprintf(
			"DefaultNodeGroupMinimumSize %d is more than DefaultNodeGroupMaximumSize %d",
			*counts.minimum, *counts.maximum,
		))
	}
	if counts.maximum != nil && counts.networkLimit != nil {
		if limit, reason := counts.networkLimit(counts); limit > 0 && *counts.maximum > limit {
			return util.NewBadRequestError(fmt.Sprintf(
				"DefaultNodeGroupMaximumSize %d is more than the %d nodes the cluster's network holds: %s",
				*counts.maximum, limit, reason,
			))
		}
	}
	if counts.maximum != nil && *counts.maximum < counts.zones {
		return util.NewBadRequestError(fmt.Sprintf(
			"DefaultNodeGroupMaximumSize %d is less than the %d zones the nodes run in: each zone needs a node",
			*counts.maximum, counts.zones,
		))
	}
	// GKE starts a node pool's initial nodes in each of its zones, the
	// initial size divided across them and rounded up
	if counts.initial != nil && counts.maximum != nil && counts.zones > 1 {
		started := (*counts.initial + counts.zones - 1) / counts.zones * counts.zones
		if started > *counts.maximum {
			return util.NewBadRequestError(fmt.Sprintf(
				"DefaultNodeGroupInitialSize %d starts %d nodes, the same number in each of the %d zones, which is more than DefaultNodeGroupMaximumSize %d",
				*counts.initial, started, counts.zones, *counts.maximum,
			))
		}
	}

	return nil
}

// validateNodeCountsUpdate checks the node group sizes a definition has after
// an update that changes them.  A PATCH's sizes are taken with the stored
// ones it leaves as they are.  An update that leaves the sizes and zones as
// they are isn't checked against them: a definition created before a rule,
// whose clusters keep the node limits they were created with, takes an
// update of its other fields.  counts reads the sizes from a definition.
func validateNodeCountsUpdate(tx *gorm.DB, counts func(any) (nodeCounts, bool)) error {
	incoming, ok := counts(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate node counts: update carries %T", lib.IncomingValues(tx))
	}
	if !lib.IsFullReplace(tx) && incoming.minimum == nil && incoming.initial == nil && incoming.maximum == nil && incoming.zones == 0 &&
		incoming.instanceType == nil && incoming.nodeGroupPerZone == nil {
		return nil
	}

	var current any
	var err error
	if lib.IsFullReplace(tx) {
		current, err = lib.LoadObjectFromDB(tx)
	} else {
		current, err = lib.LockObjectInDB(tx)
	}
	if err != nil {
		return err
	}
	stored, ok := counts(current)
	if !ok {
		return fmt.Errorf("cannot validate node counts: loaded %T", current)
	}

	updated := incoming
	if !lib.IsFullReplace(tx) {
		updated = stored
		if incoming.minimum != nil {
			updated.minimum = incoming.minimum
		}
		if incoming.initial != nil {
			updated.initial = incoming.initial
		}
		if incoming.maximum != nil {
			updated.maximum = incoming.maximum
		}
		if incoming.zones != 0 {
			updated.zones = incoming.zones
		}
		if incoming.instanceType != nil {
			updated.instanceType = incoming.instanceType
		}
		if incoming.nodeGroupPerZone != nil {
			updated.nodeGroupPerZone = incoming.nodeGroupPerZone
		}
	}
	if sameNodeCounts(updated, stored) {
		return nil
	}

	return validateNodeCounts(updated)
}

// sameNodeCounts reports whether two definitions' node group sizes and zones
// are the same.
func sameNodeCounts(a, b nodeCounts) bool {
	return util.Deref(a.minimum) == util.Deref(b.minimum) && (a.minimum == nil) == (b.minimum == nil) &&
		util.Deref(a.initial) == util.Deref(b.initial) && (a.initial == nil) == (b.initial == nil) &&
		util.Deref(a.maximum) == util.Deref(b.maximum) && (a.maximum == nil) == (b.maximum == nil) &&
		a.zones == b.zones &&
		util.Deref(a.instanceType) == util.Deref(b.instanceType) && util.Deref(a.nodeGroupPerZone) == util.Deref(b.nodeGroupPerZone)
}

// eksNodeCounts reads an EKS definition's node group sizes.  Its node group
// spans its zones, so its maximum needn't cover them.
func eksNodeCounts(value any) (nodeCounts, bool) {
	definition, ok := value.(*AwsEksKubernetesRuntimeDefinition)
	if !ok {
		return nodeCounts{}, false
	}

	// its zones only bear on the network, as the node group spans them, so
	// they are read through the network limit, not zones
	zoneCount := definition.ZoneCount

	return nodeCounts{
		minimum:          definition.DefaultNodeGroupMinimumSize,
		initial:          definition.DefaultNodeGroupInitialSize,
		maximum:          definition.DefaultNodeGroupMaximumSize,
		instanceType:     definition.DefaultNodeGroupInstanceType,
		nodeGroupPerZone: definition.NodeGroupPerZone,
		networkLimit: func(counts nodeCounts) (int, string) {
			if counts.instanceType == nil {
				return 0, ""
			}
			return eksNetworkNodeLimit(*counts.instanceType, util.Deref(zoneCount), util.Deref(counts.nodeGroupPerZone))
		},
	}, true
}

// gkeNodeCounts reads a GKE definition's node pool sizes and the zones its
// nodes run in.
func gkeNodeCounts(value any) (nodeCounts, bool) {
	definition, ok := value.(*GcpGkeKubernetesRuntimeDefinition)
	if !ok {
		return nodeCounts{}, false
	}

	return nodeCounts{
		minimum: definition.DefaultNodeGroupMinimumSize,
		initial: definition.DefaultNodeGroupInitialSize,
		maximum: definition.DefaultNodeGroupMaximumSize,
		zones:   util.Deref(definition.ZoneCount),
		networkLimit: func(counts nodeCounts) (int, string) {
			return gkeNetworkNodeLimit(counts.zones)
		},
	}, true
}

// validateKubernetesRuntimeNodeMaximum checks a kubernetes runtime
// definition's NodeMaximum against the node group the definition reconciler
// makes of it: one that starts with kubernetesRuntimeInitialNodes nodes, in
// three zones for a highly available GKE cluster.
func validateKubernetesRuntimeNodeMaximum(definition *KubernetesRuntimeDefinition) error {
	if definition.NodeMaximum == nil {
		return nil
	}
	if *definition.NodeMaximum < kubernetesRuntimeInitialNodes {
		return util.NewBadRequestError(fmt.Sprintf(
			"NodeMaximum %d is less than the %d nodes a cluster starts with",
			*definition.NodeMaximum, kubernetesRuntimeInitialNodes,
		))
	}
	if definition.InfraProvider != nil && *definition.InfraProvider == KubernetesRuntimeInfraProviderGKE &&
		definition.HighAvailability != nil && *definition.HighAvailability && *definition.NodeMaximum < 3 {
		return util.NewBadRequestError(fmt.Sprintf(
			"NodeMaximum %d is less than the 3 zones a highly available GKE cluster's nodes run in: each zone needs a node",
			*definition.NodeMaximum,
		))
	}

	limit, reason := kubernetesRuntimeNetworkNodeLimit(definition)
	if limit > 0 && *definition.NodeMaximum > limit {
		return util.NewBadRequestError(fmt.Sprintf(
			"NodeMaximum %d is more than the %d nodes the cluster's network holds: %s",
			*definition.NodeMaximum, limit, reason,
		))
	}

	return nil
}

// kubernetesRuntimeNetworkNodeLimit returns the largest NodeMaximum the
// network of a kubernetes runtime definition's cluster holds, and the
// reason, or 0 where it isn't known: the cluster the definition reconciler
// makes of it, with the instance type its NodeSize and NodeProfile map to on
// EKS, in three zones when highly available and two otherwise, in one node
// group spanning them.
func kubernetesRuntimeNetworkNodeLimit(definition *KubernetesRuntimeDefinition) (int, string) {
	if definition.InfraProvider == nil {
		return 0, ""
	}
	highlyAvailable := util.Deref(definition.HighAvailability)

	switch *definition.InfraProvider {
	case KubernetesRuntimeInfraProviderEKS:
		if definition.NodeSize == nil || definition.NodeProfile == nil {
			return 0, ""
		}
		instanceType, err := mapping.GetMachineType(util.AwsProvider, *definition.NodeProfile, *definition.NodeSize)
		if err != nil {
			return 0, ""
		}
		zones := 2
		if highlyAvailable {
			zones = 3
		}
		return eksNetworkNodeLimit(instanceType, zones, false)
	case KubernetesRuntimeInfraProviderGKE:
		return gkeNetworkNodeLimit(1)
	case KubernetesRuntimeInfraProviderOKE:
		return okeNetworkNodeLimit()
	}

	return 0, ""
}
