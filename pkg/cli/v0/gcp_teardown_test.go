package v0

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

func TestGcpProvidersToCleanUp(t *testing.T) {
	providers := []v0.GcpProvider{
		{Name: util.Ptr("one"), ProjectID: util.Ptr("proj-a")},
		{Name: util.Ptr("no-project")},
		{ProjectID: util.Ptr("no-name")},
		{Name: util.Ptr("two"), ProjectID: util.Ptr("proj-b")},
		// created before the setting was stored: cleaned up as before
		{Name: util.Ptr("legacy"), ProjectID: util.Ptr("proj-c"), SyncServiceAccount: nil},
		{Name: util.Ptr("synced"), ProjectID: util.Ptr("proj-d"), SyncServiceAccount: util.Ptr(true)},
		// the service account is managed outside Threeport
		{Name: util.Ptr("kept"), ProjectID: util.Ptr("proj-e"), SyncServiceAccount: util.Ptr(false)},
	}

	assert.Equal(t, []PendingGcpCleanup{
		{Name: "one", ProjectID: "proj-a"},
		{Name: "two", ProjectID: "proj-b"},
		{Name: "legacy", ProjectID: "proj-c"},
		{Name: "synced", ProjectID: "proj-d"},
	}, gcpProvidersToCleanUp(providers))
}

func TestListPendingGcpCleanup(t *testing.T) {
	providers := &[]v0.GcpProvider{{Name: util.Ptr("one"), ProjectID: util.Ptr("proj-a")}}

	t.Run("lists the providers to clean up", func(t *testing.T) {
		pending, _, err := listPendingGcpCleanup(func() (*[]v0.GcpProvider, error) { return providers, nil })
		require.NoError(t, err)
		assert.Equal(t, []PendingGcpCleanup{{Name: "one", ProjectID: "proj-a"}}, pending)
	})

	t.Run("a provider created with SyncServiceAccount false is not queued", func(t *testing.T) {
		pending, kept, err := listPendingGcpCleanup(func() (*[]v0.GcpProvider, error) {
			return &[]v0.GcpProvider{
				{Name: util.Ptr("one"), ProjectID: util.Ptr("proj-a")},
				{Name: util.Ptr("kept"), ProjectID: util.Ptr("proj-b"), SyncServiceAccount: util.Ptr(false)},
			}, nil
		})
		require.NoError(t, err)
		assert.Equal(t, []PendingGcpCleanup{{Name: "one", ProjectID: "proj-a"}}, pending)
		assert.Equal(t, []PendingGcpCleanup{{Name: "kept", ProjectID: "proj-b"}}, kept)
	})

	t.Run("a server without the route has nothing to clean up", func(t *testing.T) {
		pending, _, err := listPendingGcpCleanup(func() (*[]v0.GcpProvider, error) {
			return &[]v0.GcpProvider{}, fmt.Errorf(
				"call to threeport API returned unexpected response: %w: 404 page not found",
				client_lib.ErrObjectNotFound,
			)
		})
		require.NoError(t, err)
		assert.Empty(t, pending)
	})

	t.Run("any other failure stops teardown and names the way past it", func(t *testing.T) {
		cause := errors.New("connection refused")
		pending, _, err := listPendingGcpCleanup(func() (*[]v0.GcpProvider, error) {
			return &[]v0.GcpProvider{}, cause
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, cause)
		assert.Contains(t, err.Error(), "--infra-only")
		assert.Empty(t, pending)
	})
}

func TestGcpProvidersToKeep(t *testing.T) {
	providers := []v0.GcpProvider{
		{Name: util.Ptr("legacy"), ProjectID: util.Ptr("proj-a")},
		{Name: util.Ptr("synced"), ProjectID: util.Ptr("proj-b"), SyncServiceAccount: util.Ptr(true)},
		{Name: util.Ptr("kept"), ProjectID: util.Ptr("proj-c"), SyncServiceAccount: util.Ptr(false)},
		{Name: util.Ptr("kept-no-project"), SyncServiceAccount: util.Ptr(false)},
	}

	assert.Equal(t, []PendingGcpCleanup{{Name: "kept", ProjectID: "proj-c"}}, gcpProvidersToKeep(providers))
}

// An earlier teardown attempt saves its list before the control plane is torn
// down. If it fails and the user then opts a provider out, the retry must drop
// that provider's saved entry, or the cleanup deletes the account the user kept.
func TestReconcilePendingGcpCleanup(t *testing.T) {
	one := PendingGcpCleanup{Name: "one", ProjectID: "proj-a"}
	kept := PendingGcpCleanup{Name: "kept", ProjectID: "proj-b"}
	gone := PendingGcpCleanup{Name: "gone", ProjectID: "proj-c"}
	added := PendingGcpCleanup{Name: "added", ProjectID: "proj-d"}

	t.Run("drops a saved entry whose provider now keeps its account", func(t *testing.T) {
		got := reconcilePendingGcpCleanup([]PendingGcpCleanup{one, kept}, []PendingGcpCleanup{one}, []PendingGcpCleanup{kept})
		assert.Equal(t, []PendingGcpCleanup{one}, got)
	})

	t.Run("drops it when nothing else is left to clean up", func(t *testing.T) {
		got := reconcilePendingGcpCleanup([]PendingGcpCleanup{kept}, nil, []PendingGcpCleanup{kept})
		assert.Empty(t, got)
	})

	t.Run("keeps an entry for a provider that is no longer listed", func(t *testing.T) {
		got := reconcilePendingGcpCleanup([]PendingGcpCleanup{gone}, nil, nil)
		assert.Equal(t, []PendingGcpCleanup{gone}, got)
	})

	t.Run("adds newly listed providers once", func(t *testing.T) {
		got := reconcilePendingGcpCleanup([]PendingGcpCleanup{one}, []PendingGcpCleanup{one, added}, nil)
		assert.Equal(t, []PendingGcpCleanup{one, added}, got)
	})

	t.Run("an entry only matches the provider of the same name and project", func(t *testing.T) {
		other := PendingGcpCleanup{Name: "kept", ProjectID: "other-project"}
		got := reconcilePendingGcpCleanup([]PendingGcpCleanup{other}, nil, []PendingGcpCleanup{kept})
		assert.Equal(t, []PendingGcpCleanup{other}, got)
	})

	t.Run("nothing to change leaves the saved entries as they were", func(t *testing.T) {
		existing := []PendingGcpCleanup{one, gone}
		assert.Equal(t, existing, reconcilePendingGcpCleanup(existing, []PendingGcpCleanup{one}, nil))
	})
}

func TestRunGcpCleanups(t *testing.T) {
	pending := []PendingGcpCleanup{
		{Name: "ok", ProjectID: "p"},
		{Name: "foreign", ProjectID: "p"},
		{Name: "broken", ProjectID: "p"},
	}
	cleanup := func(_, name string) error {
		switch name {
		case "foreign":
			return fmt.Errorf("%w: someone else's", provider.ErrGcpServiceAccountNotOwned)
		case "broken":
			return errors.New("permission denied")
		}
		return nil
	}
	var warnings []string

	remaining, err := runGcpCleanups(pending, cleanup, func(m string) { warnings = append(warnings, m) })

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
	assert.Equal(t, []PendingGcpCleanup{{Name: "broken", ProjectID: "p"}}, remaining,
		"only a failed cleanup is kept to retry; a done one and a foreign one are not")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "foreign")
}

func TestRunGcpCleanups_AllDone(t *testing.T) {
	remaining, err := runGcpCleanups(
		[]PendingGcpCleanup{{Name: "a", ProjectID: "p"}},
		func(string, string) error { return nil },
		func(string) { t.Fatal("no warning expected") },
	)

	require.NoError(t, err)
	assert.Empty(t, remaining)
}

func TestMergePendingGcpCleanup(t *testing.T) {
	existing := []PendingGcpCleanup{{Name: "a", ProjectID: "p"}}

	merged := mergePendingGcpCleanup(existing, []PendingGcpCleanup{
		{Name: "a", ProjectID: "p"},
		{Name: "b", ProjectID: "p"},
	})

	assert.Equal(t, []PendingGcpCleanup{{Name: "a", ProjectID: "p"}, {Name: "b", ProjectID: "p"}}, merged)
	assert.Len(t, existing, 1, "the input is not modified")
}

// a failure that is not the cleanup one leaves the cluster in place, so nothing
// is recorded as destroyed - and nothing touches the config
func TestMarkGkeCleanupPending_OtherErrorsRecordNothing(t *testing.T) {
	controlPlane := &ControlPlane{Name: "cp"}

	require.NoError(t, markGkeCleanupPending(controlPlane, errors.New("failed to destroy Pulumi stack")))
	assert.False(t, controlPlane.InfraDestroyed)
	assert.False(t, controlPlane.GKEProviderConfig.ServiceAccountCleanupPending)
}

// the bucket a first teardown recorded, and whether this run deletes what is in
// it, reach the retry: without them it skips the backup cleanup the first run
// did not finish
func TestResumeGkeInfra_CarriesBackupCleanup(t *testing.T) {
	controlPlane := &ControlPlane{
		Name: "cp",
		GKEProviderConfig: GKEProviderConfig{
			GcpProjectId: "proj",
			GcpRegion:    "us-east1",
			BackupBucket: "cp-backups",
		},
	}

	kept := resumeGkeInfra(controlPlane, false)
	assert.Equal(t, "cp-backups", kept.BackupBucket)
	assert.False(t, kept.DeleteBackups)
	assert.Equal(t, "proj", kept.ProjectID)

	deleted := resumeGkeInfra(controlPlane, true)
	assert.Equal(t, "cp-backups", deleted.BackupBucket)
	assert.True(t, deleted.DeleteBackups)
}

func TestResumeGkeCleanup_RequiresProject(t *testing.T) {
	err := resumeGkeCleanup(&ControlPlane{Name: "cp"}, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GcpProjectId")
}

// provider cleanup that failed after the infrastructure was destroyed is picked up
// by a repeat, with no GKE step for a control plane that never had one pending
func TestResumeTeardownCleanup_NothingPendingIsDone(t *testing.T) {
	require.NoError(t, resumeTeardownCleanup(&ControlPlane{
		Name:           "cp",
		Provider:       v0.KubernetesRuntimeInfraProviderGKE,
		InfraDestroyed: true,
	}, false))
	require.NoError(t, resumeTeardownCleanup(&ControlPlane{
		Name:           "cp",
		Provider:       v0.KubernetesRuntimeInfraProviderEKS,
		InfraDestroyed: true,
	}, false))
}

func TestResumeTeardownCleanup_PendingGkeAccountNeedsProject(t *testing.T) {
	err := resumeTeardownCleanup(&ControlPlane{
		Name:           "cp",
		Provider:       v0.KubernetesRuntimeInfraProviderGKE,
		InfraDestroyed: true,
		GKEProviderConfig: GKEProviderConfig{
			ServiceAccountCleanupPending: true,
		},
	}, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GcpProjectId")
}
