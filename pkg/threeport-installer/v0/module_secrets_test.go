package v0

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const testModuleNamespace = "widget-module"

// testSecret returns a secret holding env in its data, as the cluster stores
// it.
func testSecret(namespace, env string, labels map[string]string) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "Opaque",
		"metadata": map[string]interface{}{
			"name":      "controller-config",
			"namespace": namespace,
		},
		"data": map[string]interface{}{
			"env": base64.StdEncoding.EncodeToString([]byte(env)),
		},
	}}
	secret.SetLabels(labels)

	return secret
}

func TestCopySecretCreatesAModulesCopy(t *testing.T) {
	_, kubeClient, _ := newBrokerTestClient(
		testSecret(testControlPlaneNamespace, "MSG_BROKER_PASSWORD=tpSecret", map[string]string{"app": "threeport"}),
	)

	require.NoError(t, CopySecret(kubeClient, *installerTestMapper(), "controller-config", testControlPlaneNamespace, testModuleNamespace))

	copied, err := kubeClient.Resource(secretGVR).Namespace(testModuleNamespace).Get(context.Background(), "controller-config", metav1.GetOptions{})
	require.NoError(t, err)
	value, err := secretValue(copied, "env")
	require.NoError(t, err)
	assert.Equal(t, "MSG_BROKER_PASSWORD=tpSecret", value)
	assert.Equal(t, "Opaque", copied.Object["type"])
	assert.Equal(t, map[string]string{"app": "threeport"}, copied.GetLabels())
	assert.Equal(t, testModuleNamespace, copied.GetNamespace())
}

func TestCopySecretRefreshesAModulesOldCopy(t *testing.T) {
	// a module installed before the broker required TLS holds a copy
	// without the broker's password
	_, kubeClient, _ := newBrokerTestClient(
		testSecret(testControlPlaneNamespace, "MSG_BROKER_PASSWORD=tpSecret", nil),
		testSecret(testModuleNamespace, "MSG_BROKER_HOST=nats-js", nil),
	)

	require.NoError(t, CopySecret(kubeClient, *installerTestMapper(), "controller-config", testControlPlaneNamespace, testModuleNamespace))

	copied, err := kubeClient.Resource(secretGVR).Namespace(testModuleNamespace).Get(context.Background(), "controller-config", metav1.GetOptions{})
	require.NoError(t, err)
	value, err := secretValue(copied, "env")
	require.NoError(t, err)
	assert.Equal(t, "MSG_BROKER_PASSWORD=tpSecret", value)

	// the control plane's own secret is left as it is
	source, err := kubeClient.Resource(secretGVR).Namespace(testControlPlaneNamespace).Get(context.Background(), "controller-config", metav1.GetOptions{})
	require.NoError(t, err)
	value, err = secretValue(source, "env")
	require.NoError(t, err)
	assert.Equal(t, "MSG_BROKER_PASSWORD=tpSecret", value)
}

func TestCopySecretFailsWithoutTheControlPlanesSecret(t *testing.T) {
	_, kubeClient, _ := newBrokerTestClient()

	err := CopySecret(kubeClient, *installerTestMapper(), "controller-config", testControlPlaneNamespace, testModuleNamespace)
	assert.ErrorContains(t, err, "failed to get Secret 'controller-config' from namespace")
}
