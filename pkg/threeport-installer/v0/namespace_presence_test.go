package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
)

// TestOnlyANamespaceTheAPISaysIsNotFoundIsAbsent covers what decides whether
// a failed install may delete the control plane namespace.  A namespace the
// API returns is present, and one it says is not found is absent.  Any other
// error, such as a timeout or throttling, says nothing about the namespace,
// so it is returned and the presence stays unknown, which a teardown treats
// as present.  The first answer is kept, since it is the one from before the
// install created anything.
func TestOnlyANamespaceTheAPISaysIsNotFoundIsAbsent(t *testing.T) {
	namespaceMapper := func() *meta.RESTMapper {
		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
		var restMapper meta.RESTMapper = mapper
		return &restMapper
	}
	installer := func() *ControlPlaneInstaller {
		cpi := &ControlPlaneInstaller{}
		cpi.Opts.Namespace = "threeport-control-plane"
		return cpi
	}
	namespace := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]interface{}{"name": "threeport-control-plane"},
	}}

	t.Run("present", func(t *testing.T) {
		cpi := installer()
		require.NoError(t, cpi.CheckNamespacePresence(testKubeClient(namespace), namespaceMapper()))
		assert.Equal(t, NamespacePresent, cpi.Opts.NamespacePresence)
	})
	t.Run("not found", func(t *testing.T) {
		cpi := installer()
		require.NoError(t, cpi.CheckNamespacePresence(testKubeClient(), namespaceMapper()))
		assert.Equal(t, NamespaceAbsent, cpi.Opts.NamespacePresence)
	})
	for name, apiErr := range map[string]error{
		"a timeout":  k8serrors.NewServerTimeout(schema.GroupResource{Resource: "namespaces"}, "get", 1),
		"throttling": k8serrors.NewTooManyRequests("slow down", 1),
	} {
		t.Run(name, func(t *testing.T) {
			kubeClient := testKubeClient(namespace)
			kubeClient.PrependReactor("get", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apiErr
			})
			cpi := installer()

			err := cpi.CheckNamespacePresence(kubeClient, namespaceMapper())
			assert.ErrorContains(t, err, "failed to check whether the threeport-control-plane namespace exists")
			assert.ErrorIs(t, err, apiErr)
			assert.Equal(t, NamespacePresenceUnknown, cpi.Opts.NamespacePresence, "an error is not an answer")
		})
	}
	t.Run("the first answer is kept", func(t *testing.T) {
		cpi := installer()
		require.NoError(t, cpi.CheckNamespacePresence(testKubeClient(), namespaceMapper()))
		require.NoError(t, cpi.CheckNamespacePresence(testKubeClient(namespace), namespaceMapper()),
			"the install created the namespace in between")
		assert.Equal(t, NamespaceAbsent, cpi.Opts.NamespacePresence)
	})
}
