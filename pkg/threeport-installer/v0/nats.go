package v0

import (
	"context"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// The resources of the message broker's containers.  The NATS server measured
// 6Mi at rest, 15Mi holding 13 streams and a lock bucket with 39,000
// messages, and 199Mi at its peak with a consumer reading each of those
// streams at once, far more traffic than a control plane makes.  The config
// reloader measured 4Mi and the metrics exporter 16Mi.
const (
	natsServerRequestCPU    = "50m"
	natsServerRequestMemory = "64Mi"
	natsServerLimitCPU      = "1"
	natsServerLimitMemory   = "512Mi"

	// natsServerGoMemLimit is 90% of the server's memory limit, the share the
	// NATS Helm chart gives the Go runtime so it collects garbage before the
	// container is killed for running out of memory.
	natsServerGoMemLimit = "460MiB"

	natsReloaderRequestCPU    = "5m"
	natsReloaderRequestMemory = "16Mi"
	natsReloaderLimitCPU      = "100m"
	natsReloaderLimitMemory   = "64Mi"

	natsExporterRequestCPU    = "5m"
	natsExporterRequestMemory = "32Mi"
	natsExporterLimitCPU      = "100m"
	natsExporterLimitMemory   = "128Mi"

	// nats-box idles until an operator runs the nats CLI in it
	natsBoxRequestCPU    = "5m"
	natsBoxRequestMemory = "16Mi"
	natsBoxLimitCPU      = "200m"
	natsBoxLimitMemory   = "128Mi"

	// natsLivenessPath is the health check the liveness probe uses: whether
	// JetStream is enabled, without the full check's wait for every stream
	// to be current, which fails while a restarted server catches up.
	natsLivenessPath = "/healthz?js-enabled-only=true"
)

// natsServerResources returns the NATS server container's resources.
func natsServerResources() map[string]interface{} {
	return containerResources(natsServerRequestCPU, natsServerRequestMemory, natsServerLimitCPU, natsServerLimitMemory)
}

// natsReloaderResources returns the config reloader container's resources.
func natsReloaderResources() map[string]interface{} {
	return containerResources(natsReloaderRequestCPU, natsReloaderRequestMemory, natsReloaderLimitCPU, natsReloaderLimitMemory)
}

// natsExporterResources returns the metrics exporter container's resources.
func natsExporterResources() map[string]interface{} {
	return containerResources(natsExporterRequestCPU, natsExporterRequestMemory, natsExporterLimitCPU, natsExporterLimitMemory)
}

// natsBoxResources returns the nats-box container's resources.
func natsBoxResources() map[string]interface{} {
	return containerResources(natsBoxRequestCPU, natsBoxRequestMemory, natsBoxLimitCPU, natsBoxLimitMemory)
}

// containerResources returns a container's resource requests and limits.
func containerResources(requestCPU, requestMemory, limitCPU, limitMemory string) map[string]interface{} {
	return map[string]interface{}{
		"requests": map[string]interface{}{
			"cpu":    requestCPU,
			"memory": requestMemory,
		},
		"limits": map[string]interface{}{
			"cpu":    limitCPU,
			"memory": limitMemory,
		},
	}
}

// The cluster the message broker's servers form on the production tier.
const (
	// natsClusterName names the cluster the servers form.
	natsClusterName = "threeport"

	// natsClusterPort is the port the servers route to each other on.
	natsClusterPort = 6222
)

// natsReplicas returns how many message broker servers a given tier runs.
//
// Development stays at the single server it has always been.  Production
// runs three, the fewest JetStream's Raft groups need to keep a quorum, and
// with it every stream and the lock bucket, through the loss of one.
func natsReplicas(tier ControlPlaneTier) int64 {
	if string(tier) == ControlPlaneTierProd {
		return 3
	}

	return 1
}

// natsClusterConfig returns the part of the server's config that joins the
// servers of a production broker into one cluster, or nothing for a single
// server.
//
// Each server routes to every pod of the stateful set by its stable name in
// the headless service, which publishes not-ready pods so the servers find
// each other while they start.  The routes use TLS with the server's own
// certificate, which covers every pod's name, and NATS verifies the
// certificate on both ends of a route, so only a server holding a
// certificate from the broker's CA can join.  The servers don't advertise
// their pod addresses to clients, which keep connecting through the service
// name their certificate is verified against.
func natsClusterConfig(namespace string, replicas int64) string {
	if replicas < 2 {
		return ""
	}

	var routes strings.Builder
	for i := int64(0); i < replicas; i++ {
		fmt.Fprintf(
			&routes,
			"    nats://%s-%d.%s.%s.svc.cluster.local:%d\n",
			natsServiceName, i, natsServiceName, namespace, natsClusterPort,
		)
	}

	return fmt.Sprintf(`
###############
#             #
# Cluster     #
#             #
###############
cluster {
  name: %s
  port: %d
  routes: [
%s  ]
  no_advertise: true
  connect_retries: 120
  tls {
    cert_file: "%s/tls.crt"
    key_file: "%s/tls.key"
    ca_file: "%s/ca.crt"
    timeout: 2
  }
}
`,
		natsClusterName, natsClusterPort, routes.String(),
		natsServerCertsPath, natsServerCertsPath, natsServerCertsPath,
	)
}

// natsAntiAffinityLabels is what natsAntiAffinity selects: every message
// broker server.
var natsAntiAffinityLabels = map[string]interface{}{
	"app.kubernetes.io/name":     "nats",
	"app.kubernetes.io/instance": "nats-js",
}

// natsAntiAffinity requires the servers of a clustered broker to run on
// separate nodes, or nil for a single server, which keeps the scheduling it
// has always had.
//
// It is a requirement rather than a preference for the same reason as the
// database's: two servers sharing a node lose the quorum together with it,
// and nothing shows that until the node goes.  A server that can't be
// placed stays Pending instead, which names the three schedulable nodes a
// production control plane needs.
func natsAntiAffinity(replicas int64) map[string]interface{} {
	if replicas < 2 {
		return nil
	}

	return map[string]interface{}{
		"podAntiAffinity": map[string]interface{}{
			"requiredDuringSchedulingIgnoredDuringExecution": []interface{}{
				map[string]interface{}{
					"topologyKey": "kubernetes.io/hostname",
					"labelSelector": map[string]interface{}{
						"matchLabels": natsAntiAffinityLabels,
					},
				},
			},
		},
	}
}

// messageBrokerReplicas returns how many servers the message broker runs:
// the stateful set's count when it exists, and the tier's otherwise.  The
// server's config and the clients' streams and lock bucket follow it, so a
// production broker installed with one server, before production ran
// three, keeps a config and streams that one server can hold, since a
// reinstall leaves the stateful set's count as it is.
func (cpi *ControlPlaneInstaller) messageBrokerReplicas(kubeClient dynamic.Interface) (int64, error) {
	statefulSet, err := kubeClient.Resource(statefulSetGVR).Namespace(cpi.Opts.Namespace).Get(
		context.Background(), natsServiceName, metav1.GetOptions{},
	)
	if k8serrors.IsNotFound(err) {
		return natsReplicas(cpi.Opts.Tier), nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get the message broker's stateful set: %w", err)
	}

	replicas, found, err := unstructured.NestedInt64(statefulSet.Object, "spec", "replicas")
	if err != nil {
		return 0, fmt.Errorf("failed to read the message broker's replicas: %w", err)
	}
	if !found {
		// Kubernetes defaults an unset count to one
		return 1, nil
	}

	return replicas, nil
}
