package v0

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestGkeClusterNameUsesTheRecordedCluster checks that a shared cluster is not renamed from the control plane.
func TestGkeClusterNameUsesTheRecordedCluster(t *testing.T) {
	recorded := &ControlPlane{
		Name:              "genesis-02",
		GKEProviderConfig: GKEProviderConfig{ClusterName: "threeport-genesis-01"},
	}
	require.Equal(t, "threeport-genesis-01", gkeClusterName(recorded))

	named := &ControlPlane{Name: "genesis-01"}
	require.Equal(t, "threeport-genesis-01", gkeClusterName(named))
}

// TestGkeInfraForUpgradeUsesRecordedLocation checks that a filled-in config is not replaced from the stack.
func TestGkeInfraForUpgradeUsesRecordedLocation(t *testing.T) {
	// use the project and region on the control plane
	infra, err := GkeInfraForUpgrade(&ControlPlane{
		Name:     "genesis-01",
		Provider: v0.KubernetesRuntimeInfraProviderGKE,
		GKEProviderConfig: GKEProviderConfig{
			ClusterName:  "threeport-genesis-01",
			GcpProjectId: "test-project",
			GcpRegion:    "us-central1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "threeport-genesis-01", infra.RuntimeInstanceName)
	require.Equal(t, "test-project", infra.ProjectID)
	require.Equal(t, "us-central1", infra.Region)
}

// TestGkeInfraForUpgradeRejectsOtherProviders checks that only a gke control plane is upgraded in place.
func TestGkeInfraForUpgradeRejectsOtherProviders(t *testing.T) {
	// a missing control plane is an error
	_, err := GkeInfraForUpgrade(nil)
	require.Error(t, err)

	// a non-gke control plane is an error
	_, err = GkeInfraForUpgrade(&ControlPlane{
		Name:     "dev",
		Provider: v0.KubernetesRuntimeInfraProviderEKS,
	})
	require.Error(t, err)
}

// TestEnsureGcpProviderReusesNamedRow covers a retry that would
// otherwise POST a duplicate provider.
func TestEnsureGcpProviderReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":4,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := ensureGcpProvider(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpProvider{Name: util.Ptr("default")},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(4), *got.ID)
}

// TestEnsureGcpGkeKubernetesRuntimeDefinitionReusesNamedRow covers
// a retry of the GKE definition create.
func TestEnsureGcpGkeKubernetesRuntimeDefinitionReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":5,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := ensureGcpGkeKubernetesRuntimeDefinition(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpGkeKubernetesRuntimeDefinition{
			Definition: v0.Definition{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(5), *got.ID)
}

// TestEnsureGcpGkeKubernetesRuntimeInstanceReusesNamedRow covers a
// retry of the GKE instance create.
func TestEnsureGcpGkeKubernetesRuntimeInstanceReusesNamedRow(t *testing.T) {
	require := require.New(t)
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"Data":[{"ID":6,"Name":%q}]}`, r.URL.Query().Get("name"))
	}))
	defer server.Close()

	got, err := ensureGcpGkeKubernetesRuntimeInstance(
		server.Client(),
		strings.TrimPrefix(server.URL, "http://"),
		&v0.GcpGkeKubernetesRuntimeInstance{
			Instance: v0.Instance{Name: util.Ptr("threeport-dev")},
		},
	)

	require.NoError(err)
	require.Equal(0, posts)
	require.Equal(uint(6), *got.ID)
}

// TestFillClusterLocationCopiesTheHost checks that an empty child entry takes the host cluster location.
func TestFillClusterLocationCopiesTheHost(t *testing.T) {
	// a child on the host API has no location of its own
	host := ControlPlane{
		Name:      "genesis-02",
		Provider:  v0.KubernetesRuntimeInfraProviderGKE,
		APIServer: "https://genesis.example",
		GKEProviderConfig: GKEProviderConfig{
			GcpProjectId: "qleet-shared",
			GcpRegion:    "us-central1",
			ClusterName:  "threeport-genesis-01",
			BackupBucket: "threeport-genesis-01-backups-qleet-shared",
		},
	}
	child := &ControlPlane{Name: "primary-02"}
	cfg := &ThreeportConfig{ControlPlanes: []ControlPlane{host}}

	// copy the host location
	FillClusterLocation(cfg, host.APIServer, child)

	require.Equal(t, host.Provider, child.Provider)
	require.Equal(t, host.GKEProviderConfig, child.GKEProviderConfig)

	// keep a location the child already records
	child.GKEProviderConfig.ClusterName = "other"
	FillClusterLocation(cfg, host.APIServer, child)
	require.Equal(t, "other", child.GKEProviderConfig.ClusterName)
}

// TestRestoringNamedControlPlane checks which namespace is the control plane being restored.
func TestRestoringNamedControlPlane(t *testing.T) {
	// an empty namespace and the control plane name are that control plane
	require.True(t, RestoringNamedControlPlane("genesis-02", ""))
	require.True(t, RestoringNamedControlPlane("genesis-02", "genesis-02"))

	// another namespace is a control plane on the same cluster
	require.False(t, RestoringNamedControlPlane("genesis-02", "primary-02"))
}
