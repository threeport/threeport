package v0

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// Ports the control plane's own components listen on.
const (
	cockroachSqlPort     = 26257
	cockroachConsolePort = 8080
	natsClientPort       = 4222
	natsClusterPort      = 6222
	threeportApiPort     = 1323
	kubernetesApiPort    = 443
	dnsPort              = 53
)

// InstallThreeportNetworkPolicies restricts traffic within the control plane
// namespace to the paths its components actually use.
//
// Without these every pod in the namespace can reach every other pod and the
// Kubernetes API. Three of the controllers hold cluster-admin on the cluster
// they run in, so a foothold in any pod - including ones that exist only to
// talk to the API server - reaches the database and those controllers directly.
//
// A NetworkPolicy is only enforced by a CNI that implements it. Of the
// supported providers, GKE does today, because the cluster is created with
// Calico enabled. On EKS, OKE and kind these objects are accepted and stored
// and nothing acts on them. They are installed everywhere regardless: an inert
// policy costs nothing and is already correct when a cluster gains enforcement,
// whereas one installed per provider is a second thing to remember.
func (cpi *ControlPlaneInstaller) InstallThreeportNetworkPolicies(
	kubeClient dynamic.Interface,
	mapper *meta.RESTMapper,
) error {
	for _, policy := range cpi.threeportNetworkPolicies() {
		if err := cpi.CreateOrUpdateKubeResource(policy, kubeClient, mapper); err != nil {
			return fmt.Errorf(
				"failed to create network policy %s: %w",
				policy.GetName(), err,
			)
		}
	}

	return nil
}

// threeportNetworkPolicies returns every policy the control plane namespace
// gets, in the order they are applied.
func (cpi *ControlPlaneInstaller) threeportNetworkPolicies() []*unstructured.Unstructured {
	controllers := cpi.controllerPodNames()

	return []*unstructured.Unstructured{
		cpi.denyAllPolicy(),
		cpi.dnsEgressPolicy(),
		cpi.apiServerPolicy(controllers),
		cpi.databasePolicy(),
		cpi.messageBrokerPolicy(controllers),
		cpi.controllerPolicy(controllers),
		cpi.supportServicesOperatorPolicy(),
		cpi.messageBrokerUtilityPolicy(),
	}
}

// controllerPodNames returns the app.kubernetes.io/name value of every
// controller this install deploys, plus the agent, which talks to the API
// server the same way.
func (cpi *ControlPlaneInstaller) controllerPodNames() []interface{} {
	names := []interface{}{ThreeportAgentDeployName}
	for _, controller := range cpi.Opts.ControllerList {
		if controller == nil {
			continue
		}
		names = append(names, cpi.ControllerDeployName(*controller))
	}

	return names
}

// networkPolicy wraps the boilerplate every policy here shares.
func (cpi *ControlPlaneInstaller) networkPolicy(
	name string,
	spec map[string]interface{},
) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cpi.Opts.Namespace,
			},
			"spec": spec,
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

// podsLabelled selects pods by a label other than app.kubernetes.io/name,
// which not everything in the namespace carries.
func podsLabelled(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{key: value},
	}
}

func tcpPort(port int) map[string]interface{} {
	return map[string]interface{}{"protocol": "TCP", "port": int64(port)}
}

// denyAllPolicy is what makes the rest of these mean anything.
//
// A NetworkPolicy only ever adds permission: a pod selected by none is
// unrestricted. Without a policy selecting every pod for both directions, the
// allow rules below would describe paths that were open anyway.
func (cpi *ControlPlaneInstaller) denyAllPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-default-deny", map[string]interface{}{
		"podSelector": map[string]interface{}{},
		"policyTypes": []interface{}{"Ingress", "Egress"},
	})
}

// dnsEgressPolicy lets every pod resolve names.
//
// Every other rule here names a service, and a service name has to be resolved
// before it can be reached, so denying DNS would deny all of them.
func (cpi *ControlPlaneInstaller) dnsEgressPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-allow-dns", map[string]interface{}{
		"podSelector": map[string]interface{}{},
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{
						"namespaceSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"kubernetes.io/metadata.name": "kube-system",
							},
						},
					},
				},
				"ports": []interface{}{
					map[string]interface{}{"protocol": "UDP", "port": int64(dnsPort)},
					tcpPort(dnsPort),
				},
			},
		},
	})
}

// apiServerPolicy covers the one component reached from outside the cluster.
//
// Ingress is left open to any source because tptctl, and a parent control
// plane managing this one, connect from outside the cluster - there is no
// selector that describes them. Authentication is what restricts that path;
// this restricts where the API server can reach, which is the database and the
// message broker and nothing else.
func (cpi *ControlPlaneInstaller) apiServerPolicy(controllers []interface{}) *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-api-server", map[string]interface{}{
		"podSelector": podsNamed(ThreeportAPIServiceResourceName),
		"policyTypes": []interface{}{"Ingress", "Egress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"ports": []interface{}{tcpPort(threeportApiPort)},
			},
		},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("cockroachdb")},
				},
				"ports": []interface{}{tcpPort(cockroachSqlPort)},
			},
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("nats")},
				},
				"ports": []interface{}{tcpPort(natsClientPort)},
			},
		},
	})
}

// databasePolicy keeps the database reachable only by the API server.
//
// No controller talks to it directly - they go through the API server - so
// this is the rule that matters most: a foothold in any other pod has no path
// to the data.
func (cpi *ControlPlaneInstaller) databasePolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-database", map[string]interface{}{
		"podSelector": podsNamed("cockroachdb"),
		"policyTypes": []interface{}{"Ingress", "Egress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{"podSelector": podsNamed(ThreeportAPIServiceResourceName)},
					// the nodes of the cluster itself, which replicate and
					// serve the console
					map[string]interface{}{"podSelector": podsNamed("cockroachdb")},
				},
				"ports": []interface{}{tcpPort(cockroachSqlPort), tcpPort(cockroachConsolePort)},
			},
		},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("cockroachdb")},
				},
				"ports": []interface{}{tcpPort(cockroachSqlPort)},
			},
		},
	})
}

// messageBrokerPolicy lets the API server and the controllers use NATS, and
// NATS nodes reach each other.
func (cpi *ControlPlaneInstaller) messageBrokerPolicy(controllers []interface{}) *unstructured.Unstructured {
	clients := append([]interface{}{ThreeportAPIServiceResourceName}, controllers...)

	return cpi.networkPolicy("threeport-message-broker", map[string]interface{}{
		"podSelector": podsNamed("nats"),
		"policyTypes": []interface{}{"Ingress", "Egress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{"podSelector": podsNamed(clients...)},
				},
				"ports": []interface{}{tcpPort(natsClientPort)},
			},
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("nats")},
				},
				"ports": []interface{}{tcpPort(natsClusterPort)},
			},
		},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("nats")},
				},
				"ports": []interface{}{tcpPort(natsClusterPort)},
			},
		},
	})
}

// controllerPolicy covers what a controller is allowed to reach.
//
// The API server and the message broker are in-cluster and named. The two
// outside destinations are the Kubernetes API, which the controllers deploying
// workloads need, and the cloud provider APIs, which the infrastructure
// controllers call. Neither has a stable address that a selector can name - the
// Kubernetes API's endpoint is a node address, and a cloud API is a public one -
// so both are expressed as egress to 443, with the cluster's own pod and
// service ranges left to the rules above.
//
// Controllers accept no connections, so there is no ingress rule: the
// default-deny policy already covers that, and saying it again here would read
// as a decision taken twice.
func (cpi *ControlPlaneInstaller) controllerPolicy(controllers []interface{}) *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-controllers", map[string]interface{}{
		"podSelector": podsNamed(controllers...),
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed(ThreeportAPIServiceResourceName)},
				},
				"ports": []interface{}{tcpPort(threeportApiPort)},
			},
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("nats")},
				},
				"ports": []interface{}{tcpPort(natsClientPort)},
			},
			map[string]interface{}{
				"ports": []interface{}{tcpPort(kubernetesApiPort)},
			},
		},
	})
}

// supportServicesOperatorPolicy covers the operator installed alongside the
// control plane.
//
// It carries none of the labels the policies above select on, so the
// default-deny would leave it with no network at all - and it reconciles the
// support services a gateway depends on, so that failure would show up as
// gateways never becoming ready rather than as anything about networking.
func (cpi *ControlPlaneInstaller) supportServicesOperatorPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-support-services-operator", map[string]interface{}{
		"podSelector": podsLabelled("control-plane", "controller-manager"),
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"ports": []interface{}{tcpPort(kubernetesApiPort)},
			},
		},
	})
}

// messageBrokerUtilityPolicy covers the NATS utility pod.
//
// It exists to inspect the broker by hand, which is the one thing a default
// deny would stop it doing, and it is reached for when something is already
// wrong.
func (cpi *ControlPlaneInstaller) messageBrokerUtilityPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-message-broker-utility", map[string]interface{}{
		"podSelector": podsLabelled("app", "nats-js-box"),
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{"podSelector": podsNamed("nats")},
				},
				"ports": []interface{}{tcpPort(natsClientPort)},
			},
		},
	})
}
