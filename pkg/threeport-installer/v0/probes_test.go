package v0

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// threeportContainers returns the container that runs Threeport's own code in
// each Deployment, keyed by Deployment name - the one container every probe in
// this file is about.  The agent's kube-rbac-proxy sidecar is not one.
func threeportContainers(t *testing.T, cpi *ControlPlaneInstaller) map[string]map[string]interface{} {
	t.Helper()

	containers := map[string]map[string]interface{}{}
	for name, deployment := range controlPlaneDeployments(t, cpi) {
		pod := podContainers(t, deployment)
		switch name {
		case ThreeportAPIServiceResourceName:
			containers[name] = pod["api-server"]
		case ThreeportAgentDeployName:
			containers[name] = pod["manager"]
		default:
			containers[name] = pod[strings.TrimPrefix(name, "threeport-")]
		}
		require.NotNil(t, containers[name], "no Threeport container found in %s", name)
	}

	return containers
}

// probeInt returns an integer field of a probe.
func probeInt(t *testing.T, probe map[string]interface{}, field string) int64 {
	t.Helper()

	value, found, err := unstructured.NestedInt64(probe, field)
	require.NoError(t, err)
	require.True(t, found, "probe has no %s", field)

	return value
}

// TestEveryThreeportContainerHasStartupLivenessAndReadiness covers the outage
// the issue describes: without a liveness probe a process that hangs without
// crashing is marked not ready forever and never restarted.
func TestEveryThreeportContainerHasStartupLivenessAndReadiness(t *testing.T) {
	for name, container := range threeportContainers(t, testInstaller()) {
		for _, probe := range []string{"startupProbe", "livenessProbe", "readinessProbe"} {
			_, found, err := unstructured.NestedMap(container, probe)
			require.NoError(t, err)
			assert.True(t, found, "%s has no %s", name, probe)
		}
	}
}

// TestLivenessIsMoreLenientThanReadiness covers the order the two have to fail
// in.  A pod that stops answering is first taken out of its Service and only
// restarted if it stays that way - the reverse would restart a pod that a
// short pause had only slowed down.  Readiness itself must ride out a single
// slow answer, which a failure threshold of one did not.
func TestLivenessIsMoreLenientThanReadiness(t *testing.T) {
	for name, container := range threeportContainers(t, testInstaller()) {
		readiness := container["readinessProbe"].(map[string]interface{})
		liveness := container["livenessProbe"].(map[string]interface{})

		assert.Greater(t, probeInt(t, readiness, "failureThreshold"), int64(1), "%s readiness drops on one slow answer", name)

		livenessWindow := probeInt(t, liveness, "failureThreshold") * probeInt(t, liveness, "periodSeconds")
		readinessWindow := probeInt(t, readiness, "failureThreshold") * probeInt(t, readiness, "periodSeconds")
		assert.Greater(t, livenessWindow, readinessWindow, "%s restarts before it is taken out of service", name)
	}
}

// TestLivenessDoesNotProbeReadiness covers why liveness has its own endpoint.
// A controller is not ready while its NATS subscriptions are down, so a
// liveness probe on readiness would restart every controller at once during a
// NATS outage, which a restart cannot fix.
func TestLivenessDoesNotProbeReadiness(t *testing.T) {
	for name, container := range threeportContainers(t, testInstaller()) {
		for _, probe := range []string{"startupProbe", "livenessProbe"} {
			path, _, err := unstructured.NestedString(container, probe, "httpGet", "path")
			require.NoError(t, err)
			assert.NotEqual(t, "/readyz", path, "%s %s probes readiness", name, probe)
		}
	}
}

// TestEveryProbedEndpointIsServed covers the one way these probes could take
// the whole control plane down: a probe on a path its component does not serve
// fails forever, and a failing liveness probe restarts the pod forever.  The
// endpoints live in each component's main package, so this reads them there.
func TestEveryProbedEndpointIsServed(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	served := func(t *testing.T, path, handler string) {
		t.Helper()
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(source), handler, "%s does not serve the probed endpoint", path)
	}

	// the API server and every controller serve /livez from their generated main
	served(t, filepath.Join(root, "cmd", "rest-api", "main_gen.go"), `HandleFunc("`+componentLivenessPath+`"`)
	for _, controller := range ThreeportControllerList {
		served(t, filepath.Join(root, "cmd", controller.Name, "main_gen.go"), `HandleFunc("`+componentLivenessPath+`"`)
	}

	// controller-runtime serves a healthz check registered as "healthz" on /healthz
	assert.Equal(t, "/healthz", agentLivenessPath)
	served(t, filepath.Join(root, "cmd", "agent", "main.go"), `AddHealthzCheck("healthz"`)

	// and the probes ask for the port those servers listen on
	for name, container := range threeportContainers(t, testInstaller()) {
		for _, probe := range []string{"startupProbe", "livenessProbe", "readinessProbe"} {
			port, _, err := unstructured.NestedInt64(container, probe, "httpGet", "port")
			require.NoError(t, err)
			assert.Equal(t, int64(8081), port, "%s %s", name, probe)
		}
	}
	served(t, filepath.Join(root, "cmd", "rest-api", "main_gen.go"), `ListenAndServe(":8081"`)
	served(t, filepath.Join(root, "cmd", "agent", "main.go"), `"health-probe-bind-address", ":8081"`)
}
