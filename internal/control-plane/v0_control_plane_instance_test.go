package controlplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// A child records what it runs so that its own children can default to the
// same images. Its children apply an instance-wide image over whatever its
// record says, so the record cannot keep one next to components that are
// exceptions to it.
func TestRecordResolvedComponents_GrandchildKeepsTheChildsExceptions(t *testing.T) {
	child := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr("wide-registry"),
		ImageTag:       util.Ptr("wide"),
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "agent", ImageNamespace: "special-registry", ImageTag: "special"},
		},
	}
	childInstaller := threeport.NewInstaller()
	require.NoError(t, childInstaller.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child))
	require.Equal(t, "special", childInstaller.Opts.AgentInfo.ImageTag)
	require.Equal(t, "wide", childInstaller.Opts.RestApiInfo.ImageTag)

	// what the child writes as its own self record
	record := *child
	recordResolvedComponents(&record, childInstaller)

	assert.Nil(t, record.ImageNamespace)
	assert.Nil(t, record.ImageTag)

	// a control plane created from the child
	grandchildInstaller := threeport.NewInstaller()
	require.NoError(t, grandchildInstaller.ConfigureChildComponents(&record, &v0.ControlPlaneInstance{}))

	assert.Equal(t, "special", grandchildInstaller.Opts.AgentInfo.ImageTag)
	assert.Equal(t, "special-registry", grandchildInstaller.Opts.AgentInfo.ImageNamespace)
	assert.Equal(t, "wide", grandchildInstaller.Opts.RestApiInfo.ImageTag)
	assert.Equal(t, "wide-registry", grandchildInstaller.Opts.RestApiInfo.ImageNamespace)

	// the child's own instance is left as it was asked for
	assert.Equal(t, "wide", *child.ImageTag)
}
