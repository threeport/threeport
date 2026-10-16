package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// The ports the database serves on.
const (
	cockroachSqlPort     = 26257
	cockroachConsolePort = 8080
)

// LabelMaintenance marks a pod that performs maintenance on the control
// plane's own data, which is the one thing the database policy opens up for.
const LabelMaintenance = "threeport.io/maintenance"

// InstallThreeportNetworkPolicies keeps the control plane database reachable
// only by the API server.
//
// The controllers hold cluster-admin on the cluster they run in, and the
// database holds every credential and every piece of state the control plane
// has. Nothing but the API server reads it - the controllers go through the
// API - so a foothold in any other pod had a direct path to all of it for no
// reason anyone had chosen.
//
// This is one policy and not a set. Two earlier attempts fenced the whole
// namespace with a default deny and an allow per path, and both broke things
// that were not visible from inside the namespace: the first denied a
// module's controller, which does not appear in any component list, and the
// second denied the module's API server and the proxy to it, because a module
// installs into a namespace of its own. The namespace is not a closed system,
// and a rule that assumes otherwise fails silently - the pod starts, reports
// ready, and cannot talk.
//
// Selecting the database alone cannot do that. A NetworkPolicy restricts only
// the pods it selects, so every other pod in the cluster is left exactly as
// it was, and the one restriction here is the one the issue was about.
//
// A NetworkPolicy is only enforced by a CNI that implements it. GKE creates
// the cluster with Calico, and kind enforces as well - the node image
// internal/provider/kind.go pins carries a kindnetd that runs
// kube-network-policies. On EKS and OKE it depends on the CNI the cluster was
// created with. The policy is installed everywhere regardless: an inert one
// costs nothing and is already correct when a cluster gains enforcement.
func (cpi *ControlPlaneInstaller) InstallThreeportNetworkPolicies(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	policy := cpi.databasePolicy()
	if err := cpi.CreateOrUpdateKubeResource(policy, kubeClient, mapper); err != nil {
		return fmt.Errorf(
			"failed to create network policy %s: %w",
			policy.GetName(), err,
		)
	}

	return nil
}

// databasePolicy keeps the database reachable only by the API server.
//
// Selecting the database for Ingress is what closes it: a pod a policy
// selects accepts only what that policy allows, and nothing else here selects
// it. Egress is deliberately not restricted - stopping the database from
// reaching out protects nothing that this does not already protect, and every
// direction named is a direction that can be wrong.
//
// Maintenance pods are the exception, and are labelled rather than named so
// the rule says what it is for. Dropping the database is the one operation
// that has to reach past the API server, because it runs while the API server
// is scaled to zero.
func (cpi *ControlPlaneInstaller) databasePolicy() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      "threeport-database",
				"namespace": cpi.Opts.Namespace,
			},
			"spec": map[string]interface{}{
				"podSelector": podsNamed("cockroachdb"),
				"policyTypes": []interface{}{"Ingress"},
				"ingress": []interface{}{
					map[string]interface{}{
						"from": []interface{}{
							map[string]interface{}{"podSelector": podsNamed(ThreeportAPIServiceResourceName)},
							// the nodes of the cluster itself, which replicate
							// and serve the console
							map[string]interface{}{"podSelector": podsNamed("cockroachdb")},
							map[string]interface{}{"podSelector": podsLabelled(LabelMaintenance, "true")},
						},
						"ports": []interface{}{
							tcpPort(cockroachSqlPort),
							tcpPort(cockroachConsolePort),
						},
					},
				},
			},
		},
	}
}

// podsNamed selects pods by their app.kubernetes.io/name label.
func podsNamed(names ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"matchExpressions": []interface{}{
			map[string]interface{}{
				"key":      "app.kubernetes.io/name",
				"operator": "In",
				"values":   names,
			},
		},
	}
}

// podsLabelled selects pods by one label.
func podsLabelled(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{key: value},
	}
}

func tcpPort(port int) map[string]interface{} {
	return map[string]interface{}{"protocol": "TCP", "port": int64(port)}
}
