package controlplane

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestSeedMissingChildWritesTheHostRuntime checks an empty child database
// gains one host runtime when the parent reconciles it.
func TestSeedMissingChildWritesTheHostRuntime(t *testing.T) {
	// stand up a parent that knows the child and a child with an empty database
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parent.seed(v0.PathKubernetesRuntimeDefinitions, map[string]any{
		"ID": 3, "Name": "cluster", "InfraProvider": v0.KubernetesRuntimeInfraProviderKind,
	})
	parent.seed(v0.PathKubernetesRuntimeInstances, map[string]any{
		"ID": 4, "Name": "cluster", "KubernetesRuntimeDefinitionID": 3, "ThreeportControlPlaneHost": true,
	})
	parent.seed(v0.PathControlPlaneDefinitions, map[string]any{"ID": 2, "Name": "def"})
	childEndpoint := strings.TrimPrefix(child.server.URL, "http://")
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 6, "Name": "child", "Namespace": "child-ns", "IsSelf": false,
		"ApiServerEndpoint":           childEndpoint,
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 1, "Name": "serving", "Namespace": "serving-ns", "IsSelf": true,
		"ApiServerEndpoint":           parent.target().endpoint,
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})

	reconciler := &controller.Reconciler{
		APIClient: parent.server.Client(),
		APIServer: parent.target().endpoint,
	}
	stored := &v0.ControlPlaneInstance{Common: v0.Common{ID: util.Ptr(uint(6))}}
	require.NoError(t, syncManagedChild(reconciler, stored))

	created := child.createdOn(v0.PathKubernetesRuntimeInstances)
	hosts := 0
	for _, runtime := range created {
		if runtime["ThreeportControlPlaneHost"] == true {
			hosts++
		}
	}
	require.Equal(t, 1, hosts)
	instances := child.createdOn(v0.PathControlPlaneInstances)
	foundSelf := false
	for _, instance := range instances {
		if instance["Name"] == "child" && instance["IsSelf"] == true {
			foundSelf = true
		}
	}
	require.True(t, foundSelf)
}

// TestSyncCopiesTheManagingParentWhenTheSelfRowExists checks a child that
// already has its own row still receives the managing parent.
func TestSyncCopiesTheManagingParentWhenTheSelfRowExists(t *testing.T) {
	// stand up a child that already serves itself and has no parent row
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parent.seed(v0.PathKubernetesRuntimeDefinitions, map[string]any{
		"ID": 3, "Name": "cluster", "InfraProvider": v0.KubernetesRuntimeInfraProviderKind,
	})
	parent.seed(v0.PathKubernetesRuntimeInstances, map[string]any{
		"ID": 4, "Name": "cluster", "KubernetesRuntimeDefinitionID": 3,
	})
	parent.seed(v0.PathControlPlaneDefinitions, map[string]any{"ID": 2, "Name": "def"})
	childEndpoint := strings.TrimPrefix(child.server.URL, "http://")
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 6, "Name": "child", "Namespace": "child-ns", "IsSelf": false,
		"ApiServerEndpoint":           childEndpoint,
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 1, "Name": "serving", "Namespace": "serving-ns", "IsSelf": true,
		"ApiServerEndpoint":           parent.target().endpoint,
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})
	child.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 9, "Name": "child", "Namespace": "child-ns", "IsSelf": true,
	})

	reconciler := &controller.Reconciler{
		APIClient: parent.server.Client(),
		APIServer: parent.target().endpoint,
	}
	stored := &v0.ControlPlaneInstance{Common: v0.Common{ID: util.Ptr(uint(6))}}
	require.NoError(t, syncManagedChild(reconciler, stored))

	foundParent := false
	for _, instance := range child.createdOn(v0.PathControlPlaneInstances) {
		if instance["Name"] == "serving" {
			foundParent = true
		}
	}
	require.True(t, foundParent)
}

// TestSyncRefreshesAStaleGenesisCopy checks a child keeps the genesis name
// the parent is serving after the parent moves.
func TestSyncRefreshesAStaleGenesisCopy(t *testing.T) {
	// a parent serving genesis-06 and a child still holding genesis-05
	parent := newFakeAPI(t, 1)
	child := newFakeAPI(t, 500)
	parent.seed(v0.PathKubernetesRuntimeDefinitions, map[string]any{
		"ID": 3, "Name": "cluster", "InfraProvider": v0.KubernetesRuntimeInfraProviderKind,
	})
	parent.seed(v0.PathKubernetesRuntimeInstances, map[string]any{
		"ID": 4, "Name": "cluster", "KubernetesRuntimeDefinitionID": 3,
	})
	parent.seed(v0.PathControlPlaneDefinitions, map[string]any{"ID": 2, "Name": "def"})
	childEndpoint := strings.TrimPrefix(child.server.URL, "http://")
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 6, "Name": "primary-12", "Namespace": "primary-12", "IsSelf": false,
		"ApiServerEndpoint":           childEndpoint,
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})
	parent.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 1, "Name": "genesis-06", "Namespace": "genesis-06", "IsSelf": true, "Genesis": true,
		"ApiServerEndpoint":           "10.0.0.6:443",
		"ControlPlaneDefinitionID":    2,
		"KubernetesRuntimeInstanceID": 4,
	})
	parentID := 8
	child.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": 9, "Name": "primary-12", "Namespace": "primary-12", "IsSelf": true,
		"ParentControlPlaneInstanceID": parentID,
	})
	child.seed(v0.PathControlPlaneInstances, map[string]any{
		"ID": parentID, "Name": "genesis-05", "Namespace": "genesis-05", "IsSelf": false, "Genesis": true,
		"ApiServerEndpoint": "10.0.0.5:443",
	})

	reconciler := &controller.Reconciler{
		APIClient: parent.server.Client(),
		APIServer: parent.target().endpoint,
	}
	stored := &v0.ControlPlaneInstance{Common: v0.Common{ID: util.Ptr(uint(6))}}
	require.NoError(t, syncManagedChild(reconciler, stored))

	// the copied genesis row takes the name the parent is serving
	renamed := false
	for _, patch := range child.patchedOn(v0.PathControlPlaneInstances) {
		if patch["Name"] == "genesis-06" && patch["ApiServerEndpoint"] == "10.0.0.6:443" {
			renamed = true
		}
	}
	require.True(t, renamed)
}
