package provider

import (
	"testing"

	"cloud.google.com/go/container/apiv1/containerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheControlPlaneCanConnectToTheProjectsClusters covers the role that
// lets a control plane's controllers reach a private cluster's DNS endpoint:
// it is granted to every Kubernetes service account in the control plane's
// namespace, once, and removed from that role alone, leaving the namespace's
// other roles and the role's other members.
func TestTheControlPlaneCanConnectToTheProjectsClusters(t *testing.T) {
	withNoIAMRetryDelay(t)
	member := controlPlaneConnectMember(123456789, "proj")
	assert.Equal(t,
		"principalSet://iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/proj.svc.id.goog/namespace/threeport-control-plane",
		member,
	)

	store := &fakePolicies{policy: policyWith(
		binding(gkeClusterConnectRole, "user:someone@example.com"),
		binding("roles/storage.objectViewer", member),
	)}
	require.NoError(t, grantClusterConnect(store, "proj", member))
	assert.ElementsMatch(t, []string{"user:someone@example.com", member}, store.policy.Bindings[0].Members)
	require.NoError(t, grantClusterConnect(store, "proj", member))
	assert.Equal(t, 1, store.sets, "a second grant writes nothing")

	require.NoError(t, revokeClusterConnect(store, "proj", member))
	assert.Equal(t, []string{"user:someone@example.com"}, store.policy.Bindings[0].Members)
	assert.Equal(t, []string{member}, store.policy.Bindings[1].Members, "another role the namespace holds is kept")

	// a role left with no member is dropped
	alone := &fakePolicies{policy: policyWith(binding(gkeClusterConnectRole, member))}
	require.NoError(t, revokeClusterConnect(alone, "proj", member))
	assert.Empty(t, alone.policy.Bindings)
}

// TestTheConnectRoleStaysWhileAnotherThreeportClusterRemains covers when a
// teardown removes the role: the namespace is shared by every control plane
// in the project that uses it, so the role stays while the project holds a
// cluster Threeport provisioned other than the one going.
func TestTheConnectRoleStaysWhileAnotherThreeportClusterRemains(t *testing.T) {
	cluster := func(name, location string, threeport bool) *containerpb.Cluster {
		labels := map[string]string{}
		if threeport {
			labels = GcpResourceLabels(name)
		}
		return &containerpb.Cluster{Name: name, Location: location, ResourceLabels: labels}
	}

	assert.False(t, threeportClusterOtherThan(nil, "going", "us-central1"))
	assert.False(t, threeportClusterOtherThan([]*containerpb.Cluster{
		cluster("going", "us-central1", true), cluster("someone-elses", "us-central1", false),
	}, "going", "us-central1"))
	assert.True(t, threeportClusterOtherThan([]*containerpb.Cluster{
		cluster("going", "us-central1", true), cluster("workload", "us-central1", true),
	}, "going", "us-central1"))
	assert.True(t, threeportClusterOtherThan([]*containerpb.Cluster{
		cluster("going", "us-central1", true), cluster("going", "us-east1", true),
	}, "going", "us-central1"), "a cluster of the same name in another region")
}
