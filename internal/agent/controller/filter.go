package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/threeport/threeport/internal/agent"
)

// ControlPlanePredicate returns a predicate that scopes the agent to the
// ThreeportWorkloads of the named control plane.  ThreeportWorkload is
// cluster-scoped, so without this every agent on a cluster would reconcile
// every control plane's workloads.
//
// A ThreeportWorkload with no control plane label is let through.  Those were
// created before the label existed, they carry the agent's finalizer, and an
// agent that cannot see one can never remove that finalizer - the resource
// would stay Terminating after its workload is deleted.  Nothing records which
// control plane owns an unlabeled resource (the workload instance IDs it holds
// are only unique within one control plane's database), so no agent can claim
// one safely; they are handled as they were before this filter, by whichever
// agent sees them, until the workload instance is created again and the
// resource is replaced with a labeled one.
//
// An empty name admits everything, so an agent that has not been told its
// control plane keeps working.
func ControlPlanePredicate(controlPlaneName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		if controlPlaneName == "" {
			return true
		}

		owner, labeled := object.GetLabels()[agent.ControlPlaneNameLabelKey]
		return !labeled || owner == controlPlaneName
	})
}
