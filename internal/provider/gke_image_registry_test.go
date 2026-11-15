package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gkeRepositoryIAMMemberToken = "gcp:artifactregistry/repositoryIamMember:RepositoryIamMember"

// TestParseArtifactRegistryNamespace covers which image namespaces name an
// Artifact Registry repository, and which parts it reads from them.
func TestParseArtifactRegistryNamespace(t *testing.T) {
	for name, tc := range map[string]struct {
		namespace string
		want      artifactRegistryRepository
		wantOK    bool
	}{
		"repository": {
			namespace: "us-central1-docker.pkg.dev/qleet-shared/threeport",
			want:      artifactRegistryRepository{location: "us-central1", project: "qleet-shared", name: "threeport"},
			wantOK:    true,
		},
		"trailing slash and image path": {
			namespace: "europe-west4-docker.pkg.dev/acme/images/team/",
			want:      artifactRegistryRepository{location: "europe-west4", project: "acme", name: "images"},
			wantOK:    true,
		},
		"no repository":  {namespace: "us-central1-docker.pkg.dev/qleet-shared"},
		"other registry": {namespace: "ghcr.io/threeport/threeport"},
		"docker hub":     {namespace: "docker.io/rj9317"},
		"empty":          {namespace: ""},
	} {
		t.Run(name, func(t *testing.T) {
			// parse the namespace
			got, ok := parseArtifactRegistryNamespace(tc.namespace)

			// assert whether it names a repository, and which one
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestGkeNodesCanPullFromTheImageRepository covers the read grant on the
// Artifact Registry repository for the account the nodes run as: made when the
// image namespace names a repository, and made before the node pool starts.
func TestGkeNodesCanPullFromTheImageRepository(t *testing.T) {
	// run the program with an Artifact Registry image namespace
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		ImageNamespace:  "us-central1-docker.pkg.dev/image-project/threeport",
	}
	mocks := runGkeProgram(t, i)

	// assert the grant names the repository, the role, and the nodes' account
	grants := mocks.ofType(gkeRepositoryIAMMemberToken)
	require.Len(t, grants, 1)
	assert.Equal(t, "image-project", grants[0].inputs["project"])
	assert.Equal(t, "us-central1", grants[0].inputs["location"])
	assert.Equal(t, "threeport", grants[0].inputs["repository"])
	assert.Equal(t, "roles/artifactregistry.reader", grants[0].inputs["role"])
	assert.Equal(
		t,
		"serviceAccount:"+i.nodeServiceAccountID()+"@test-project.iam.gserviceaccount.com",
		grants[0].inputs["member"],
	)

	// assert the node pool waits for the grant
	nodePools := mocks.ofType(gkeNodePoolTypeToken)
	require.Len(t, nodePools, 1)
	waits := false
	for _, dependency := range nodePools[0].dependencies {
		if strings.HasSuffix(dependency, "::"+grants[0].name) {
			waits = true
		}
	}
	assert.True(t, waits, "the node pool waits for the grant")
}

// TestGkeMakesNoImageRepositoryGrantWithoutOne covers the cases with no
// repository grant: another registry, and nodes that keep the default account.
func TestGkeMakesNoImageRepositoryGrantWithoutOne(t *testing.T) {
	// run the program with a registry that is not Artifact Registry
	other := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		ImageNamespace:  "ghcr.io/threeport",
	}
	assert.Empty(t, runGkeProgram(t, other).ofType(gkeRepositoryIAMMemberToken))

	// run the program with nodes that keep the default account
	keep := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:                PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		ImageNamespace:                 "us-central1-docker.pkg.dev/image-project/threeport",
		nodesKeepDefaultServiceAccount: true,
	}
	assert.Empty(t, runGkeProgram(t, keep).ofType(gkeRepositoryIAMMemberToken))
}
