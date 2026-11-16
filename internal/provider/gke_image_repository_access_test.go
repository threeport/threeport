package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	registryapi "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/option"
)

// fakeRegistry is an Artifact Registry API server for tests. It serves one
// repository's policy and records the policy a caller writes back.
type fakeRegistry struct {
	policy      registryapi.Policy
	permissions []string
	written     *registryapi.Policy
	server      *httptest.Server
}

// newFakeRegistry starts a fake registry that serves the policy and reports
// the permissions as the ones the caller holds.
func newFakeRegistry(t *testing.T, policy registryapi.Policy, permissions []string) *fakeRegistry {
	registry := &fakeRegistry{policy: policy, permissions: permissions}
	registry.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":getIamPolicy"):
			_ = json.NewEncoder(w).Encode(registry.policy)
		case strings.HasSuffix(r.URL.Path, ":setIamPolicy"):
			var request registryapi.SetIamPolicyRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			registry.written = request.Policy
			_ = json.NewEncoder(w).Encode(request.Policy)
		case strings.HasSuffix(r.URL.Path, ":testIamPermissions"):
			_ = json.NewEncoder(w).Encode(registryapi.TestIamPermissionsResponse{Permissions: registry.permissions})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(registry.server.Close)

	return registry
}

// options returns client options that reach the fake registry.
func (f *fakeRegistry) options() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(f.server.URL),
		option.WithoutAuthentication(),
	}
}

var testRepository = artifactRegistryRepository{location: "us-central1", project: "image-project", name: "threeport"}

// TestGrantRepositoryRoleAddsTheMember covers the policy written back: a new
// binding when the role has none, a new member on the role's binding when it
// has one, and the other bindings left as they were.
func TestGrantRepositoryRoleAddsTheMember(t *testing.T) {
	for name, tc := range map[string]struct {
		policy registryapi.Policy
		want   []*registryapi.Binding
	}{
		"role has no binding": {
			policy: registryapi.Policy{Bindings: []*registryapi.Binding{
				{Role: "roles/artifactregistry.reader", Members: []string{"serviceAccount:node@x"}},
			}},
			want: []*registryapi.Binding{
				{Role: "roles/artifactregistry.reader", Members: []string{"serviceAccount:node@x"}},
				{Role: imageRepositoryManagerRole, Members: []string{"serviceAccount:provider@x"}},
			},
		},
		"role has a binding": {
			policy: registryapi.Policy{Bindings: []*registryapi.Binding{
				{Role: imageRepositoryManagerRole, Members: []string{"user:someone@x"}},
			}},
			want: []*registryapi.Binding{
				{Role: imageRepositoryManagerRole, Members: []string{"user:someone@x", "serviceAccount:provider@x"}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// serve a repository holding the policy
			registry := newFakeRegistry(t, tc.policy, nil)

			// grant the role to the provider account
			err := grantRepositoryRole(
				context.Background(),
				registry.options(),
				testRepository,
				imageRepositoryManagerRole,
				"serviceAccount:provider@x",
			)

			// assert the policy written back holds the member and the rest
			require.NoError(t, err)
			require.NotNil(t, registry.written)
			assert.Equal(t, tc.want, registry.written.Bindings)
		})
	}
}

// TestGrantRepositoryRoleLeavesAnExistingGrant covers a member that already
// holds the role: no policy is written.
func TestGrantRepositoryRoleLeavesAnExistingGrant(t *testing.T) {
	// serve a repository where the member already holds the role
	registry := newFakeRegistry(t, registryapi.Policy{Bindings: []*registryapi.Binding{
		{Role: imageRepositoryManagerRole, Members: []string{"serviceAccount:provider@x"}},
	}}, nil)

	// grant the same role to the same member
	err := grantRepositoryRole(
		context.Background(),
		registry.options(),
		testRepository,
		imageRepositoryManagerRole,
		"serviceAccount:provider@x",
	)

	// assert nothing was written
	require.NoError(t, err)
	assert.Nil(t, registry.written)
}

// TestHoldsRepositoryPermission covers the permission check: true when the
// registry reports the permission, false when it does not.
func TestHoldsRepositoryPermission(t *testing.T) {
	for name, tc := range map[string]struct {
		held []string
		want bool
	}{
		"held":     {held: []string{repositorySetPolicyPermission}, want: true},
		"not held": {held: []string{}, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			// serve a repository that reports the permissions
			registry := newFakeRegistry(t, registryapi.Policy{}, tc.held)

			// ask whether the caller can set the repository's policy
			got, err := holdsRepositoryPermission(
				context.Background(),
				registry.options(),
				testRepository,
				repositorySetPolicyPermission,
			)

			// assert the answer follows the registry's report
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestImageRepositoryAccessSkipsOtherRegistries covers a namespace that names
// no Artifact Registry repository: nothing is granted and nothing is asked.
func TestImageRepositoryAccessSkipsOtherRegistries(t *testing.T) {
	// grant on a namespace of another registry
	assert.NoError(t, GrantImageRepositoryManager("ghcr.io/threeport", "provider@x"))
	assert.NoError(t, GrantImageRepositoryManager("", "provider@x"))

	// ask about a namespace of another registry
	can, err := CanManageImageRepositoryAccess("ghcr.io/threeport", "")
	require.NoError(t, err)
	assert.False(t, can)
}
