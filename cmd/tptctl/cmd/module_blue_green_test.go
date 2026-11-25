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

// TestAControlPlaneThatIsNotAChildCarriesNoInstallRequest covers the module
// install request of a blue-green: a control plane that is not a child carries
// none, and the lookup needs no API client to say so.
func TestAControlPlaneThatIsNotAChildCarriesNoInstallRequest(t *testing.T) {
	request, err := moduleBlueGreenInstall(nil, "", "")
	require.NoError(t, err)
	require.Empty(t, request)
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
	imageNamespace, imageTag, err := moduleBlueGreenImage("", "", running)
	require.NoError(t, err)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "abc12345", imageTag)

	// assert a set flag wins and the other flag still takes the running value
	imageNamespace, imageTag, err = moduleBlueGreenImage("", "def67890", running)
	require.NoError(t, err)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "def67890", imageTag)

	// assert a control plane that records no image leaves both values empty
	imageNamespace, imageTag, err = moduleBlueGreenImage("", "", &api.ControlPlaneInstance{})
	require.NoError(t, err)
	require.Equal(t, "", imageNamespace)
	require.Equal(t, "", imageTag)

	// assert recorded component images fill empty instance fields
	recorded := &api.ControlPlaneInstance{
		CustomComponentInfo: []*api.ControlPlaneComponent{{
			Name:           "rest-api",
			ImageNamespace: "registry.example/threeport",
			ImageTag:       "recorded1",
		}},
	}
	imageNamespace, imageTag, err = moduleBlueGreenImage("", "", recorded)
	require.NoError(t, err)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "recorded1", imageTag)
}

// TestModuleBlueGreenRejectsDifferingComponentImages covers recorded component
// images that do not share one tag or one namespace.
func TestModuleBlueGreenRejectsDifferingComponentImages(t *testing.T) {
	// seed two components whose tags disagree and whose namespaces agree
	tags := &api.ControlPlaneInstance{
		CustomComponentInfo: []*api.ControlPlaneComponent{
			{Name: "rest-api", ImageNamespace: "registry.example/threeport", ImageTag: "a"},
			{Name: "agent", ImageNamespace: "registry.example/threeport", ImageTag: "b"},
		},
	}

	// assert empty flags reject the disagreeing tags
	_, _, err := moduleBlueGreenImage("", "", tags)
	require.EqualError(t, err, "recorded component image tags differ: pass --control-plane-image-tag")

	// assert a set tag wins and the shared namespace is kept
	imageNamespace, imageTag, err := moduleBlueGreenImage("", "def67890", tags)
	require.NoError(t, err)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "def67890", imageTag)

	// seed two components whose namespaces disagree
	namespaces := &api.ControlPlaneInstance{
		CustomComponentInfo: []*api.ControlPlaneComponent{
			{Name: "rest-api", ImageNamespace: "one.example", ImageTag: "a"},
			{Name: "agent", ImageNamespace: "two.example", ImageTag: "a"},
		},
	}

	// assert empty flags reject the disagreeing namespaces
	_, _, err = moduleBlueGreenImage("", "", namespaces)
	require.EqualError(t, err, "recorded component image namespaces differ: pass --control-plane-image-namespace")

	// assert a blank component tag does not count as a disagreement
	partial := &api.ControlPlaneInstance{
		CustomComponentInfo: []*api.ControlPlaneComponent{
			{Name: "rest-api", ImageNamespace: "registry.example/threeport", ImageTag: "only"},
			{Name: "agent", ImageNamespace: "registry.example/threeport"},
		},
	}
	imageNamespace, imageTag, err = moduleBlueGreenImage("", "", partial)
	require.NoError(t, err)
	require.Equal(t, "registry.example/threeport", imageNamespace)
	require.Equal(t, "only", imageTag)
}
