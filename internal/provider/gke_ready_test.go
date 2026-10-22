package provider

import (
	"testing"

	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
)

// readyCluster returns a cluster that is ready to be connected to.
func readyCluster() *containerpb.Cluster {
	return &containerpb.Cluster{
		Status:     containerpb.Cluster_RUNNING,
		Endpoint:   "203.0.113.10",
		MasterAuth: &containerpb.MasterAuth{ClusterCaCertificate: "Y2E="},
		NodePools: []*containerpb.NodePool{
			{Name: "workers", Status: containerpb.NodePool_RUNNING},
		},
	}
}

// TestGkeClusterReady covers the states GetConnection must not record an
// endpoint from.
func TestGkeClusterReady(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*containerpb.Cluster)
		wantErr string
	}{
		{name: "running with endpoint and CA", mutate: func(*containerpb.Cluster) {}},
		{
			name:    "provisioning",
			mutate:  func(c *containerpb.Cluster) { c.Status = containerpb.Cluster_PROVISIONING },
			wantErr: "PROVISIONING",
		},
		{
			name:    "reconciling",
			mutate:  func(c *containerpb.Cluster) { c.Status = containerpb.Cluster_RECONCILING },
			wantErr: "RECONCILING",
		},
		{
			name:    "running with no endpoint",
			mutate:  func(c *containerpb.Cluster) { c.Endpoint = "" },
			wantErr: "no API endpoint",
		},
		{
			name:    "running with no master auth",
			mutate:  func(c *containerpb.Cluster) { c.MasterAuth = nil },
			wantErr: "no CA certificate",
		},
		{
			name:    "running with an empty CA",
			mutate:  func(c *containerpb.Cluster) { c.MasterAuth.ClusterCaCertificate = "" },
			wantErr: "no CA certificate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := readyCluster()
			tt.mutate(cluster)

			err := gkeClusterReady(cluster)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestGkeNodePoolsReady covers a cluster whose node pools are not all running.
func TestGkeNodePoolsReady(t *testing.T) {
	t.Run("every pool running", func(t *testing.T) {
		require.NoError(t, gkeNodePoolsReady(readyCluster()))
	})

	t.Run("no pools", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = nil
		assert.Error(t, gkeNodePoolsReady(cluster))
	})

	t.Run("one pool still provisioning", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = append(cluster.NodePools, &containerpb.NodePool{
			Name:   "extra",
			Status: containerpb.NodePool_PROVISIONING,
		})
		err := gkeNodePoolsReady(cluster)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extra")
	})
}

// TestPolicyGrantsWorkloadIdentity covers the check that stands in for the last
// step of the create.
func TestPolicyGrantsWorkloadIdentity(t *testing.T) {
	members := []string{"serviceAccount:proj.svc.id.goog[ns/ksa]"}

	t.Run("member bound", func(t *testing.T) {
		policy := &iam.Policy{Bindings: []*iam.Binding{
			{Role: "roles/iam.workloadIdentityUser", Members: members},
		}}
		assert.True(t, policyGrantsWorkloadIdentity(policy, members))
	})

	t.Run("empty policy", func(t *testing.T) {
		assert.False(t, policyGrantsWorkloadIdentity(&iam.Policy{}, members))
	})

	t.Run("member bound to another role", func(t *testing.T) {
		policy := &iam.Policy{Bindings: []*iam.Binding{
			{Role: "roles/iam.serviceAccountUser", Members: members},
		}}
		assert.False(t, policyGrantsWorkloadIdentity(policy, members))
	})

	t.Run("a different member bound", func(t *testing.T) {
		policy := &iam.Policy{Bindings: []*iam.Binding{
			{Role: "roles/iam.workloadIdentityUser", Members: []string{"serviceAccount:proj.svc.id.goog[ns/other]"}},
		}}
		assert.False(t, policyGrantsWorkloadIdentity(policy, members))
	})
}
