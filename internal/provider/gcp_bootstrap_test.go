package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestThreeportServiceAccountRoles_AssertsInstanceAdminRole asserts
// threeportServiceAccountRoles includes roles/compute.instanceAdmin.v1.
func TestThreeportServiceAccountRoles_AssertsInstanceAdminRole(t *testing.T) {
	assert.Contains(t, threeportServiceAccountRoles, "roles/compute.instanceAdmin.v1")
}
