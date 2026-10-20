package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// productionComponentReplicas is how many replicas the API server and each
// controller run on the production tier, so losing a pod or its node leaves
// another one serving.  Controllers lock each object in NATS before they
// reconcile it, so their replicas share the work rather than repeat it.
const productionComponentReplicas = 2

// componentReplicas returns how many replicas a component runs: the configured
// count when there is one, and otherwise the default for the tier.  A tier that
// is not production - including none at all - runs one.
func (cpi *ControlPlaneInstaller) componentReplicas(configured int) int64 {
	if configured > 0 {
		return int64(configured)
	}
	if cpi.Opts.Tier == ControlPlaneTier(ControlPlaneTierProd) {
		return productionComponentReplicas
	}

	return 1
}

// componentAntiAffinity returns an affinity that spreads a Deployment's pods
// across nodes when it can, since two replicas on one node do not survive the
// loss of that node.  It is a preference rather than a requirement, so a small
// cluster can still schedule every replica.
func componentAntiAffinity(deployName string) map[string]interface{} {
	return map[string]interface{}{
		"podAntiAffinity": map[string]interface{}{
			"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
				map[string]interface{}{
					"weight": int64(100),
					"podAffinityTerm": map[string]interface{}{
						"topologyKey": "kubernetes.io/hostname",
						"labelSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.kubernetes.io/name": deployName,
							},
						},
					},
				},
			},
		},
	}
}

// componentDisruptionBudget returns a PodDisruptionBudget that keeps one pod of
// a Deployment running through a voluntary disruption such as a node drain.
func (cpi *ControlPlaneInstaller) componentDisruptionBudget(deployName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]interface{}{
				"name":      deployName,
				"namespace": cpi.Opts.Namespace,
			},
			"spec": map[string]interface{}{
				"minAvailable": int64(1),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/name": deployName,
					},
				},
			},
		},
	}
}

// applyComponentDisruptionBudget creates the budget for a Deployment that runs
// more than one replica and removes it from one that runs a single replica.
// With one replica, a budget of one available pod can never be met while that
// pod is evicted, so a node drain would wait on it forever.
func (cpi *ControlPlaneInstaller) applyComponentDisruptionBudget(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
	deployName string,
	replicas int64,
) error {
	budget := cpi.componentDisruptionBudget(deployName)

	if replicas > 1 {
		if err := cpi.CreateOrUpdateKubeResource(budget, kubeClient, mapper); err != nil {
			return fmt.Errorf("failed to create pod disruption budget for %s: %w", deployName, err)
		}
		return nil
	}

	if err := kube.DeleteResource(budget, kubeClient, *mapper); err != nil {
		return fmt.Errorf("failed to remove pod disruption budget for %s: %w", deployName, err)
	}

	return nil
}
