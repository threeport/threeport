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

// An upgrade moves every default deployment to one tag and records it as the
// instance-wide tag of the control plane. The record of the components cannot
// be updated through the API, so it still carries the tag an exception had
// before the upgrade. A child created afterwards takes the tag the upgrade left
// running, which is what its parent runs, and keeps the image name and registry
// of the exception, which the upgrade does not touch.
func TestChildAfterUpgradeTakesTheTagTheUpgradeLeftRunning(t *testing.T) {
	parent := &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "rest-api", ImageNamespace: "ghcr.io/threeport", ImageName: "threeport-rest-api", ImageTag: "v0.7.0"},
			{Name: "agent", ImageNamespace: "mirror.example.com", ImageName: "forked-agent", ImageTag: "forked"},
		},
	}

	// what `tptctl upgrade control-plane --version v0.7.1` records
	parent.ImageTag = util.Ptr("v0.7.1")

	cpi := threeport.NewInstaller()
	require.NoError(t, cpi.ConfigureChildComponents(parent, &v0.ControlPlaneInstance{}))

	assert.Equal(t, "v0.7.1", cpi.Opts.RestApiInfo.ImageTag)
	assert.Equal(t, "v0.7.1", cpi.Opts.AgentInfo.ImageTag, "the upgrade moved the pinned agent too")
	assert.Equal(t, "forked-agent", cpi.Opts.AgentInfo.ImageName)
	assert.Equal(t, "mirror.example.com", cpi.Opts.AgentInfo.ImageNamespace)

	// the child's own settings still win over the upgrade
	child := &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{{Name: "agent", ImageTag: "child-agent"}},
	}
	cpi = threeport.NewInstaller()
	require.NoError(t, cpi.ConfigureChildComponents(parent, child))

	assert.Equal(t, "child-agent", cpi.Opts.AgentInfo.ImageTag)
	assert.Equal(t, "v0.7.1", cpi.Opts.RestApiInfo.ImageTag)
}
