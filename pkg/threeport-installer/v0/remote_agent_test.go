package v0

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// The agent on a managed cluster reports to a control plane on another cluster.
// What these cover is that it is told where that control plane is, that it is
// given the certificate it authenticates with, and that an agent installed
// without either is not mistaken for a current one.

const testExternalEndpoint = "34.1.2.3:443"

// agentDeployment builds an agent deployment running the image the installer
// would install, with the given args.
func agentDeployment(cpi *ControlPlaneInstaller, args ...string) *unstructured.Unstructured {
	agent := deployment(cpi.Opts.Namespace, ThreeportAgentDeployName, agentImageFor(cpi))

	rawArgs := make([]interface{}, 0, len(args))
	for _, arg := range args {
		rawArgs = append(rawArgs, arg)
	}
	containers, _, _ := unstructured.NestedSlice(agent.Object, "spec", "template", "spec", "containers")
	containers[0].(map[string]interface{})["args"] = rawArgs
	_ = unstructured.SetNestedSlice(agent.Object, containers, "spec", "template", "spec", "containers")

	return agent
}

// secret builds a secret with the given name.
func secret(namespace, name string, data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}
}

// remoteAgentInstaller returns an installer set up as the kubernetes runtime
// controller sets it up for a managed cluster.
func remoteAgentInstaller() *ControlPlaneInstaller {
	cpi := NewInstaller()
	cpi.Opts.AgentApiServer = testExternalEndpoint
	cpi.Opts.AuthEnabled = true
	cpi.Opts.CreateOrUpdateKubeResources = true

	return cpi
}

func argStrings(args []interface{}) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, arg.(string))
	}

	return out
}

func TestGetAgentArgs(t *testing.T) {
	t.Run("an agent in the control plane's own cluster keeps the default address", func(t *testing.T) {
		cpi := NewInstaller()
		cpi.Opts.AuthEnabled = true

		args := argStrings(cpi.getAgentArgs())
		for _, arg := range args {
			assert.NotContains(t, arg, "--threeport-api-server")
		}
		assert.NotContains(t, args, "--auth-enabled=false")
	})

	t.Run("an agent on a managed cluster is told where the control plane is", func(t *testing.T) {
		args := argStrings(remoteAgentInstaller().getAgentArgs())

		assert.Contains(t, args, "--threeport-api-server="+testExternalEndpoint)
		assert.NotContains(t, args, "--auth-enabled=false")
	})

	t.Run("an agent with auth off says so", func(t *testing.T) {
		cpi := remoteAgentInstaller()
		cpi.Opts.AuthEnabled = false

		assert.Contains(t, argStrings(cpi.getAgentArgs()), "--auth-enabled=false")
	})
}

func TestComputeSpaceAgentCommonName(t *testing.T) {
	assert.Equal(t, "threeport-agent-gke-01", ComputeSpaceAgentCommonName("gke-01"))
	assert.Equal(t, "threeport-agent", ComputeSpaceAgentCommonName(""))
}

func TestControllerConfigExternalApiEndpoint(t *testing.T) {
	t.Run("recorded when the API has an external address", func(t *testing.T) {
		cpi := NewInstaller()
		cpi.Opts.ExternalApiEndpoint = testExternalEndpoint

		data, _, err := unstructured.NestedStringMap(
			cpi.getControllerSecret("controller", cpi.Opts.Namespace).Object, "stringData",
		)
		require.NoError(t, err)
		assert.Equal(t, testExternalEndpoint, data[ControllerConfigExternalApiEndpointKey])
	})

	t.Run("absent when it has none", func(t *testing.T) {
		cpi := NewInstaller()

		data, _, err := unstructured.NestedStringMap(
			cpi.getControllerSecret("controller", cpi.Opts.Namespace).Object, "stringData",
		)
		require.NoError(t, err)
		assert.NotContains(t, data, ControllerConfigExternalApiEndpointKey)
	})
}

// TestSigningCaMount covers who is given the CA's private key: the controller
// that issues agent certificates, and no other.
func TestSigningCaMount(t *testing.T) {
	cpi := NewInstaller()
	cpi.Opts.AuthEnabled = true

	mountsSigningCa := func(controller string) bool {
		_, mounts, err := cpi.getControllerVolumes(controllerNamed(t, cpi, controller))
		require.NoError(t, err)
		for _, mount := range mounts {
			if mount.(map[string]interface{})["mountPath"] == SigningCaMountPath {
				return true
			}
		}

		return false
	}

	assert.True(t, mountsSigningCa(ThreeportKubernetesRuntimeControllerName))
	for _, controller := range ThreeportControllerList {
		if controller.Name == ThreeportKubernetesRuntimeControllerName {
			continue
		}
		assert.False(t, mountsSigningCa(controller.Name), "%s must not hold the CA key", controller.Name)
	}

	t.Run("not mounted when auth is off", func(t *testing.T) {
		noAuth := NewInstaller()
		_, mounts, err := noAuth.getControllerVolumes(controllerNamed(t, noAuth, ThreeportKubernetesRuntimeControllerName))
		require.NoError(t, err)
		assert.Empty(t, mounts)
	})

	t.Run("mounts the api-ca secret", func(t *testing.T) {
		vols, _, err := cpi.getControllerVolumes(controllerNamed(t, cpi, ThreeportKubernetesRuntimeControllerName))
		require.NoError(t, err)
		var found bool
		for _, vol := range vols {
			v := vol.(map[string]interface{})
			if v["name"] == signingCaVolumeName {
				found = true
				assert.Equal(t, ThreeportApiCaSecret, v["secret"].(map[string]interface{})["secretName"])
			}
		}
		assert.True(t, found)
	})
}

func controllerNamed(t *testing.T, cpi *ControlPlaneInstaller, name string) v0.ControlPlaneComponent {
	t.Helper()
	for _, c := range cpi.Opts.ControllerList {
		if c.Name == name {
			return *c
		}
	}
	require.Failf(t, "controller not in list", name)

	return v0.ControlPlaneComponent{}
}

func TestComputeSpaceAgentCurrent(t *testing.T) {
	t.Run("not current when the agent was installed without an address to report to", func(t *testing.T) {
		cpi := remoteAgentInstaller()
		// the args an agent installed before this change carries
		stale := agentDeployment(cpi, "--metrics-bind-address=127.0.0.1:8080", "--leader-elect", "--auth-enabled=false")

		client := testClient(append(
			[]runtime.Object{stale, secret(cpi.Opts.Namespace, agentCertSecretName, nil)},
			installedCRDs()...,
		)...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.False(t, current)
	})

	t.Run("current when the agent has the address and its certificate", func(t *testing.T) {
		cpi := remoteAgentInstaller()
		good := agentDeployment(cpi, argStrings(cpi.getAgentArgs())...)

		client := testClient(append(
			[]runtime.Object{good, secret(cpi.Opts.Namespace, agentCertSecretName, nil)},
			installedCRDs()...,
		)...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})

	t.Run("not current when auth is on and the agent has no certificate", func(t *testing.T) {
		cpi := remoteAgentInstaller()
		good := agentDeployment(cpi, argStrings(cpi.getAgentArgs())...)

		client := testClient(append([]runtime.Object{good}, installedCRDs()...)...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.False(t, current)
	})

	t.Run("the agent is not required when the install leaves it out", func(t *testing.T) {
		cpi := NewInstaller()
		cpi.Opts.SkipAgent = true

		client := testClient(installedCRDs()...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})

	t.Run("a create-only install does not compare args", func(t *testing.T) {
		cpi := NewInstaller()
		stale := agentDeployment(cpi, "--leader-elect")

		client := testClient(append([]runtime.Object{stale}, installedCRDs()...)...)

		current, err := cpi.ComputeSpaceControlPlaneComponentsCurrent(client, testMapper())
		require.NoError(t, err)
		assert.True(t, current)
	})
}

func TestInstalledExternalApiEndpoint(t *testing.T) {
	cpi := NewInstaller()

	t.Run("read from the controller config", func(t *testing.T) {
		client := testClient(secret(cpi.Opts.Namespace, "controller-config", map[string]interface{}{
			ControllerConfigExternalApiEndpointKey: base64.StdEncoding.EncodeToString([]byte(testExternalEndpoint)),
		}))

		endpoint, err := cpi.installedExternalApiEndpoint(client, testMapper())
		require.NoError(t, err)
		assert.Equal(t, testExternalEndpoint, endpoint)
	})

	t.Run("empty when the config records none", func(t *testing.T) {
		client := testClient(secret(cpi.Opts.Namespace, "controller-config", map[string]interface{}{}))

		endpoint, err := cpi.installedExternalApiEndpoint(client, testMapper())
		require.NoError(t, err)
		assert.Empty(t, endpoint)
	})

	t.Run("empty when there is no controller config", func(t *testing.T) {
		endpoint, err := cpi.installedExternalApiEndpoint(testClient(), testMapper())
		require.NoError(t, err)
		assert.Empty(t, endpoint)
	})
}
