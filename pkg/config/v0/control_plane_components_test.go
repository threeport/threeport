package v0

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestControlPlaneConfig_RejectsUnknownApis refuses an unknown group before anything is written.
func TestControlPlaneConfig_RejectsUnknownApis(t *testing.T) {
	config := ControlPlaneConfig{ControlPlane: ControlPlaneValues{
		Name: util.Ptr("primary-01"),
		Apis: util.Ptr("not-a-group"),
	}}

	// a nil client would fail on any request, so these only return the
	// validation error if nothing was sent
	_, err := config.Create(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-group")

	_, err = config.Replace(nil, "", "primary-01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-group")
}

// TestControlPlaneInstanceConfig_RejectsUnknownApis refuses an unknown group before anything is written.
func TestControlPlaneInstanceConfig_RejectsUnknownApis(t *testing.T) {
	config := instanceConfig("not-a-group")

	// call Create with a nil client
	_, err := config.Create(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-group")

	// call Replace with a nil client
	_, err = config.Replace(nil, "", "child")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-group")
}

// TestControlPlaneInstanceConfig_ReplaceRejectsApisChange refuses a change to Apis after creation.
func TestControlPlaneInstanceConfig_ReplaceRejectsApisChange(t *testing.T) {
	body := `{"Data":[{"ID":1,"Name":"child","Apis":"gcp"}]}`
	puts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	// call Replace with a different Apis value
	changed := instanceConfig("none")
	_, err := changed.Replace(http.DefaultClient, addr, "child")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Apis cannot be changed")
	assert.Equal(t, 0, puts)

	// call Replace with the stored Apis value
	same := instanceConfig("gcp")
	_, err = same.Replace(http.DefaultClient, addr, "child")
	require.NoError(t, err)
	assert.Equal(t, 1, puts)
}

// instanceConfig returns a control plane instance config with Apis set to apis.
func instanceConfig(apis string) ControlPlaneInstanceConfig {
	return ControlPlaneInstanceConfig{ControlPlaneInstance: ControlPlaneInstanceValues{
		Name:                   util.Ptr("child"),
		Namespace:              util.Ptr("child"),
		ControlPlaneDefinition: &ControlPlaneDefinitionValues{Name: util.Ptr("child")},
		Apis:                   util.Ptr(apis),
	}}
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
