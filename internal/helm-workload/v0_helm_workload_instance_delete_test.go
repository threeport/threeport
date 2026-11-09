package helmworkload

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// These cover the check at the end of a helm workload instance delete that
// decides whether the release namespace can go. If the query it sends is
// rejected, the delete never completes and is retried forever; if the sharing
// decision is wrong, a namespace in use is removed or one left behind.

func uintPtr(i uint) *uint { return &i }

func strPtr(s string) *string { return &s }

// hwi returns an instance with the given ID and release namespace.
func hwi(id uint, namespace *string) v0.HelmWorkloadInstance {
	inst := v0.HelmWorkloadInstance{ReleaseNamespace: namespace}
	inst.ID = uintPtr(id)
	return inst
}

// bindSharedNamespaceQuery binds the query the delete handler sends for current
// through the API server's real query binder. An unknown key is a 400 there,
// which is what stranded every delete.
func bindSharedNamespaceQuery(t *testing.T, current *v0.HelmWorkloadInstance) v0.HelmWorkloadInstance {
	t.Helper()
	req, err := http.NewRequest(
		http.MethodGet,
		v0.PathHelmWorkloadInstances+"?"+sharedNamespaceQuery(current),
		nil,
	)
	require.NoError(t, err)
	c := echo.New().NewContext(req, nil)

	var filter v0.HelmWorkloadInstance
	require.NoError(t, apiserver.NewQueryBinder().Bind(&filter, c))
	return filter
}

func TestSharedNamespaceQueryIsAcceptedByBinder(t *testing.T) {
	current := hwi(1, strPtr("gateway-system"))
	current.KubernetesRuntimeInstanceID = uintPtr(7)

	filter := bindSharedNamespaceQuery(t, &current)
	require.NotNil(t, filter.KubernetesRuntimeInstanceID,
		"the query must filter on the runtime, not return instances from every runtime")
	assert.Equal(t, uint(7), *filter.KubernetesRuntimeInstanceID)
	// the API returns one page, so instances in other namespaces must not be
	// able to push a sibling in this one off it
	require.NotNil(t, filter.ReleaseNamespace,
		"the query must filter on the namespace so unrelated instances cannot fill the page")
	assert.Equal(t, "gateway-system", *filter.ReleaseNamespace)
}

func TestSharedNamespaceQueryWithoutNamespace(t *testing.T) {
	for name, ns := range map[string]*string{"nil": nil, "empty": strPtr("")} {
		t.Run(name, func(t *testing.T) {
			current := hwi(1, ns)
			current.KubernetesRuntimeInstanceID = uintPtr(7)

			// no namespace to filter on; sending an empty one would be a filter
			// on the empty string rather than none
			assert.Equal(t, "kubernetesruntimeinstanceid=7", sharedNamespaceQuery(&current))
			filter := bindSharedNamespaceQuery(t, &current)
			assert.Nil(t, filter.ReleaseNamespace)
		})
	}
}

func TestReleaseNamespaceShared(t *testing.T) {
	ns := strPtr("gateway-system")
	current := hwi(1, ns)

	tests := []struct {
		name    string
		current v0.HelmWorkloadInstance
		others  []v0.HelmWorkloadInstance
		want    bool
	}{
		{
			name:    "another instance in the same namespace keeps it",
			current: current,
			others:  []v0.HelmWorkloadInstance{current, hwi(2, strPtr("gateway-system"))},
			want:    true,
		},
		{
			name:    "instances in other namespaces do not",
			current: current,
			others:  []v0.HelmWorkloadInstance{current, hwi(2, strPtr("other"))},
			want:    false,
		},
		{
			name:    "the instance alone does not",
			current: current,
			others:  []v0.HelmWorkloadInstance{current},
			want:    false,
		},
		{
			name:    "no instances returned does not",
			current: current,
			want:    false,
		},
		{
			name:    "instances without a namespace or ID are ignored",
			current: current,
			others:  []v0.HelmWorkloadInstance{hwi(2, nil), {ReleaseNamespace: ns}},
			want:    false,
		},
		{
			name:    "an instance with no namespace never shares one",
			current: hwi(1, nil),
			others:  []v0.HelmWorkloadInstance{hwi(2, nil), hwi(3, strPtr(""))},
			want:    false,
		},
		{
			name:    "an empty namespace never shares one",
			current: hwi(1, strPtr("")),
			others:  []v0.HelmWorkloadInstance{hwi(2, strPtr(""))},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, releaseNamespaceShared(&tt.current, tt.others))
		})
	}
}
