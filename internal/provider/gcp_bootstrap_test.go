package provider

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
)

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.securityAdmin")
}

// TestGcpResourceLabels_SanitizesOwnerName covers lowercase, dashes, and the 63-char cap.
func TestGcpResourceLabels_SanitizesOwnerName(t *testing.T) {
	got := GcpResourceLabels("My_Provider.Name")
	assert.Equal(t, GcpLabelProvisionedByValue, got[GcpLabelProvisionedBy])
	assert.Equal(t, "my_provider-name", got[GcpLabelThreeportName])

	got = GcpResourceLabels("!!!")
	assert.Equal(t, "unnamed", got[GcpLabelThreeportName])
}

// TestServiceAccountOwnedBy_RequiresOwnershipDescription covers reuse vs refuse.
func TestServiceAccountOwnedBy_RequiresOwnershipDescription(t *testing.T) {
	name := "my-provider"
	owned := &iam.ServiceAccount{
		Description: "Service account for Threeport GcpProvider my-provider to manage GCP resources; " + GcpOwnershipDescription(name),
	}
	require.True(t, serviceAccountOwnedBy(owned, name))
	require.False(t, serviceAccountOwnedBy(owned, "other-provider"))
	require.False(t, serviceAccountOwnedBy(&iam.ServiceAccount{Description: "someone else"}, name))
	require.False(t, serviceAccountOwnedBy(nil, name))
}

// TestServiceAccountOwnedBy_RejectsNamePrefix covers a shorter owner name that
// is a prefix of the stored threeport-name value.
func TestServiceAccountOwnedBy_RejectsNamePrefix(t *testing.T) {
	owned := &iam.ServiceAccount{
		Description: "Service account for Threeport GcpProvider app-prod to manage GCP resources; " + GcpOwnershipDescription("app-prod"),
	}

	require.True(t, serviceAccountOwnedBy(owned, "app-prod"))
	require.False(t, serviceAccountOwnedBy(owned, "app"))
}

// TestCanonicalGCPAccountName_RejectsCaseFolding covers names that would
// share a service-account ID with a lowercase sibling.
func TestCanonicalGCPAccountName_RejectsCaseFolding(t *testing.T) {
	require.True(t, canonicalGCPAccountName("my-provider"))
	require.False(t, canonicalGCPAccountName("My-Provider"))
	require.False(t, canonicalGCPAccountName("my_provider"))
	require.False(t, canonicalGCPAccountName("aaaaaaaaaaaaaaaaa"))
}

// TestGkeServiceAccountDescription_EmbedsOwnership covers the GKE create path
// writing GcpOwnershipDescription so a later reuse can prove ownership.
func TestGkeServiceAccountDescription_EmbedsOwnership(t *testing.T) {
	name := "my-runtime"

	got := gkeServiceAccountDescription(name)

	require.Contains(t, got, GcpOwnershipDescription(name))
	require.True(t, serviceAccountOwnedBy(&iam.ServiceAccount{Description: got}, name))
	require.False(t, serviceAccountOwnedBy(&iam.ServiceAccount{Description: got}, "other-runtime"))
}

// TestGkeServiceAccountDescription_OldFormatIsUnowned covers a leftover GKE
// account created before the ownership suffix; reuse must fail closed.
func TestGkeServiceAccountDescription_OldFormatIsUnowned(t *testing.T) {
	name := "my-runtime"
	old := &iam.ServiceAccount{
		Email:       "threeport-svc-my-runtime@proj.iam.gserviceaccount.com",
		Description: fmt.Sprintf("Service account for Threeport instance %s to manage GCP resources", name),
	}

	require.False(t, serviceAccountOwnedBy(old, name))
	require.Error(t, refuseUnownedExistingGCPServiceAccount(old, true, name))
}

// TestRefuseUnownedExistingGCPServiceAccount_AllowsOwnedReuse covers create vs reuse.
func TestRefuseUnownedExistingGCPServiceAccount_AllowsOwnedReuse(t *testing.T) {
	name := "my-runtime"
	owned := &iam.ServiceAccount{
		Email:       "threeport-svc-my-runtime@proj.iam.gserviceaccount.com",
		Description: gkeServiceAccountDescription(name),
	}

	require.NoError(t, refuseUnownedExistingGCPServiceAccount(owned, false, name))
	require.NoError(t, refuseUnownedExistingGCPServiceAccount(owned, true, name))
	require.Error(t, refuseUnownedExistingGCPServiceAccount(owned, true, "other-runtime"))
}

// TestRequireServiceAccountEmail covers the guard in front of the workload
// identity binding.
//
// Without it an empty email builds "projects/<project>/serviceAccounts/" and
// GCP answers 404 about a resource that names nothing - after the cluster's
// network, control plane and node pool are already provisioned.
func TestRequireServiceAccountEmail(t *testing.T) {
	t.Run("a known account passes", func(t *testing.T) {
		infra := &KubernetesRuntimeInfraGKE{
			ProjectID:           "a-project",
			ServiceAccountEmail: "threeport@a-project.iam.gserviceaccount.com",
		}
		require.NoError(t, infra.requireServiceAccountEmail())
	})

	t.Run("an unknown account says so rather than asking GCP", func(t *testing.T) {
		infra := &KubernetesRuntimeInfraGKE{ProjectID: "a-project"}

		err := infra.requireServiceAccountEmail()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no account to name")
	})
}

// TestWorkloadIdentityMembers_NameNoRuntime states why deleting a runtime does
// not revoke the workload identity grant.
//
// The member names the project, the control plane namespace and the controller's
// service account - nothing about the runtime. Two runtimes in one project
// produce the same member, so revoking on a delete would take the grant away
// from the ones still running. The grant belongs to the control plane, which
// outlives any runtime it creates.
func TestWorkloadIdentityMembers_NameNoRuntime(t *testing.T) {
	first := &KubernetesRuntimeInfraGKE{RuntimeInstanceName: "runtime-one", ProjectID: "a-project"}
	second := &KubernetesRuntimeInfraGKE{RuntimeInstanceName: "runtime-two", ProjectID: "a-project"}

	pool := fmt.Sprintf(workloadIdentityPoolFormat, "a-project")
	assert.Equal(
		t,
		first.getWorkloadIdentityMembers(pool),
		second.getWorkloadIdentityMembers(pool),
		"two runtimes in one project share the grant, so it is not one runtime's to revoke",
	)

	for _, member := range first.getWorkloadIdentityMembers(pool) {
		assert.NotContains(t, member, "runtime-one")
	}
}
