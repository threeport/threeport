package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yaml "sigs.k8s.io/yaml"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// A control plane config creates its definition before its instance. A
// component override that is going to be refused has to be refused before
// either is written, or a definition is left behind for an instance that was
// never created.

func TestControlPlaneConfig_RejectsUnknownComponentBeforeWritingAnything(t *testing.T) {
	config := ControlPlaneConfig{ControlPlane: ControlPlaneValues{
		Name:      util.Ptr("child"),
		Namespace: util.Ptr("child"),
		CustomComponentInfo: []*api_v0.ControlPlaneComponent{
			{Name: "gateway-controler", ImageTag: "v1"},
		},
	}}

	// a nil client would fail on any request, so these only return the
	// validation error if nothing was sent
	_, err := config.Create(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"gateway-controler" is not a default component`)

	_, err = config.Replace(nil, "", "child")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"gateway-controler" is not a default component`)
}

func TestControlPlaneInstanceConfig_ValidateComponentInfo(t *testing.T) {
	valid := func(components ...*api_v0.ControlPlaneComponent) ControlPlaneInstanceConfig {
		return ControlPlaneInstanceConfig{ControlPlaneInstance: ControlPlaneInstanceValues{
			Name:                   util.Ptr("child"),
			Namespace:              util.Ptr("child"),
			ControlPlaneDefinition: &ControlPlaneDefinitionValues{Name: util.Ptr("child")},
			CustomComponentInfo:    components,
		}}
	}

	t.Run("default component override", func(t *testing.T) {
		config := valid(&api_v0.ControlPlaneComponent{Name: "gateway-controller", Enabled: util.Ptr(false)})
		assert.NoError(t, config.Validate())
	})

	t.Run("additional component", func(t *testing.T) {
		config := valid(&api_v0.ControlPlaneComponent{
			Name: "mine", Additional: util.Ptr(true),
			ImageName: "mine", ImageNamespace: "ghcr.io/me", ImageTag: "v1",
		})
		assert.NoError(t, config.Validate())
	})

	t.Run("unknown component", func(t *testing.T) {
		config := valid(&api_v0.ControlPlaneComponent{Name: "mine", ImageTag: "v1"})
		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set Additional")
	})
}

// The shipped samples are what users copy, so they have to be accepted as
// written.
func TestControlPlaneSamples_AreAccepted(t *testing.T) {
	for _, sample := range []string{"control-plane.yaml", "control-plane-local.yaml"} {
		t.Run(sample, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "..", "samples", "control-plane", sample))
			require.NoError(t, err)

			var config ControlPlaneConfig
			require.NoError(t, yaml.UnmarshalStrict(content, &config))
			assert.NoError(t, threeport.ValidateComponentOverrides(config.ControlPlane.CustomComponentInfo))
		})
	}
}
