package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/container/apiv1/containerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
)

// gkeCluster is a cluster as GKE lists it: labelled as Threeport's or not,
// with a public IP endpoint or private.
func gkeCluster(name, location string, threeport, private bool) *containerpb.Cluster {
	labels := map[string]string{}
	if threeport {
		labels = GcpResourceLabels(name)
	}
	public := !private

	return &containerpb.Cluster{
		Name:           name,
		Location:       location,
		ResourceLabels: labels,
		ControlPlaneEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig{
			IpEndpointsConfig: &containerpb.ControlPlaneEndpointsConfig_IPEndpointsConfig{EnablePublicEndpoint: &public},
		},
	}
}

// listing returns a cluster lister that answers with each of lists in turn,
// the last one from then on, and counts its calls.
func listing(calls *int, lists ...[]*containerpb.Cluster) func() ([]*containerpb.Cluster, error) {
	return func() ([]*containerpb.Cluster, error) {
		*calls++
		if *calls <= len(lists) {
			return lists[*calls-1], nil
		}
		return lists[len(lists)-1], nil
	}
}

// TestTheControlPlaneCanConnectToTheProjectsPrivateClusters covers the role
// that lets a control plane's controllers reach a private cluster's DNS
// endpoint: it is granted to every Kubernetes service account in the control
// plane's namespace, and removed from that role alone, leaving the
// namespace's other roles and the role's other members.
func TestTheControlPlaneCanConnectToTheProjectsPrivateClusters(t *testing.T) {
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
	assert.Equal(t, 2, store.sets, "a grant writes even when the member holds the role, so a concurrent revoke's etag goes stale")
	assert.ElementsMatch(t, []string{"user:someone@example.com", member}, store.policy.Bindings[0].Members, "and adds it once")

	calls := 0
	require.NoError(t, revokeClusterConnect(store, "proj", member, listing(&calls, nil), "going", "us-central1"))
	assert.Equal(t, []string{"user:someone@example.com"}, store.policy.Bindings[0].Members)
	assert.Equal(t, []string{member}, store.policy.Bindings[1].Members, "another role the namespace holds is kept")

	// a role left with no member is dropped
	alone := &fakePolicies{policy: policyWith(binding(gkeClusterConnectRole, member))}
	require.NoError(t, revokeClusterConnect(alone, "proj", member, listing(&calls, nil), "going", "us-central1"))
	assert.Empty(t, alone.policy.Bindings)

	// a binding of the role under a condition someone else made is theirs
	conditional := binding(gkeClusterConnectRole, member)
	conditional.Condition = &cloudresourcemanager.Expr{Expression: "request.time < timestamp('2030-01-01T00:00:00Z')"}
	theirs := &fakePolicies{policy: policyWith(conditional)}
	require.NoError(t, revokeClusterConnect(theirs, "proj", member, listing(&calls, nil), "going", "us-central1"))
	assert.Equal(t, []string{member}, theirs.policy.Bindings[0].Members)

	// with nothing to remove, the clusters are not listed and nothing is written
	calls, sets := 0, alone.sets
	require.NoError(t, revokeClusterConnect(alone, "proj", member, listing(&calls, nil), "going", "us-central1"))
	assert.Zero(t, calls)
	assert.Equal(t, sets, alone.sets)
}

// TestAPublicClusterGrantsNoConnectRole covers which clusters are granted the
// role: a cluster with a public IP endpoint is reached there, so nothing is
// granted for it, nor any client made.
func TestAPublicClusterGrantsNoConnectRole(t *testing.T) {
	infra := &KubernetesRuntimeInfraGKE{
		ProjectID: "proj",
		Region:    "us-central1",
		// unusable credentials: any client made from them fails
		ServiceAccountCredentials: "not json",
	}
	require.NoError(t, infra.grantControlPlaneClusterConnect())

	infra.PrivateApi = true
	require.Error(t, infra.grantControlPlaneClusterConnect(), "a private cluster asks for the grant")
}

// TestTheConnectRoleStaysWhileAnotherPrivateThreeportClusterRemains covers
// when a teardown removes the role: the namespace is shared by every control
// plane in the project that uses it, so the role stays while the project
// holds a private cluster Threeport provisioned other than the one going.
func TestTheConnectRoleStaysWhileAnotherPrivateThreeportClusterRemains(t *testing.T) {
	assert.False(t, privateThreeportClusterOtherThan(nil, "going", "us-central1"))
	assert.False(t, privateThreeportClusterOtherThan([]*containerpb.Cluster{
		gkeCluster("going", "us-central1", true, true),
		gkeCluster("someone-elses", "us-central1", false, true),
		gkeCluster("public", "us-central1", true, false),
	}, "going", "us-central1"), "the one going, someone else's, and a public one")
	assert.True(t, privateThreeportClusterOtherThan([]*containerpb.Cluster{
		gkeCluster("going", "us-central1", true, true), gkeCluster("workload", "us-central1", true, true),
	}, "going", "us-central1"))
	assert.True(t, privateThreeportClusterOtherThan([]*containerpb.Cluster{
		gkeCluster("going", "us-central1", true, true), gkeCluster("going", "us-east1", true, true),
	}, "going", "us-central1"), "a cluster of the same name in another region")
	assert.True(t, privateThreeportClusterOtherThan([]*containerpb.Cluster{
		{Name: "older", Location: "us-central1", ResourceLabels: GcpResourceLabels("older"),
			PrivateClusterConfig: &containerpb.PrivateClusterConfig{EnablePrivateEndpoint: true}},
	}, "going", "us-central1"), "a private cluster from before the endpoints config")

	withNoIAMRetryDelay(t)
	member := controlPlaneConnectMember(1, "proj")
	store := &fakePolicies{policy: policyWith(binding(gkeClusterConnectRole, member))}
	calls := 0
	require.NoError(t, revokeClusterConnect(store, "proj", member, listing(&calls,
		[]*containerpb.Cluster{gkeCluster("workload", "us-central1", true, true)},
	), "going", "us-central1"))
	assert.Equal(t, []string{member}, store.policy.Bindings[0].Members)
	assert.Zero(t, store.sets)

	failing := func() ([]*containerpb.Cluster, error) { return nil, errors.New("list failed") }
	require.ErrorContains(t, revokeClusterConnect(store, "proj", member, failing, "going", "us-central1"), "list failed")
	assert.Equal(t, []string{member}, store.policy.Bindings[0].Members, "a failed listing removes nothing")
}

// TestARevokeDecidesAgainWhenACreateGrantsMeanwhile covers a teardown that
// overlaps a create in the same project: the create's grant is written after
// the revoke read the policy and listed the clusters, before the revoke wrote
// it.  The revoke's write is refused on its stale etag, and it reads and
// lists again, finding the new private cluster, so it keeps the role.
func TestARevokeDecidesAgainWhenACreateGrantsMeanwhile(t *testing.T) {
	withNoIAMRetryDelay(t)
	member := controlPlaneConnectMember(1, "proj")
	store := &fakePolicies{
		policy:   policyWith(binding(gkeClusterConnectRole, member)),
		conflict: 1,
	}
	calls := 0
	require.NoError(t, revokeClusterConnect(store, "proj", member, listing(&calls,
		nil,
		[]*containerpb.Cluster{gkeCluster("new", "us-central1", true, true)},
	), "going", "us-central1"))
	assert.Equal(t, 2, calls, "the clusters are listed again after the conflict")
	assert.Equal(t, []string{member}, store.policy.Bindings[0].Members)
}

// TestProjectPoliciesAreReadAndWrittenWithTheirConditions covers the version
// a project policy is read and written at: one that carries conditional
// bindings, so a project holding any is written back with its conditions
// rather than refused.
func TestProjectPoliciesAreReadAndWrittenWithTheirConditions(t *testing.T) {
	var requested, written int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects/proj:getIamPolicy":
			var request cloudresourcemanager.GetIamPolicyRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			if request.Options != nil {
				requested = request.Options.RequestedPolicyVersion
			}
			// a policy with no condition is version 1, whatever is asked
			require.NoError(t, json.NewEncoder(w).Encode(&cloudresourcemanager.Policy{Version: 1}))
		case "/v1/projects/proj:setIamPolicy":
			var request cloudresourcemanager.SetIamPolicyRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			written = request.Policy.Version
			require.NoError(t, json.NewEncoder(w).Encode(request.Policy))
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)
	service, err := cloudresourcemanager.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
	require.NoError(t, err)

	require.NoError(t, grantClusterConnect(crmProjectPolicies{service: service}, "proj", "principalSet://member"))
	assert.Equal(t, int64(conditionalPolicyVersion), requested)
	assert.Equal(t, int64(conditionalPolicyVersion), written)
}
