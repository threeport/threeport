package v0

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestRedeployInstalledModuleSkipsCore checks that a core module is not retargeted.
func TestRedeployInstalledModuleSkipsCore(t *testing.T) {
	// return before any cluster call
	cpi := &ControlPlaneInstaller{}
	err := cpi.redeployInstalledModule(nil, nil, api.ModuleApi{Core: util.Ptr(true)}, nil, time.Time{})
	require.NoError(t, err)
}

// TestModuleDeploymentNamesUsesTheInstalledName checks the module-prefixed deployment name.
func TestModuleDeploymentNamesUsesTheInstalledName(t *testing.T) {
	// the install puts the module in front of the recorded controller name
	names := moduleDeploymentNames("threeport-core-controller", "acme")
	if len(names) != 2 || names[1] != "threeport-acme-core-controller" {
		t.Fatalf("names = %v", names)
	}
}

// TestRecordedModuleImages checks when a retarget is allowed to change images.
func TestRecordedModuleImages(t *testing.T) {
	// require both values and skip a core module
	namespace, tag := recordedModuleImages(&api.ModuleApi{
		ImageNamespace: util.Ptr("repo.example"),
		ImageTag:       util.Ptr("v1"),
	})
	assert.Equal(t, "repo.example", namespace)
	assert.Equal(t, "v1", tag)

	namespace, tag = recordedModuleImages(&api.ModuleApi{
		Core:           util.Ptr(true),
		ImageNamespace: util.Ptr("repo.example"),
		ImageTag:       util.Ptr("v1"),
	})
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", tag)

	namespace, tag = recordedModuleImages(&api.ModuleApi{
		ImageNamespace: util.Ptr("repo.example"),
	})
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", tag)
}

// TestSplitDeploymentName checks a recorded controller deployment reference.
func TestSplitDeploymentName(t *testing.T) {
	// accept namespace/name and reject anything else
	namespace, name, err := splitDeploymentName("acme/threeport-acme-widget-controller")
	require.NoError(t, err)
	assert.Equal(t, "acme", namespace)
	assert.Equal(t, "threeport-acme-widget-controller", name)

	_, _, err = splitDeploymentName("threeport-acme-widget-controller")
	require.Error(t, err)
}
