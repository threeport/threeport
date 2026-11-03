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

// TestSelectInstanceControllersKeepsNamedGroup checks that gcp keeps only gcp-controller.
// The image applied before the select stays on that controller.
func TestSelectInstanceControllersKeepsNamedGroup(t *testing.T) {
	childInstaller := threeport.NewInstaller()
	child := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr("ghcr.io/child"),
		Apis:           util.Ptr("gcp"),
	}
	require.NoError(t, childInstaller.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child))

	// keep gcp-controller after images are set
	require.NoError(t, selectInstanceControllers(childInstaller, child.Apis))

	require.Len(t, childInstaller.Opts.ControllerList, 1)
	assert.Equal(t, "gcp-controller", childInstaller.Opts.ControllerList[0].Name)
	assert.Equal(t, "ghcr.io/child", childInstaller.Opts.ControllerList[0].ImageNamespace)
}

// TestSelectInstanceControllersNoneClearsTheList checks that none installs no controllers.
func TestSelectInstanceControllersNoneClearsTheList(t *testing.T) {
	childInstaller := threeport.NewInstaller()

	// none clears the controller list
	require.NoError(t, selectInstanceControllers(childInstaller, util.Ptr("none")))

	assert.Nil(t, childInstaller.Opts.ControllerList)
}

// TestSelectInstanceControllersNilLeavesTheList checks that an omitted Apis installs every controller.
func TestSelectInstanceControllersNilLeavesTheList(t *testing.T) {
	childInstaller := threeport.NewInstaller()
	before := len(childInstaller.Opts.ControllerList)

	// leave the full list when Apis is omitted
	require.NoError(t, selectInstanceControllers(childInstaller, nil))

	assert.Len(t, childInstaller.Opts.ControllerList, before)
}

// TestSelectInstanceControllersRejectsUnknownGroup checks that an unknown group returns an error.
func TestSelectInstanceControllersRejectsUnknownGroup(t *testing.T) {
	childInstaller := threeport.NewInstaller()

	// an unknown group fails and leaves the list in place
	err := selectInstanceControllers(childInstaller, util.Ptr("not-a-group"))

	require.Error(t, err)
	assert.NotEmpty(t, childInstaller.Opts.ControllerList)
}
