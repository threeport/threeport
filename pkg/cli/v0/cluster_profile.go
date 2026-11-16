package v0

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	mapping "github.com/threeport/threeport/pkg/mapping/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controlPlaneNodeProfile is the node profile of the cluster tptctl up
// creates for a control plane.  Its machine types come from the same mapping
// a KubernetesRuntimeDefinition's NodeProfile and NodeSize do, so the
// definition tptctl records for the cluster describes the nodes it has.
const controlPlaneNodeProfile = "Balanced"

// gkeClusterProfile is the GKE cluster tptctl up creates for a control plane.
// GKE creates a regional cluster's node pool in three zones, and the node
// counts are per zone.
type gkeClusterProfile struct {
	NodeSize         string
	MachineType      string
	LocationPolicy   string
	InitialNodeCount int32
	MinNodeCount     int32
	MaxNodeCount     int32
}

// gkeControlPlaneProfile returns the GKE cluster for a control plane tier.
//
// A production cluster runs on general purpose machines that don't depend on
// CPU bursting, which a database under sustained load uses up, and its
// autoscaler keeps the zones balanced, so a node in each of three zones is
// there for each of the database's three replicas.  A development cluster is
// the one tptctl has always created.
func gkeControlPlaneProfile(tier threeport.ControlPlaneTier) (gkeClusterProfile, error) {
	profile := gkeClusterProfile{
		NodeSize:         "Medium",
		LocationPolicy:   "ANY",
		InitialNodeCount: 2,
		MinNodeCount:     1,
		MaxNodeCount:     10,
	}
	if string(tier) == threeport.ControlPlaneTierProd {
		profile = gkeClusterProfile{
			NodeSize:         "Large",
			LocationPolicy:   "BALANCED",
			InitialNodeCount: 1,
			MinNodeCount:     1,
			MaxNodeCount:     10,
		}
	}

	machineType, err := mapping.GetMachineType(util.GcpProvider, controlPlaneNodeProfile, profile.NodeSize)
	if err != nil {
		return gkeClusterProfile{}, fmt.Errorf("failed to map the control plane's node size to a GCP machine type: %w", err)
	}
	profile.MachineType = machineType

	return profile, nil
}

// eksClusterProfile is the EKS cluster tptctl up creates for a control plane.
// With a node group per zone, the node counts are per node group.
type eksClusterProfile struct {
	NodeSize         string
	InstanceType     string
	ZoneCount        int32
	NodeGroupPerZone bool
	InitialNodeCount int32
	MinNodeCount     int32
	MaxNodeCount     int32
}

// eksControlPlaneProfile returns the EKS cluster for a control plane tier.
//
// A production cluster spans three zones, so losing one leaves two of the
// database's three replicas, with a node group in each zone, so the cluster
// autoscaler can replace a lost node in the zone its replica's volume is in,
// on general purpose instances that don't depend on CPU bursting.  A
// development cluster is the one tptctl has always created.
func eksControlPlaneProfile(tier threeport.ControlPlaneTier) (eksClusterProfile, error) {
	profile := eksClusterProfile{
		NodeSize:         "Medium",
		ZoneCount:        2,
		InitialNodeCount: 3,
		MinNodeCount:     3,
		MaxNodeCount:     250,
	}
	if string(tier) == threeport.ControlPlaneTierProd {
		profile = eksClusterProfile{
			NodeSize:         "Large",
			ZoneCount:        3,
			NodeGroupPerZone: true,
			InitialNodeCount: 1,
			MinNodeCount:     1,
			MaxNodeCount:     10,
		}
	}

	instanceType, err := mapping.GetMachineType(util.AwsProvider, controlPlaneNodeProfile, profile.NodeSize)
	if err != nil {
		return eksClusterProfile{}, fmt.Errorf("failed to map the control plane's node size to an AWS instance type: %w", err)
	}
	profile.InstanceType = instanceType

	return profile, nil
}

// recordClusterProfile sets on the KubernetesRuntimeDefinition of the cluster
// tptctl up creates the values that describe it, so the definition matches
// the cluster: whether it spans three zones, the node size and profile its
// machine type maps from, and its maximum node count.  A provider whose
// cluster has no profile here keeps the definition's defaults.
func recordClusterProfile(definition *v0.KubernetesRuntimeDefinition, tier threeport.ControlPlaneTier) error {
	switch *definition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderGKE:
		profile, err := gkeControlPlaneProfile(tier)
		if err != nil {
			return err
		}
		// a regional cluster spans three zones on either tier
		definition.HighAvailability = util.Ptr(true)
		definition.NodeProfile = util.Ptr(controlPlaneNodeProfile)
		definition.NodeSize = util.Ptr(profile.NodeSize)
		definition.NodeMaximum = util.Ptr(int(profile.MaxNodeCount))
	case v0.KubernetesRuntimeInfraProviderEKS:
		profile, err := eksControlPlaneProfile(tier)
		if err != nil {
			return err
		}
		definition.HighAvailability = util.Ptr(profile.ZoneCount >= 3)
		definition.NodeProfile = util.Ptr(controlPlaneNodeProfile)
		definition.NodeSize = util.Ptr(profile.NodeSize)
		// the maximum of the default node group, which a production
		// cluster runs one of in each zone
		definition.NodeMaximum = util.Ptr(int(profile.MaxNodeCount))
	}

	return nil
}
