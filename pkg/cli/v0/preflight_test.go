package v0

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestMissingImagesFailsOnlyForAnImageTheRegistryLacks covers the image check:
// every image found passes, an image the registry lacks fails and is named, and
// an image the lookup could not reach is a warning because it may still exist.
func TestMissingImagesFailsOnlyForAnImageTheRegistryLacks(t *testing.T) {
	exists := func(_ context.Context, image string) (bool, error) {
		switch image {
		case "registry.example/gone:v1":
			return false, nil
		case "registry.example/unreachable:v1":
			return false, errors.New("connection refused")
		}
		return true, nil
	}

	// assert images that exist pass with no warning
	warning, err := missingImages(context.Background(), []string{"registry.example/present:v1"}, exists)
	require.NoError(t, err)
	assert.Empty(t, warning)

	// assert a missing image fails and is named
	_, err = missingImages(context.Background(), []string{"registry.example/present:v1", "registry.example/gone:v1"}, exists)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry.example/gone:v1")

	// assert an unreachable image warns and does not fail
	warning, err = missingImages(context.Background(), []string{"registry.example/unreachable:v1"}, exists)
	require.NoError(t, err)
	assert.Contains(t, warning, "registry.example/unreachable:v1")
}

// TestRegistryImageExistsReadsTheRegistryAnswer covers the lookup against a
// registry that holds one tag: that tag resolves and another does not.
func TestRegistryImageExistsReadsTheRegistryAnswer(t *testing.T) {
	// serve a manifest for one tag only
	const digest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/threeport/threeport-rest-api/manifests/abc12345" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")

	// assert the held tag exists
	found, err := registryImageExists(context.Background(), host+"/threeport/threeport-rest-api:abc12345")
	require.NoError(t, err)
	assert.True(t, found)

	// assert a tag the registry lacks does not
	found, err = registryImageExists(context.Background(), host+"/threeport/threeport-rest-api:other")
	require.NoError(t, err)
	assert.False(t, found)
}

// TestOnlyGoogleOwnedHostsGetAGoogleToken covers which registry hosts receive the
// caller's Google token: Artifact Registry hosts under pkg.dev do, and a
// lookalike domain or another registry does not.
func TestOnlyGoogleOwnedHostsGetAGoogleToken(t *testing.T) {
	// assert hosts under pkg.dev qualify, with or without a port
	for _, host := range []string{"us-central1-docker.pkg.dev", "pkg.dev", "pkg.dev:443", "US-CENTRAL1-DOCKER.PKG.DEV"} {
		assert.True(t, isArtifactRegistryHost(host), host)
	}

	// assert lookalikes and other registries do not
	for _, host := range []string{"evilpkg.dev", "pkg.dev.evil.example", "ghcr.io", "evilpkg.dev:443", ""} {
		assert.False(t, isArtifactRegistryHost(host), host)
	}
}

// TestPendingUpgradeRefusesASourceWithARequest covers the check that no upgrade
// is already waiting on the source: a recorded blue-green source namespace or a
// stored module install refuses, and a clean source passes.
func TestPendingUpgradeRefusesASourceWithARequest(t *testing.T) {
	// assert a clean source passes
	require.NoError(t, pendingUpgrade(&api.ControlPlaneInstance{Namespace: util.Ptr("primary-05")}))

	// assert a pending blue-green request refuses
	require.Error(t, pendingUpgrade(&api.ControlPlaneInstance{
		Namespace:     util.Ptr("primary-06"),
		FromNamespace: util.Ptr("primary-05"),
	}))

	// assert a pending module install refuses
	require.Error(t, pendingUpgrade(&api.ControlPlaneInstance{
		Namespace:     util.Ptr("primary-05"),
		ModuleInstall: util.Ptr("example.com/acme-module-api registry.example v1"),
	}))
}

// TestControlPlaneImagesCheckTheControllerOnlyForARoot covers which images a
// preflight looks up: every control plane pulls three, a control plane with no
// parent also pulls the control plane controller, and an empty tag checks none.
func TestControlPlaneImagesCheckTheControllerOnlyForARoot(t *testing.T) {
	root := &api.ControlPlaneInstance{}
	child := &api.ControlPlaneInstance{ParentControlPlaneInstanceID: util.Ptr(uint(1))}

	// assert a root checks four images
	images := controlPlaneImages(root, "registry.example/threeport", "abc12345")
	assert.Len(t, images, 4)
	assert.Contains(t, images, "registry.example/threeport/threeport-control-plane-controller:abc12345")

	// assert a child checks three
	assert.Len(t, controlPlaneImages(child, "registry.example/threeport", "abc12345"), 3)

	// assert an empty tag checks none
	assert.Empty(t, controlPlaneImages(root, "registry.example/threeport", ""))

	// assert the source's recorded namespace fills an empty one
	recorded := &api.ControlPlaneInstance{ImageNamespace: util.Ptr("recorded.example/threeport")}
	assert.Contains(t, controlPlaneImages(recorded, "", "abc12345")[0], "recorded.example/threeport/")
}

// TestPreflightRunCollectsEveryFailure covers how a preflight reports: a failing
// check does not stop the next one, and a warning does not fail.
func TestPreflightRunCollectsEveryFailure(t *testing.T) {
	run := &preflightRun{}

	// run a passing check, a warning, and two failures
	assert.True(t, run.check("first", func() (string, error) { return "", nil }))
	assert.True(t, run.check("second", func() (string, error) { return "could not confirm", nil }))
	assert.False(t, run.check("third", func() (string, error) { return "", errors.New("bad") }))
	assert.False(t, run.check("fourth", func() (string, error) { return "", errors.New("worse") }))

	// assert both failures are kept and the warning is not
	require.Len(t, run.failures, 2)
	assert.Contains(t, errors.Join(run.failures...).Error(), "third: bad")
	assert.Contains(t, errors.Join(run.failures...).Error(), "fourth: worse")
}
