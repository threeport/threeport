package v0

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

func TestGcpProvidersToCleanUp(t *testing.T) {
	providers := []v0.GcpProvider{
		{Name: util.Ptr("one"), ProjectID: util.Ptr("proj-a")},
		{Name: util.Ptr("no-project")},
		{ProjectID: util.Ptr("no-name")},
		{Name: util.Ptr("two"), ProjectID: util.Ptr("proj-b")},
	}

	assert.Equal(t, []PendingGcpCleanup{
		{Name: "one", ProjectID: "proj-a"},
		{Name: "two", ProjectID: "proj-b"},
	}, gcpProvidersToCleanUp(providers))
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

func TestResumeGkeCleanup_RequiresProject(t *testing.T) {
	err := resumeGkeCleanup(&ControlPlane{Name: "cp"})

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
	}))
	require.NoError(t, resumeTeardownCleanup(&ControlPlane{
		Name:           "cp",
		Provider:       v0.KubernetesRuntimeInfraProviderEKS,
		InfraDestroyed: true,
	}))
}

func TestResumeTeardownCleanup_PendingGkeAccountNeedsProject(t *testing.T) {
	err := resumeTeardownCleanup(&ControlPlane{
		Name:           "cp",
		Provider:       v0.KubernetesRuntimeInfraProviderGKE,
		InfraDestroyed: true,
		GKEProviderConfig: GKEProviderConfig{
			ServiceAccountCleanupPending: true,
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GcpProjectId")
}
