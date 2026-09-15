package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iam/v1"
)

// TestOldestUserManagedKeys_KeepZero deletes every user-managed key.
func TestOldestUserManagedKeys_KeepZero(t *testing.T) {
	keys := []*iam.ServiceAccountKey{
		{Name: "k1", KeyType: "USER_MANAGED", ValidAfterTime: "2020-01-01T00:00:00Z"},
		{Name: "k2", KeyType: "USER_MANAGED", ValidAfterTime: "2020-01-02T00:00:00Z"},
		{Name: "sys", KeyType: "SYSTEM_MANAGED", ValidAfterTime: "2019-01-01T00:00:00Z"},
	}
	got := oldestUserManagedKeys(keys, 0)
	assert.Len(t, got, 2)
	assert.Equal(t, "k1", got[0].Name)
	assert.Equal(t, "k2", got[1].Name)
}

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
}
