package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// threeportClient returns the api client for the local control plane.
func threeportClient(t *testing.T) (*http.Client, string, string) {
	t.Helper()

	// read the current control plane from the local config
	cli.InitConfig(nil, "")
	threeportConfig, _, err := cli.GetThreeportConfig("")
	require.NoError(t, err)
	apiClient, err := threeportConfig.GetHTTPClient(threeportConfig.CurrentControlPlane)
	require.NoError(t, err)
	controlPlane, err := threeportConfig.GetControlPlaneConfig(threeportConfig.CurrentControlPlane)
	require.NoError(t, err)
	key, err := threeportConfig.GetThreeportEncryptionKey(threeportConfig.CurrentControlPlane)
	require.NoError(t, err)
	return apiClient, controlPlane.APIServer, key
}

// controlPlaneRuntime returns the runtime that hosts the control plane.
func controlPlaneRuntime(t *testing.T, apiClient *http.Client, endpoint string) *api.KubernetesRuntimeInstance {
	t.Helper()

	// find the runtime marked as the control plane host
	runtimes, err := client.GetKubernetesRuntimeInstances(apiClient, endpoint)
	require.NoError(t, err)
	for _, runtime := range *runtimes {
		if runtime.ThreeportControlPlaneHost != nil && *runtime.ThreeportControlPlaneHost {
			found := runtime
			return &found
		}
	}
	t.Fatal("no control plane runtime")
	return nil
}

// waitForReconciled retries until read returns a reconciled flag.
func waitForReconciled(t *testing.T, label string, read func() (*bool, error)) {
	t.Helper()

	// read the object until its controller marks it reconciled
	err := util.Retry(reconcileAttempts, 2, func() error {
		reconciled, err := read()
		if err != nil {
			return err
		}
		if reconciled == nil || !*reconciled {
			return fmt.Errorf("%s is not reconciled", label)
		}
		return nil
	})
	require.NoError(t, err)
}
