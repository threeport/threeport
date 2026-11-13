package v0

import (
	"fmt"

	"gorm.io/gorm"

	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// kubernetesRuntimeInitialNodes is the number of nodes the definition
// reconciler gives a kubernetes runtime definition's default node group to
// start with.
const kubernetesRuntimeInitialNodes = 2

// nodeCounts are the sizes of a definition's default node group, and the
// number of zones its nodes run in, zero where the maximum needn't cover them.
type nodeCounts struct {
	minimum, initial, maximum *int
	zones                     int
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
// an update, when the update sets any of them.  A PATCH's sizes are taken
// with the stored ones it leaves as they are.  counts reads the sizes from a
// definition.
func validateNodeCountsUpdate(tx *gorm.DB, counts func(any) (nodeCounts, bool)) error {
	incoming, ok := counts(lib.IncomingValues(tx))
	if !ok {
		return fmt.Errorf("cannot validate node counts: update carries %T", lib.IncomingValues(tx))
	}
	if lib.IsFullReplace(tx) {
		return validateNodeCounts(incoming)
	}
	if incoming.minimum == nil && incoming.initial == nil && incoming.maximum == nil && incoming.zones == 0 {
		return nil
	}

	current, err := lib.LockObjectInDB(tx)
	if err != nil {
		return err
	}
	stored, ok := counts(current)
	if !ok {
		return fmt.Errorf("cannot validate node counts: loaded %T", current)
	}
	if incoming.minimum != nil {
		stored.minimum = incoming.minimum
	}
	if incoming.initial != nil {
		stored.initial = incoming.initial
	}
	if incoming.maximum != nil {
		stored.maximum = incoming.maximum
	}
	if incoming.zones != 0 {
		stored.zones = incoming.zones
	}

	return validateNodeCounts(stored)
}

// eksNodeCounts reads an EKS definition's node group sizes.  Its node group
// spans its zones, so its maximum needn't cover them.
func eksNodeCounts(value any) (nodeCounts, bool) {
	definition, ok := value.(*AwsEksKubernetesRuntimeDefinition)
	if !ok {
		return nodeCounts{}, false
	}

	return nodeCounts{
		minimum: definition.DefaultNodeGroupMinimumSize,
		initial: definition.DefaultNodeGroupInitialSize,
		maximum: definition.DefaultNodeGroupMaximumSize,
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

	return nil
}
