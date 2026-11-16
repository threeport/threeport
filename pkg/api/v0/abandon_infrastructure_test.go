package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestRuntimeInstanceBeforeCreate_RejectsAbandonInfrastructure covers the
// override being refused on create: an instance born with it set would skip the
// controller's safeguards on every later delete.
func TestRuntimeInstanceBeforeCreate_RejectsAbandonInfrastructure(t *testing.T) {
	cases := []struct {
		name string
		// beforeCreate runs the hook for one instance type with the flag
		beforeCreate func(abandon *bool) error
	}{
		{
			name: "gcp gke",
			beforeCreate: func(abandon *bool) error {
				return (&GcpGkeKubernetesRuntimeInstance{AbandonInfrastructure: abandon}).beforeCreate(nil)
			},
		},
		{
			name: "aws eks",
			beforeCreate: func(abandon *bool) error {
				return (&AwsEksKubernetesRuntimeInstance{AbandonInfrastructure: abandon}).beforeCreate(nil)
			},
		},
		{
			name: "oci oke",
			beforeCreate: func(abandon *bool) error {
				return (&OciOkeKubernetesRuntimeInstance{AbandonInfrastructure: abandon}).beforeCreate(nil)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, tc.beforeCreate(nil), "unset is allowed")
			assert.NoError(t, tc.beforeCreate(util.Ptr(false)), "false is allowed")
			assert.ErrorContains(t, tc.beforeCreate(util.Ptr(true)), "AbandonInfrastructure cannot be set when an instance is created")
		})
	}
}
