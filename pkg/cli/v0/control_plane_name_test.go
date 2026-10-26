package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/threeport/threeport/internal/provider"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestValidateCreateGenesisControlPlaneFlags_NameLength covers --name being
// capped at one length for every provider, and by the shorter limit AWS puts on
// role names for eks alone.
func TestValidateCreateGenesisControlPlaneFlags_NameLength(t *testing.T) {
	validate := func(infraProvider, name string) error {
		return ValidateCreateGenesisControlPlaneFlags(
			name, infraProvider, threeport.ControlPlaneTierDev, "", true, nil, false, "", 0,
		)
	}
	long := strings.Repeat("a", threeport.InstanceNameMaxLength+1)
	atCap := strings.Repeat("a", provider.NameMaxLength)
	overCap := strings.Repeat("a", provider.NameMaxLength+1)

	// a name a service-account ID once could not hold
	assert.NoError(t, validate("gke", "genesis-02"))
	assert.NoError(t, validate("gke", long))
	assert.NoError(t, validate("gke", atCap))
	assert.NoError(t, validate("kind", long))

	// no provider takes a name over the cap
	for _, infraProvider := range []string{"kind", "gke", "eks"} {
		assert.ErrorContains(t, validate(infraProvider, overCap), "too long", infraProvider)
	}

	// eks keeps the AWS-derived limit
	assert.NoError(t, validate("eks", strings.Repeat("a", threeport.InstanceNameMaxLength)))
	assert.ErrorContains(t, validate("eks", long), "too long")
}
