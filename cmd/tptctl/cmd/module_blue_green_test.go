package cmd

import (
	"errors"
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

// TestModuleInstallFollowsAChildBlueGreen covers the module install a child's
// upgrade records: it runs after the blue-green, it is skipped for a control
// plane that is not a child, and a failed blue-green records none.
func TestModuleInstallFollowsAChildBlueGreen(t *testing.T) {
	steps := func(child bool, blueGreenErr error) ([]string, error) {
		var ran []string
		err := installModuleAfterBlueGreen(
			child,
			func() error {
				ran = append(ran, "blue-green")
				return blueGreenErr
			},
			func() error {
				ran = append(ran, "install")
				return nil
			},
		)
		return ran, err
	}

	// assert a child installs after its blue-green
	ran, err := steps(true, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"blue-green", "install"}, ran)

	// assert a control plane that is not a child records no install
	ran, err = steps(false, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"blue-green"}, ran)

	// assert a failed blue-green records no install
	ran, err = steps(true, errors.New("blue-green failed"))
	require.Error(t, err)
	require.Equal(t, []string{"blue-green"}, ran)
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
