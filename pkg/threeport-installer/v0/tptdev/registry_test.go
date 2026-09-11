package tptdev

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

// TestResolveKubeconfigPath covers the defect where the local registry step
// ignored --kind-kubeconfig. The path the rest of the command resolved has to
// win; re-resolving here sent the configmap to whatever cluster happened to be
// active in the default kubeconfig, which with --control-plane-only against a
// pre-existing kind cluster is a different cluster or none at all.
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
