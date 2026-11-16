package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

// ThreeportWorkload is cluster-scoped, so without scoping every agent on a
// cluster would reconcile every control plane's workloads.  The agent scopes
// what it caches with two label selectors, so another control plane's
// workloads never enter either cache:
//
//   - CacheOptions selects the workloads labeled for this control plane.  It
//     is set on the manager, so it backs the controller's primary watch.
//   - LegacyCacheOptions selects the workloads with no control plane label.
//     Those were created before the label existed, they carry the agent's
//     finalizer, and an agent that cannot see one can never remove it - the
//     resource would stay Terminating after its workload is deleted.
//     Nothing records which control plane owns an unlabeled resource (the
//     workload instance IDs it holds are only unique within one control
//     plane's database), so no agent can claim one safely; they are handled
//     as they were before the label, by whichever agent sees them, until the
//     workload instance is created again and the resource is replaced with a
//     labeled one.
//
// A single selector cannot express "labeled for me or unlabeled", which is why
// the second cache is separate.  With no control plane name set neither cache
// is scoped, so an agent that has not been told its control plane keeps
// working.

// CacheOptions returns the manager cache options that scope the agent to the
// ThreeportWorkloads labeled for the named control plane.
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

// LegacyCacheOptions returns the cache options for the ThreeportWorkloads that
// carry no control plane label.
func LegacyCacheOptions() cache.Options {
	unlabeled, err := labels.NewRequirement(
		agent.ControlPlaneNameLabelKey,
		selection.DoesNotExist,
		nil,
	)
	if err != nil {
		// the key is a constant and DoesNotExist takes no values, so this
		// cannot fail
		panic(fmt.Sprintf("invalid legacy workload selector: %s", err))
	}

	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&agentapi.ThreeportWorkload{}: {
				Label: labels.NewSelector().Add(*unlabeled),
			},
		},
	}
}

// NewLegacyWorkloadCluster returns a cluster whose cache holds only the
// ThreeportWorkloads with no control plane label, or nil when the agent has no
// control plane name and so is not scoped.  It is added to the manager so that
// its cache is started and synced before the controller starts.
func NewLegacyWorkloadCluster(
	config *rest.Config,
	scheme *runtime.Scheme,
	controlPlaneName string,
) (cluster.Cluster, error) {
	if controlPlaneName == "" {
		return nil, nil
	}

	legacy, err := cluster.New(config, func(o *cluster.Options) {
		o.Scheme = scheme
		o.Cache = LegacyCacheOptions()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster for unlabeled ThreeportWorkloads: %w", err)
	}

	return legacy, nil
}
