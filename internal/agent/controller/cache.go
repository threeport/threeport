package controller

import (
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

// CacheOptions returns the manager cache options that scope the agent to the
// ThreeportWorkloads of the named control plane.  ThreeportWorkload is
// cluster-scoped, so without this every agent on a cluster would reconcile
// every control plane's workloads.  Scoping the cache rather than filtering in
// Reconcile keeps other control planes' workloads out of this agent's cache
// altogether.  An empty name leaves the cache unscoped so that an agent that
// has not been told its control plane keeps working.
func CacheOptions(controlPlaneName string) cache.Options {
	if controlPlaneName == "" {
		return cache.Options{}
	}

	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&agentapi.ThreeportWorkload{}: {
				Label: labels.SelectorFromSet(
					agent.ThreeportWorkloadLabels(controlPlaneName),
				),
			},
		},
	}
}
