package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestDropSameNameControlPlaneRejectsWithoutForce covers the same-name guard.
func TestDropSameNameControlPlaneRejectsWithoutForce(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{
			{Name: "keep"},
			{Name: "target"},
		},
	}

	err := dropSameNameControlPlane(cfg, "target", false)

	require.Error(err)
	require.ErrorIs(err, ErrThreeportConfigAlreadyExists)
	require.Len(cfg.ControlPlanes, 2)
}

// TestDropSameNameControlPlaneForceKeepsSiblings covers additive overwrite.
func TestDropSameNameControlPlaneForceKeepsSiblings(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{
			{Name: "keep"},
			{Name: "target"},
		},
	}

	err := dropSameNameControlPlane(cfg, "target", true)

	require.NoError(err)
	require.Equal([]ControlPlane{{Name: "keep"}}, cfg.ControlPlanes)
}

// TestDropSameNameControlPlaneMissingNameIsNoop covers a first install.
func TestDropSameNameControlPlaneMissingNameIsNoop(t *testing.T) {
	require := require.New(t)

	cfg := &ThreeportConfig{
		ControlPlanes: []ControlPlane{{Name: "keep"}},
	}

	err := dropSameNameControlPlane(cfg, "new", false)

	require.NoError(err)
	require.Equal([]ControlPlane{{Name: "keep"}}, cfg.ControlPlanes)
}

// TestEnsureBootstrapKubernetesRuntimeReusesExistingRows covers a retry.
func TestEnsureBootstrapKubernetesRuntimeReusesExistingRows(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		name := r.URL.Query().Get("name")
		fmt.Fprintf(w, `{"Data":[{"ID":9,"Name":%q}]}`, name)
	}))
	defer server.Close()

	def, inst, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(9), *def.ID)
	require.Equal(uint(9), *inst.ID)
}

// TestEnsureBootstrapKubernetesRuntimeCreatesMissingRows covers a first install.
func TestEnsureBootstrapKubernetesRuntimeCreatesMissingRows(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusCreated)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			name, _ := body["Name"].(string)
			fmt.Fprintf(w, `{"Data":[{"ID":%d,"Name":%q}]}`, posts, name)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Data":[]}`))
	}))
	defer server.Close()

	def, inst, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.NoError(err)
	require.Equal(2, posts)
	require.NotNil(def.ID)
	require.NotNil(inst.ID)
}

// TestEnsureBootstrapKubernetesRuntimeSurfacesLookupErrors covers a failed GET.
func TestEnsureBootstrapKubernetesRuntimeSurfacesLookupErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"Data":[]}`))
	}))
	defer server.Close()

	_, _, err := ensureBootstrapKubernetesRuntime(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		"threeport-dev",
		"threeport-dev",
		true,
		"kind",
		&v0.KubernetesRuntimeInstance{Instance: v0.Instance{Name: util.Ptr("threeport-dev")}},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to look up kubernetes runtime definition by name")
}
