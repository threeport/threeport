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

// The delete path finds its service account by deriving the name from the
// provider's, which is not proof that Threeport created it. These cover what it
// does before stripping a project's IAM bindings and deleting the account.

// ownedServiceAccount returns a service account carrying Threeport's ownership
// marker for a provider, as the create path writes it.
func ownedServiceAccount(ownerName string) *iam.ServiceAccount {
	return &iam.ServiceAccount{
		Email: fmt.Sprintf("%s@a-project.iam.gserviceaccount.com", generateServiceAccountID(ownerName)),
		Description: fmt.Sprintf(
			"Service account for Threeport GcpProvider %s to manage GCP resources; %s",
			ownerName,
			GcpOwnershipDescription(ownerName),
		),
	}
}

func TestGcpServiceAccountDeletable_OwnedAccount(t *testing.T) {
	deletable, err := gcpServiceAccountDeletable(
		ownedServiceAccount("my-provider"), true, "my-provider", "my-provider@a-project.iam.gserviceaccount.com",
	)
	require.NoError(t, err)
	assert.True(t, deletable)
}

// a delete re-run after a partial one has to be able to finish
func TestGcpServiceAccountDeletable_MissingAccountIsNothingToDo(t *testing.T) {
	deletable, err := gcpServiceAccountDeletable(
		nil, false, "my-provider", "my-provider@a-project.iam.gserviceaccount.com",
	)
	require.NoError(t, err)
	assert.False(t, deletable)
}

// an account Threeport did not create is refused rather than stripped
func TestGcpServiceAccountDeletable_UnownedAccountIsRefused(t *testing.T) {
	unowned := &iam.ServiceAccount{
		Email:       "my-provider@a-project.iam.gserviceaccount.com",
		Description: "created by hand for the data team",
	}

	deletable, err := gcpServiceAccountDeletable(
		unowned, true, "my-provider", unowned.Email,
	)
	require.Error(t, err)
	assert.False(t, deletable)
	assert.Contains(t, err.Error(), "not created by Threeport")
}

// TestGcpServiceAccountDeletable_TruncationCollision covers the case the derived
// name cannot distinguish.
//
// Two provider names longer than the thirty characters a service account id
// allows can derive the same id, so deleting one would find the other's account.
// The ownership marker carries the full provider name, which is what separates
// them.
func TestGcpServiceAccountDeletable_TruncationCollision(t *testing.T) {
	first := "a-very-long-threeport-provider-name-one"
	second := "a-very-long-threeport-provider-name-two"
	require.Equal(
		t, generateServiceAccountID(first), generateServiceAccountID(second),
		"this test is meaningless unless the two names derive the same account id",
	)

	// deleting the second must not touch the account the first owns
	deletable, err := gcpServiceAccountDeletable(
		ownedServiceAccount(first), true, second, "shared@a-project.iam.gserviceaccount.com",
	)
	require.Error(t, err)
	assert.False(t, deletable)
}
