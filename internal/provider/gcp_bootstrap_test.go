package provider

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/iam/v1"
)

// TestOldestUserManagedKeys_UnderQuota keeps every live key when under the cap.
func TestOldestUserManagedKeys_UnderQuota(t *testing.T) {
	keys := []*iam.ServiceAccountKey{
		{Name: "k1", KeyType: "USER_MANAGED", ValidAfterTime: "2020-01-01T00:00:00Z"},
		{Name: "sys", KeyType: "SYSTEM_MANAGED", ValidAfterTime: "2019-01-01T00:00:00Z"},
	}
	got := oldestUserManagedKeys(keys, 9)
	assert.Empty(t, got)
}

// TestOldestUserManagedKeys_AtQuota deletes the oldest user-managed keys only.
func TestOldestUserManagedKeys_AtQuota(t *testing.T) {
	keys := make([]*iam.ServiceAccountKey, 0, 10)
	for i := 0; i < 10; i++ {
		keys = append(keys, &iam.ServiceAccountKey{
			Name:           fmt.Sprintf("k%d", i),
			KeyType:        "USER_MANAGED",
			ValidAfterTime: fmt.Sprintf("2020-01-%02dT00:00:00Z", i+1),
		})
	}
	got := oldestUserManagedKeys(keys, 9)
	assert.Len(t, got, 1)
}

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
}
