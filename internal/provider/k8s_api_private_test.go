package provider

import (
	"encoding/base64"
	"testing"

	"cloud.google.com/go/container/apiv1/containerpb"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gkeClusterResourceInputs returns the inputs the GKE program gives the
// cluster.
func gkeClusterResourceInputs(t *testing.T, infra *KubernetesRuntimeInfraGKE) map[string]any {
	t.Helper()
	clusters := gkeResources(t, infra)[gkeClusterTypeToken]
	require.Len(t, clusters, 1)
	return clusters[0].inputs
}

// TestAPrivateGkeClusterHasNoPublicIpEndpoint covers #729's PrivateApi on
// GKE: the public IP endpoint is off and the DNS endpoint admits traffic
// from outside Google Cloud, which is how Threeport and tptctl reach the
// cluster.  AllowedCidrs limit the internal endpoint and are not joined by
// the control plane's public addresses, which could not reach it.  A public
// cluster is provisioned as before.
func TestAPrivateGkeClusterHasNoPublicIpEndpoint(t *testing.T) {
	t.Run("private", func(t *testing.T) {
		infra := testGkeInfra()
		infra.PrivateApi = true
		inputs := gkeClusterResourceInputs(t, infra)

		assert.Equal(t, true, inputs["privateClusterConfig"].(map[string]any)["enablePrivateEndpoint"])
		assert.Equal(t, map[string]any{
			"dnsEndpointConfig": map[string]any{"allowExternalTraffic": true},
		}, inputs["controlPlaneEndpointsConfig"])
		assert.NotContains(t, inputs, "masterAuthorizedNetworksConfig",
			"with no AllowedCidrs the internal endpoint stays open to the cluster's network and those connected to it")
	})
	t.Run("private with AllowedCidrs", func(t *testing.T) {
		infra := testGkeInfra()
		infra.PrivateApi = true
		infra.AllowedCidrs = []string{"10.8.0.0/16"}
		infra.ControlPlaneEgressCidrs = []string{"198.51.100.20/32"}
		config := gkeClusterResourceInputs(t, infra)["masterAuthorizedNetworksConfig"].(map[string]any)
		assert.Equal(t, []any{map[string]any{"cidrBlock": "10.8.0.0/16"}}, config["cidrBlocks"])
		assert.Equal(t, true, config["privateEndpointEnforcementEnabled"],
			"GKE applies the list to the internal endpoint only when told to")
	})
	t.Run("private admits GKE's private limit", func(t *testing.T) {
		infra := testGkeInfra()
		infra.PrivateApi = true
		infra.ControlPlaneEgressCidrs = []string{"198.51.100.20/32"}

		infra.AllowedCidrs = blocksForTest(100)
		allowlist, err := infra.kubeApiAllowlist()
		require.NoError(t, err)
		assert.Len(t, allowlist, 100)

		infra.AllowedCidrs = blocksForTest(101)
		err = pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", &gkeRecordingMocks{}))
		assert.ErrorContains(t, err, "AllowedCidrs has 101 entries, but gke admits at most 100 on a private Kubernetes API")
		_, err = infra.Create()
		assert.ErrorContains(t, err, "gke admits at most 100 on a private Kubernetes API", "refused before anything is created")
	})
	t.Run("public", func(t *testing.T) {
		inputs := gkeClusterResourceInputs(t, testGkeInfra())
		assert.Equal(t, false, inputs["privateClusterConfig"].(map[string]any)["enablePrivateEndpoint"])
		assert.NotContains(t, inputs, "controlPlaneEndpointsConfig")
	})
}

// TestTheDnsEndpointReachesAPrivateGkeCluster covers GetConnection's choice
// of endpoint: a cluster with a public IP endpoint is reached there and
// verified with its own CA, as before, and one without is reached through
// its DNS endpoint, whose certificate the system's roots verify.
func TestTheDnsEndpointReachesAPrivateGkeCluster(t *testing.T) {
	ca := "-----BEGIN CERTIFICATE-----\ncluster ca\n-----END CERTIFICATE-----\n"
	masterAuth := &containerpb.MasterAuth{ClusterCaCertificate: base64.StdEncoding.EncodeToString([]byte(ca))}
	dns := &containerpb.ControlPlaneEndpointsConfig_DNSEndpointConfig{
		Endpoint:             "gke-0123456789abcdef-123456789012.us-central1.gke.goog",
		AllowExternalTraffic: ptr(true),
	}

	cases := []struct {
		name     string
		cluster  *containerpb.Cluster
		endpoint string
		ca       string
		err      string
	}{
		{
			name:     "a public cluster from before",
			cluster:  &containerpb.Cluster{Endpoint: "34.1.2.3", MasterAuth: masterAuth},
			endpoint: "https://34.1.2.3", ca: ca,
		},
		{
			name: "a public cluster with a DNS endpoint",
			cluster: &containerpb.Cluster{
				Endpoint:   "34.1.2.3",
				MasterAuth: masterAuth,
				ControlPlaneEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig{
					DnsEndpointConfig: dns,
					IpEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig_IPEndpointsConfig{EnablePublicEndpoint: ptr(true)},
				},
			},
			endpoint: "https://34.1.2.3", ca: ca,
		},
		{
			name: "a private cluster",
			cluster: &containerpb.Cluster{
				Endpoint:             "172.16.0.2",
				MasterAuth:           masterAuth,
				PrivateClusterConfig: &containerpb.PrivateClusterConfig{EnablePrivateEndpoint: true},
				ControlPlaneEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig{
					DnsEndpointConfig: dns,
					IpEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig_IPEndpointsConfig{EnablePublicEndpoint: ptr(false)},
				},
			},
			endpoint: "https://" + dns.Endpoint, ca: "",
		},
		{
			name: "a private cluster that reports only the IP endpoints config",
			cluster: &containerpb.Cluster{
				Endpoint:   "172.16.0.2",
				MasterAuth: masterAuth,
				ControlPlaneEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig{
					DnsEndpointConfig: dns,
					IpEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig_IPEndpointsConfig{EnablePublicEndpoint: ptr(false)},
				},
			},
			endpoint: "https://" + dns.Endpoint, ca: "",
		},
		{
			name: "a private cluster whose DNS endpoint admits no outside traffic",
			cluster: &containerpb.Cluster{
				Name:                 "closed",
				PrivateClusterConfig: &containerpb.PrivateClusterConfig{EnablePrivateEndpoint: true},
				ControlPlaneEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig{
					DnsEndpointConfig: &containerpb.ControlPlaneEndpointsConfig_DNSEndpointConfig{Endpoint: dns.Endpoint, AllowExternalTraffic: ptr(false)},
				},
			},
			err: "GKE cluster closed has no public IP endpoint and no DNS endpoint that admits traffic from outside its network",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			endpoint, gotCA, err := gkeKubeApiEndpoint(c.cluster)
			if c.err != "" {
				assert.ErrorContains(t, err, c.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.endpoint, endpoint)
			assert.Equal(t, c.ca, gotCA)
		})
	}
}

func ptr[T any](v T) *T { return &v }
