package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestBlueGreenControlPlaneCopiesTheClientEntry covers the client entry recorded
// once a restored control plane answers.
func TestBlueGreenControlPlaneCopiesTheClientEntry(t *testing.T) {
	// set up a source entry with two credentials and a cleanup record
	source := &ControlPlane{
		Name:          "source",
		AuthEnabled:   true,
		Genesis:       true,
		APIServer:     "10.0.0.1:443",
		CACert:        "ca-cert",
		EncryptionKey: "key",
		Provider:      "gke",
		GKEProviderConfig: GKEProviderConfig{
			GcpProjectId: "project",
			BackupBucket: "bucket",
		},
		KubeAPI: KubeAPI{APIEndpoint: "https://cluster"},
		Credentials: []Credential{
			{Name: "source", ClientCert: "cert", ClientKey: "private"},
			{Name: "other", ClientCert: "other-cert"},
		},
		PendingGcpCleanup: []PendingGcpCleanup{{Name: "account", ProjectID: "project"}},
	}

	// copy onto the new name and address
	copied := blueGreenControlPlane(source, "restored", "10.0.0.2:443")

	// the copy names the new control plane and keeps the source material
	require.Equal(t, "restored", copied.Name)
	require.Equal(t, "10.0.0.2:443", copied.APIServer)
	require.True(t, copied.Genesis)
	require.Equal(t, "ca-cert", copied.CACert)
	require.Equal(t, "key", copied.EncryptionKey)
	require.Equal(t, "gke", copied.Provider)
	require.Equal(t, source.GKEProviderConfig, copied.GKEProviderConfig)
	require.Equal(t, source.KubeAPI, copied.KubeAPI)
	require.Equal(t, "restored", copied.Credentials[0].Name)
	require.Equal(t, "cert", copied.Credentials[0].ClientCert)
	require.Equal(t, "private", copied.Credentials[0].ClientKey)
	require.Equal(t, "other", copied.Credentials[1].Name)
	require.Equal(t, "other-cert", copied.Credentials[1].ClientCert)
	require.Equal(t, source.PendingGcpCleanup, copied.PendingGcpCleanup)

	// renaming the copy does not change the source
	require.Equal(t, "source", source.Name)
	require.Equal(t, "source", source.Credentials[0].Name)
	copied.PendingGcpCleanup[0].Name = "changed"
	require.Equal(t, "account", source.PendingGcpCleanup[0].Name)
	copied.Credentials[0].ClientCert = "replaced"
	require.Equal(t, "cert", source.Credentials[0].ClientCert)

	// an unknown address keeps the source address
	kept := blueGreenControlPlane(source, "restored", "")
	require.Equal(t, "10.0.0.1:443", kept.APIServer)
}

// TestAPIHostDropsTheSchemeAndPort covers the address compared with a load balancer.
func TestAPIHostDropsTheSchemeAndPort(t *testing.T) {
	require.Equal(t, "10.0.0.1", apiHost("https://10.0.0.1:443"))
	require.Equal(t, "10.0.0.1", apiHost("10.0.0.1:443"))
	require.Equal(t, "api.example.com", apiHost("api.example.com"))
	require.Equal(t, "", apiHost(""))
}

// TestNamespaceForAPIHostUsesTheLoadBalancer covers the namespace a backup must use.
func TestNamespaceForAPIHostUsesTheLoadBalancer(t *testing.T) {
	// two control planes publish different addresses
	services := []unstructured.Unstructured{
		serviceWithAddress("genesis-02", "10.0.0.1"),
		serviceWithAddress("genesis-03", "10.0.0.2"),
	}

	// the address selects its namespace
	found, err := namespaceForAPIHost(services, "10.0.0.2")
	require.NoError(t, err)
	require.Equal(t, "genesis-03", found)

	// an unknown address selects nothing
	found, err = namespaceForAPIHost(services, "10.0.0.9")
	require.NoError(t, err)
	require.Equal(t, "", found)

	// one address in two namespaces is refused
	_, err = namespaceForAPIHost([]unstructured.Unstructured{
		serviceWithAddress("a", "10.0.0.2"),
		serviceWithAddress("b", "10.0.0.2"),
	}, "10.0.0.2")
	require.Error(t, err)
}

// serviceWithAddress builds a service whose load balancer publishes ip.
func serviceWithAddress(namespace, ip string) unstructured.Unstructured {
	// publish the address from namespace
	return unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"namespace": namespace},
		"status": map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"ingress": []interface{}{
					map[string]interface{}{"ip": ip},
				},
			},
		},
	}}
}
