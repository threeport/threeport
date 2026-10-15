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
	dnsPort              = 53
)

// Ports a Kubernetes API server is reached on. GKE and EKS publish 443; OKE
// publishes 6443. A remote runtime's endpoint is a provider address rather
// than a name a selector can describe, so the port is all there is to go on.
const (
	kubernetesApiPort    = 443
	kubernetesApiAltPort = 6443
)

// LabelMaintenance marks a pod that performs maintenance on the control
// plane's own data, which is the one thing the database policy opens up for.
const LabelMaintenance = "threeport.io/maintenance"

// InstallThreeportNetworkPolicies restricts traffic within the control plane
// namespace to the paths its components actually use.
//
// Without these every pod in the namespace can reach every other pod and the
// Kubernetes API. Three of the controllers hold cluster-admin on the cluster
// they run in, so a foothold in any pod - including ones that exist only to
// talk to the API server - reaches the database and those controllers directly.
//
// The namespace is treated as one unit rather than as a list of components.
// Anything running in it may reach the API server, the message broker and a
// Kubernetes API; nothing may reach the database except the API server. That
// is where the value is: the database holds every credential and every piece
// of state, and the components that hold cluster-admin have no route to it.
//
// The alternative was an allow rule per named component, which is tighter and
// wrong by construction: the namespace's population changes after install. A
// module brings its own controller, reinstall can enable a different set, and
// maintenance jobs appear and finish. A policy naming the components present
// at install denies every later arrival, and denies it silently - the pod
// starts, reports ready and cannot talk to anything.
//
// A NetworkPolicy is only enforced by a CNI that implements it, and two of
// the supported providers do. GKE creates the cluster with Calico. kind
// enforces as well: the node image internal/provider/kind.go pins carries a
// kindnetd that implements NetworkPolicy, so a local development cluster and
// the integration tests are both held to these rules - which is where the
// first version of this file was caught denying a module its own controller.
// On EKS and OKE it depends on the CNI the cluster was created with.
//
// They are installed everywhere regardless: an inert policy costs nothing and
// is already correct when a cluster gains enforcement, whereas one installed
// per provider is a second thing to remember.
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
	return []*unstructured.Unstructured{
		cpi.denyAllPolicy(),
		cpi.dnsEgressPolicy(),
		cpi.namespaceEgressPolicy(),
		cpi.apiServerPolicy(),
		cpi.databasePolicy(),
		cpi.messageBrokerPolicy(),
		cpi.maintenancePolicy(),
	}
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

// podsLabelled selects pods by one label.
func podsLabelled(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"matchLabels": map[string]interface{}{key: value},
	}
}

// everyPodInTheNamespace selects every pod alongside this one. An empty pod
// selector in a rule means the policy's own namespace, not the cluster.
func everyPodInTheNamespace() map[string]interface{} {
	return map[string]interface{}{}
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

// namespaceEgressPolicy is what every pod in the namespace is allowed to
// reach.
//
// It selects pods by being in the namespace rather than by name, so a
// controller added by a module, a controller enabled by a later reinstall and
// a maintenance job that did not exist at install time are all covered without
// anything being added here.
//
// The destinations are the two services the control plane talks to itself
// through and a Kubernetes API. The Kubernetes API has no address a selector
// can name - its endpoint is a node or a provider address - so it is expressed
// as the ports one is served on, which also covers the cloud provider APIs the
// infrastructure controllers call.
//
// The database is deliberately absent. Egress to it is denied here and ingress
// to it is denied by the database policy, so the restriction holds from both
// ends rather than resting on one rule.
func (cpi *ControlPlaneInstaller) namespaceEgressPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-namespace-egress", map[string]interface{}{
		"podSelector": map[string]interface{}{},
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
				"ports": []interface{}{
					tcpPort(kubernetesApiPort),
					tcpPort(kubernetesApiAltPort),
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
func (cpi *ControlPlaneInstaller) apiServerPolicy() *unstructured.Unstructured {
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
// to the data. It is the one place the namespace is not treated as a unit.
//
// Maintenance pods are the exception, and are labelled rather than named so
// the rule says what it is for. Dropping the database is the one operation
// that has to reach past the API server, because it runs while the API server
// is scaled to zero.
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
					map[string]interface{}{"podSelector": podsLabelled(LabelMaintenance, "true")},
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

// messageBrokerPolicy lets the namespace use NATS, and NATS nodes reach each
// other.
//
// The clients are every pod in the namespace rather than a list: the API
// server and the controllers are the expected ones, but a module's controller
// subscribes the same way, and so does the utility pod that exists to inspect
// the broker by hand. Naming them would have left each of those denied - and
// the egress half allowed, which reads as working until the connection is
// refused at this end.
func (cpi *ControlPlaneInstaller) messageBrokerPolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-message-broker", map[string]interface{}{
		"podSelector": podsNamed("nats"),
		"policyTypes": []interface{}{"Ingress", "Egress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{"podSelector": everyPodInTheNamespace()},
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

// maintenancePolicy gives the labelled maintenance pods the database egress
// the namespace policy withholds.
//
// Its counterpart is the ingress exception in databasePolicy; a path needs
// both ends and these are the two halves of the same decision.
func (cpi *ControlPlaneInstaller) maintenancePolicy() *unstructured.Unstructured {
	return cpi.networkPolicy("threeport-maintenance", map[string]interface{}{
		"podSelector": podsLabelled(LabelMaintenance, "true"),
		"policyTypes": []interface{}{"Egress"},
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
