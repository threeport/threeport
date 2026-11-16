package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestModuleImagePrecedesBlueGreen checks the module image is recorded before the backup.
func TestModuleImagePrecedesBlueGreen(t *testing.T) {
	var steps []string
	err := recordModuleThenInstall(
		func() error {
			steps = append(steps, "module")
			return nil
		},
		func() error {
			steps = append(steps, "install")
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"module", "install"}, steps)
}

// TestAChildModuleUpgradeCarriesAnInstallRequest covers the module install
// request a blue-green carries: a child gets one built from the module image, a
// control plane that is not a child gets none, and a child needs both image
// values.
func TestAChildModuleUpgradeCarriesAnInstallRequest(t *testing.T) {
	// set the module image the flags record, and restore the flags afterward
	moduleUpgradeName = "acme.example/acme-module-api"
	moduleUpgradeImageNamespace = "registry.example/repo"
	moduleUpgradeImageTag = "v1"
	t.Cleanup(func() {
		moduleUpgradeName, moduleUpgradeImageNamespace, moduleUpgradeImageTag = "", "", ""
	})

	// assert a child carries the module name, image namespace, and tag
	request, err := moduleBlueGreenInstall("primary-04")
	require.NoError(t, err)
	require.Equal(t, "acme.example/acme-module-api registry.example/repo v1", request)

	// assert a control plane that is not a child carries none
	request, err = moduleBlueGreenInstall("")
	require.NoError(t, err)
	require.Empty(t, request)

	// assert a child with no image namespace is refused
	moduleUpgradeImageNamespace = ""
	_, err = moduleBlueGreenInstall("primary-04")
	require.Error(t, err)
}

// TestModuleBlueGreenKeepsTheRunningControlPlaneImage covers the control plane
// image flags of a module upgrade: a flag left empty takes the namespace or tag
// the replaced control plane runs, and a flag that is set wins.
func TestModuleBlueGreenKeepsTheRunningControlPlaneImage(t *testing.T) {
	running := &api.ControlPlaneInstance{
		ImageNamespace: util.Ptr("registry.example/threeport"),
		ImageTag:       util.Ptr("abc12345"),
	}

	// assert both empty flags take the running namespace and tag
	imageNamespace, imageTag := moduleBlueGreenImage("", "", running)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "abc12345", imageTag)

	// assert a set flag wins and the other flag still takes the running value
	imageNamespace, imageTag = moduleBlueGreenImage("", "def67890", running)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "def67890", imageTag)

	// assert a control plane that records no image leaves both values empty
	imageNamespace, imageTag = moduleBlueGreenImage("", "", &api.ControlPlaneInstance{})
	require.Equal(t, "", imageNamespace)
	require.Equal(t, "", imageTag)
}
