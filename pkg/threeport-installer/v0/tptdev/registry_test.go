package tptdev

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
