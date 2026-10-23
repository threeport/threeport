package provider

import (
	"testing"

	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
)

// testWorkerPool is the name of the worker pool readyCluster carries.
const testWorkerPool = "rt-nodepool"

// readyCluster returns a cluster that is ready to be connected to.
func readyCluster() *containerpb.Cluster {
	return &containerpb.Cluster{
		Status:     containerpb.Cluster_RUNNING,
		Endpoint:   "203.0.113.10",
		MasterAuth: &containerpb.MasterAuth{ClusterCaCertificate: "Y2E="},
		NodePools: []*containerpb.NodePool{
			{Name: testWorkerPool, Status: containerpb.NodePool_RUNNING},
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

// TestGkeWorkerPoolReady covers a cluster whose worker pool is not running yet.
func TestGkeWorkerPoolReady(t *testing.T) {
	t.Run("worker pool running", func(t *testing.T) {
		require.NoError(t, gkeWorkerPoolReady(readyCluster(), testWorkerPool))
	})

	t.Run("no pools", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = nil
		assert.Error(t, gkeWorkerPoolReady(cluster, testWorkerPool))
	})

	// The stack creates the cluster with a default pool and removes it before
	// the worker pool is created. A running default pool is not a finished create.
	t.Run("only the default pool, running", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = []*containerpb.NodePool{
			{Name: "default-pool", Status: containerpb.NodePool_RUNNING},
		}
		err := gkeWorkerPoolReady(cluster, testWorkerPool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), testWorkerPool)
	})

	t.Run("default pool running beside a worker pool still provisioning", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = []*containerpb.NodePool{
			{Name: "default-pool", Status: containerpb.NodePool_RUNNING},
			{Name: testWorkerPool, Status: containerpb.NodePool_PROVISIONING},
		}
		err := gkeWorkerPoolReady(cluster, testWorkerPool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PROVISIONING")
	})

	t.Run("worker pool running while the default pool is removed", func(t *testing.T) {
		cluster := readyCluster()
		cluster.NodePools = append(cluster.NodePools, &containerpb.NodePool{
			Name:   "default-pool",
			Status: containerpb.NodePool_STOPPING,
		})
		require.NoError(t, gkeWorkerPoolReady(cluster, testWorkerPool))
	})
}

// TestWorkerNodePoolName pins the name the create check looks for to the one
// the stack gives the pool.
func TestWorkerNodePoolName(t *testing.T) {
	infra := &KubernetesRuntimeInfraGKE{}
	infra.RuntimeInstanceName = "rt"
	assert.Equal(t, testWorkerPool, infra.workerNodePoolName())
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
