package v0

import (
	"io"
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

// TestControlPlaneConfig_RefusedInstanceReplaceLeavesDefinition leaves the definition unwritten when the instance replace is refused.
func TestControlPlaneConfig_RefusedInstanceReplaceLeavesDefinition(t *testing.T) {
	// record each call and refuse every update
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"Status":{"code":400,"error":"cannot update Apis on a control plane this API does not serve"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Data":[{"ID":1,"Name":"child","Namespace":"child","Apis":"gcp"}]}`))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	// change the definition and the instance together
	config := ControlPlaneConfig{ControlPlane: ControlPlaneValues{
		Name:          util.Ptr("child"),
		Namespace:     util.Ptr("child"),
		OnboardParent: util.Ptr(false),
		Apis:          util.Ptr("none"),
	}}
	_, err := config.Replace(http.DefaultClient, addr, "child")

	// the definition is not called
	require.Error(t, err)
	assert.Equal(t, []string{
		"GET /v0/control-plane-instances",
		"PUT /v0/control-plane-instances/1",
	}, calls)
}

// TestControlPlaneConfig_RefusesTierChangeBeforeWriting refuses a tier change before either object is written.
func TestControlPlaneConfig_RefusesTierChangeBeforeWriting(t *testing.T) {
	// serve the stored definition and record any write
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"Data":[{"ID":1,"Name":"child","Tier":"development"}]}`))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	// ask for a new tier and a new controller list together, under a new name
	config := ControlPlaneConfig{ControlPlane: ControlPlaneValues{
		Name: util.Ptr("renamed"),
		Apis: util.Ptr("none"),
		Tier: util.Ptr(api_v0.ControlPlaneTierProduction),
	}}
	_, err := config.Replace(http.DefaultClient, addr, "child")

	// the stored definition is the replace name, and nothing is written
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be changed")
	assert.Contains(t, err.Error(), api_v0.ControlPlaneTierDevelopment)
	assert.Contains(t, err.Error(), api_v0.ControlPlaneTierProduction)
	assert.Equal(t, []string{
		"GET /v0/control-plane-definitions?name=child",
	}, calls)
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

// TestControlPlaneInstanceConfig_ReplaceWritesApisChange writes a new Apis value.
func TestControlPlaneInstanceConfig_ReplaceWritesApisChange(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			raw, readErr := io.ReadAll(r.Body)
			require.NoError(t, readErr)
			putBody = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Data":[{"ID":1,"Name":"child","Apis":"gcp"}]}`))
	}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "http://")

	// call Replace with a different Apis value
	changed := instanceConfig("none")
	_, err := changed.Replace(http.DefaultClient, addr, "child")
	require.NoError(t, err)
	assert.Contains(t, putBody, `"Apis":"none"`)
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
