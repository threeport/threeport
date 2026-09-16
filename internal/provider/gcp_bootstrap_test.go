package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
)

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
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

// TestCanonicalGCPAccountName_RejectsCaseFolding covers names that would
// share a service-account ID with a lowercase sibling.
func TestCanonicalGCPAccountName_RejectsCaseFolding(t *testing.T) {
	require.True(t, canonicalGCPAccountName("my-provider"))
	require.False(t, canonicalGCPAccountName("My-Provider"))
	require.False(t, canonicalGCPAccountName("my_provider"))
	require.False(t, canonicalGCPAccountName("aaaaaaaaaaaaaaaaa"))
}
