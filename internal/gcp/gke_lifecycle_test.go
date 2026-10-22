package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// TestBuildGkeInfra_MapsDefinitionNodePoolFields is a regression test for the
// GcpGkeKubernetesRuntimeDefinition -> KubernetesRuntimeInfraGKE mapping in
// buildGkeInfra. It asserts MachineType, MinNodeCount, and MaxNodeCount are
// correctly read from the definition, since a prior version of this function
// silently dropped these fields, leaving the Pulumi node pool with hardcoded
// values regardless of what the definition requested.
func TestBuildGkeInfra_MapsDefinitionNodePoolFields(t *testing.T) {
	projectID := "some-project"
	gcpProviderID := uint(1)

	// credentials are part of a provider that can provision: the workload
	// identity binding made after the cluster is created names the account they
	// belong to, and buildGkeInfra refuses to build infra it could not bind
	serviceAccountCredentials := `{"type":"service_account","project_id":"some-project","client_email":"threeport@some-project.iam.gserviceaccount.com"}`

	gcpProvider := api_v0.GcpProvider{
		Common:                    api_v0.Common{ID: &gcpProviderID},
		ProjectID:                 &projectID,
		ServiceAccountCredentials: &serviceAccountCredentials,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{gcpProvider},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	r := &controller.Reconciler{
		APIClient: &http.Client{},
		APIServer: strings.TrimPrefix(srv.URL, "http://"),
	}

	instanceName := "test-instance"
	region := "us-east1"
	instance := &api_v0.GcpGkeKubernetesRuntimeInstance{
		Instance:      api_v0.Instance{Name: &instanceName},
		Region:        &region,
		GcpProviderID: &gcpProviderID,
	}

	machineType := "e2-standard-4"
	initialSize := 2
	minSize := 3
	maxSize := 7
	definition := &api_v0.GcpGkeKubernetesRuntimeDefinition{
		DefaultNodeGroupInstanceType: &machineType,
		DefaultNodeGroupInitialSize:  &initialSize,
		DefaultNodeGroupMinimumSize:  &minSize,
		DefaultNodeGroupMaximumSize:  &maxSize,
	}

	infra, err := buildGkeInfra(r, instance, definition, nil)
	require.NoError(t, err)

	assert.Equal(t, "e2-standard-4", infra.MachineType)
	assert.Equal(t, int32(2), infra.WorkerNodeInitialCount)
	assert.Equal(t, int32(3), infra.MinNodeCount)
	assert.Equal(t, int32(7), infra.MaxNodeCount)
}

// TestBuildGkeInfra_RefusesWithoutAnAccountToBind covers a provider with no
// stored credentials, built somewhere with no ambient identity either.
//
// The workload identity binding made after the cluster exists names a service
// account. With neither source for it, that binding cannot be made - and
// finding that out afterwards means the network, control plane and node pool
// have already been provisioned. It fails here instead.
func TestBuildGkeInfra_RefusesWithoutAnAccountToBind(t *testing.T) {
	projectID := "some-project"
	providerName := "some-provider"
	gcpProviderID := uint(1)

	gcpProvider := api_v0.GcpProvider{
		Common:    api_v0.Common{ID: &gcpProviderID},
		Name:      &providerName,
		ProjectID: &projectID,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{gcpProvider},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	r := &controller.Reconciler{
		APIClient: &http.Client{},
		APIServer: strings.TrimPrefix(srv.URL, "http://"),
	}

	instanceName := "test-instance"
	region := "us-east1"
	instance := &api_v0.GcpGkeKubernetesRuntimeInstance{
		Instance:      api_v0.Instance{Name: &instanceName},
		Region:        &region,
		GcpProviderID: &gcpProviderID,
	}

	machineType := "e2-standard-4"
	initialSize := 2
	minSize := 3
	maxSize := 7
	definition := &api_v0.GcpGkeKubernetesRuntimeDefinition{
		DefaultNodeGroupInstanceType: &machineType,
		DefaultNodeGroupInitialSize:  &initialSize,
		DefaultNodeGroupMinimumSize:  &minSize,
		DefaultNodeGroupMaximumSize:  &maxSize,
	}

	_, err := buildGkeInfra(r, instance, definition, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ambient service account could be resolved")
}

// TestRequireSameProject covers the project a workload identity binding is
// addressed through.
//
// The binding is made at projects/<project>/serviceAccounts/<account>, so an
// account from another project is not named by that path - and the failure
// lands after the cluster is provisioned.
func TestRequireSameProject(t *testing.T) {
	t.Run("an account in the runtime's project passes", func(t *testing.T) {
		require.NoError(t, requireSameProject(
			"threeport@some-project.iam.gserviceaccount.com", "some-project", "a-provider",
		))
	})

	t.Run("an account in another project is refused", func(t *testing.T) {
		err := requireSameProject(
			"threeport@control-plane-project.iam.gserviceaccount.com", "runtime-project", "a-provider",
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control-plane-project")
		assert.Contains(t, err.Error(), "runtime-project")
	})

	// an address whose project cannot be read is refused rather than guessed at
	t.Run("an account that is not a project service account is refused", func(t *testing.T) {
		require.Error(t, requireSameProject("someone@example.com", "some-project", "a-provider"))
		require.Error(t, requireSameProject("not-an-email", "some-project", "a-provider"))
	})
}

// gkeProviderServer serves a GcpProvider, so buildGkeInfra can be driven end to
// end.
func gkeProviderServer(t *testing.T, gcpProvider api_v0.GcpProvider) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{gcpProvider},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

// TestBuildGkeInfra_RefusesAnAmbientAccountFromAnotherProject drives the project
// check through buildGkeInfra rather than calling it directly.
//
// Testing the check alone would pass whether or not anything calls it, which is
// the failure this is guarding: a control plane in one project creating runtimes
// in another binds at a path that does not name its account, and finds out after
// the cluster exists.
func TestBuildGkeInfra_RefusesAnAmbientAccountFromAnotherProject(t *testing.T) {
	restore := gcpauth.SetAmbientServiceAccountEmailForTest(func(context.Context) (string, error) {
		return "threeport@control-plane-project.iam.gserviceaccount.com", nil
	})
	defer restore()

	projectID := "runtime-project"
	providerName := "some-provider"
	gcpProviderID := uint(1)

	srv := gkeProviderServer(t, api_v0.GcpProvider{
		Common:    api_v0.Common{ID: &gcpProviderID},
		Name:      &providerName,
		ProjectID: &projectID,
	})
	defer srv.Close()

	instanceName := "test-instance"
	region := "us-east1"
	machineType := "e2-standard-4"
	initialSize := 2
	minSize := 3
	maxSize := 7

	_, err := buildGkeInfra(
		&controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(srv.URL, "http://"),
		},
		&api_v0.GcpGkeKubernetesRuntimeInstance{
			Instance:      api_v0.Instance{Name: &instanceName},
			Region:        &region,
			GcpProviderID: &gcpProviderID,
		},
		&api_v0.GcpGkeKubernetesRuntimeDefinition{
			DefaultNodeGroupInstanceType: &machineType,
			DefaultNodeGroupInitialSize:  &initialSize,
			DefaultNodeGroupMinimumSize:  &minSize,
			DefaultNodeGroupMaximumSize:  &maxSize,
		},
		nil,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "control-plane-project")
	assert.Contains(t, err.Error(), "runtime-project")
}

// TestBuildGkeInfra_RefusesStoredCredentialsFromAnotherProject covers a provider
// whose stored credentials name an account in a different project.
//
// The binding is addressed through the runtime's project whatever the account's
// source, so this reaches the same path that never names it - and the same
// failure after the cluster is already provisioned. Only the ambient route was
// checked before.
func TestBuildGkeInfra_RefusesStoredCredentialsFromAnotherProject(t *testing.T) {
	projectID := "runtime-project"
	providerName := "some-provider"
	gcpProviderID := uint(1)
	credentials := `{"type":"service_account","project_id":"another-project","client_email":"threeport@another-project.iam.gserviceaccount.com"}`

	srv := gkeProviderServer(t, api_v0.GcpProvider{
		Common:                    api_v0.Common{ID: &gcpProviderID},
		Name:                      &providerName,
		ProjectID:                 &projectID,
		ServiceAccountCredentials: &credentials,
	})
	defer srv.Close()

	instanceName := "test-instance"
	region := "us-east1"
	machineType := "e2-standard-4"
	initialSize := 2
	minSize := 3
	maxSize := 7

	_, err := buildGkeInfra(
		&controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(srv.URL, "http://"),
		},
		&api_v0.GcpGkeKubernetesRuntimeInstance{
			Instance:      api_v0.Instance{Name: &instanceName},
			Region:        &region,
			GcpProviderID: &gcpProviderID,
		},
		&api_v0.GcpGkeKubernetesRuntimeDefinition{
			DefaultNodeGroupInstanceType: &machineType,
			DefaultNodeGroupInitialSize:  &initialSize,
			DefaultNodeGroupMinimumSize:  &minSize,
			DefaultNodeGroupMaximumSize:  &maxSize,
		},
		nil,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "another-project")
	assert.Contains(t, err.Error(), "runtime-project")
}

// TestBuildGkeInfra_PulumiProjectSetAtConstruction covers the state file path
// the crash-recovery watcher is built from.
//
// executeInfraCreate starts the fsnotify watcher that streams partial state to
// the API before it calls DeployInfra, and GetStateFilePath builds the watched
// path from ProjectName. Filling that in later - from CreateInfra, as it used
// to be - leaves the watcher reading an empty name, so it watches one directory
// above where Pulumi writes and never fires. A controller that then crashes
// mid-provision has persisted nothing, and the retry starts over with the first
// attempt's resources still in the project. Fixed for EKS in #572; this is the
// same pattern.
func TestBuildGkeInfra_PulumiProjectSetAtConstruction(t *testing.T) {
	projectID := "some-project"
	gcpProviderID := uint(1)
	serviceAccountCredentials := `{"type":"service_account","project_id":"some-project","client_email":"threeport@some-project.iam.gserviceaccount.com"}`

	gcpProvider := api_v0.GcpProvider{
		Common:                    api_v0.Common{ID: &gcpProviderID},
		ProjectID:                 &projectID,
		ServiceAccountCredentials: &serviceAccountCredentials,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{gcpProvider},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	instanceName := "test-instance"
	region := "us-east1"
	machineType := "e2-standard-4"
	size := 2
	infra, err := buildGkeInfra(
		&controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(srv.URL, "http://"),
		},
		&api_v0.GcpGkeKubernetesRuntimeInstance{
			Instance:      api_v0.Instance{Name: &instanceName},
			Region:        &region,
			GcpProviderID: &gcpProviderID,
		},
		&api_v0.GcpGkeKubernetesRuntimeDefinition{
			DefaultNodeGroupInstanceType: &machineType,
			DefaultNodeGroupInitialSize:  &size,
			DefaultNodeGroupMinimumSize:  &size,
			DefaultNodeGroupMaximumSize:  &size,
		},
		nil,
	)
	require.NoError(t, err)

	require.NotEmpty(
		t, infra.ProjectName,
		"the project name has to be set before the lifecycle starts watching for state",
	)

	statePath, err := infra.GetStateFilePath()
	require.NoError(t, err)
	assert.Contains(
		t, statePath, filepath.Join("stacks", infra.ProjectName),
		"the watched path must name the project directory Pulumi writes into",
	)

	// the project and region reach the stack config at construction too, for
	// the same reason: nothing may write to the infra once the watcher reads it
	assert.Equal(t, infra.ProjectID, infra.StackConfigs["gcp:project"])
	assert.Equal(t, infra.Region, infra.StackConfigs["gcp:region"])
}

// gkeInstanceServer serves one GcpGkeKubernetesRuntimeInstance, so
// IsCreateComplete can read the inventory the way the controller does.
func gkeInstanceServer(t *testing.T, instance api_v0.GcpGkeKubernetesRuntimeInstance) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   []apiserver_lib.Object{instance},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

// newTestGkeLifecycle returns a lifecycle reading an instance that holds the
// inventory, with GCP stood in for by ready. The returned counter is how many
// times GCP was asked.
func newTestGkeLifecycle(
	t *testing.T,
	inventory *datatypes.JSON,
	ready func() (bool, error),
) (*gkeLifecycle, *atomic.Int32) {
	t.Helper()

	instanceID := uint(7)
	srv := gkeInstanceServer(t, api_v0.GcpGkeKubernetesRuntimeInstance{
		Common:            api_v0.Common{ID: &instanceID},
		ResourceInventory: inventory,
	})
	t.Cleanup(srv.Close)

	asked := &atomic.Int32{}
	logger := logr.Discard()
	g := newGkeLifecycleProvider(
		&controller.Reconciler{
			APIClient: &http.Client{},
			APIServer: strings.TrimPrefix(srv.URL, "http://"),
		},
		&api_v0.GcpGkeKubernetesRuntimeInstance{Common: api_v0.Common{ID: &instanceID}},
		&logger,
	)
	g.createReady = func(*api_v0.GcpGkeKubernetesRuntimeInstance) (bool, error) {
		asked.Add(1)
		return ready()
	}

	return g, asked
}

// TestGkeIsCreateComplete_PartialStateIsNotComplete covers the state the stack
// streams while the create is still running. The inventory is non-empty, so it
// was enough to confirm the create before the cluster had an endpoint.
func TestGkeIsCreateComplete_PartialStateIsNotComplete(t *testing.T) {
	partial := datatypes.JSON(`{"version":3,"deployment":{"resources":[{"urn":"urn:pulumi:stack::gke::gcp:compute/network:Network::net"}]}}`)
	g, asked := newTestGkeLifecycle(t, &partial, func() (bool, error) { return false, nil })

	complete, err := g.IsCreateComplete()

	require.NoError(t, err)
	assert.False(t, complete)
	assert.Equal(t, int32(1), asked.Load(), "non-empty state must still be checked against GCP")
}

// TestGkeIsCreateComplete_CompleteOnceGcpSaysSo covers the same state once the
// create has run to its last step.
func TestGkeIsCreateComplete_CompleteOnceGcpSaysSo(t *testing.T) {
	state := datatypes.JSON(`{"version":3,"deployment":{"resources":[{"urn":"urn:x"}]}}`)
	g, _ := newTestGkeLifecycle(t, &state, func() (bool, error) { return true, nil })

	complete, err := g.IsCreateComplete()

	require.NoError(t, err)
	assert.True(t, complete)
}

// TestGkeIsCreateComplete_NoStateSkipsGcp covers a create that has saved
// nothing, which cannot be complete and needs no call to GCP.
func TestGkeIsCreateComplete_NoStateSkipsGcp(t *testing.T) {
	empty := datatypes.JSON(`{}`)
	for name, inventory := range map[string]*datatypes.JSON{
		"nil":   nil,
		"empty": &empty,
		"null":  jsonPtr(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			g, asked := newTestGkeLifecycle(t, inventory, func() (bool, error) { return true, nil })

			complete, err := g.IsCreateComplete()

			require.NoError(t, err)
			assert.False(t, complete)
			assert.Equal(t, int32(0), asked.Load())
		})
	}
}

// TestGkeIsCreateComplete_GcpErrorPropagates covers a failed check, which must
// not read as complete or as not complete.
func TestGkeIsCreateComplete_GcpErrorPropagates(t *testing.T) {
	state := datatypes.JSON(`{"deployment":{"resources":[{"urn":"urn:x"}]}}`)
	errGcp := errors.New("gcp unavailable")
	g, _ := newTestGkeLifecycle(t, &state, func() (bool, error) { return false, errGcp })

	complete, err := g.IsCreateComplete()

	require.ErrorIs(t, err, errGcp)
	assert.False(t, complete)
}

// jsonPtr returns a pointer to s as JSON bytes.
func jsonPtr(s string) *datatypes.JSON {
	j := datatypes.JSON(s)
	return &j
}
