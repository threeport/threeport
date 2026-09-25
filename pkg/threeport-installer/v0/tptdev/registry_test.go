package tptdev

import (
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

// TestRegistryNeedsStart reports which leftover registry states get started.
func TestRegistryNeedsStart(t *testing.T) {
	// compare each docker state to the start decision
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{
			name:   "an exited container is started",
			status: container.StateExited,
			want:   true,
		},
		{
			name:   "a container that never ran is started",
			status: container.StateCreated,
			want:   true,
		},
		{
			name:   "a running container is left alone",
			status: container.StateRunning,
			want:   false,
		},
		{
			name:   "a paused container needs an unpause rather than a start",
			status: container.StatePaused,
			want:   false,
		},
		{
			name:   "a restarting container is already on its way up",
			status: container.StateRestarting,
			want:   false,
		},
		{
			name:   "a container being removed cannot be started",
			status: container.StateRemoving,
			want:   false,
		},
		{
			name:   "a dead container cannot be started",
			status: container.StateDead,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registryNeedsStart(tt.status); got != tt.want {
				t.Errorf(
					"registryNeedsStart(%q) = %v, want %v",
					tt.status,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestResolveKubeconfigPath(t *testing.T) {
	t.Run("a supplied path is used as given", func(t *testing.T) {
		const supplied = "/tmp/kind-cluster.kubeconfig"

		assert.Equal(t, supplied, resolveKubeconfigPath(supplied))
	})

	t.Run("an empty path falls back to client-go precedence", func(t *testing.T) {
		want := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()

		assert.Equal(t, want, resolveKubeconfigPath(""))
	})
}

// TestApplyK8sConfigUsesSuppliedPath covers the seam the resolver test cannot
// reach: that the path is actually what the REST config is built from, rather
// than resolved again inside. Pointing at a file that does not exist is enough
// to tell the two apart, because client-go names the file it failed to stat —
// a version that ignored the argument would name the default kubeconfig
// instead, or succeed against whatever cluster happens to be active.
func TestApplyK8sConfigUsesSuppliedPath(t *testing.T) {
	supplied := filepath.Join(t.TempDir(), "kind-cluster.kubeconfig")

	err := applyK8sConfig(supplied)

	require.Error(t, err, "a kubeconfig that does not exist cannot produce a client")
	assert.Contains(
		t, err.Error(), supplied,
		"the error must name the supplied kubeconfig, or the path was not the one used",
	)
	assert.NotContains(
		t, err.Error(), clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename(),
		"the default kubeconfig must not be consulted when a path was supplied",
	)
}
