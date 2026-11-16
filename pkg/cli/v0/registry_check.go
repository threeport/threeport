package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// registryCheckTimeout bounds one image lookup, so an unreachable registry
// cannot hold a preflight for long.
const registryCheckTimeout = 20 * time.Second

// registryImageExists reports whether an image reference resolves in its
// registry. A missing image returns false with no error. A registry that cannot
// be reached or authenticated to returns an error, which the caller reports as a
// warning because the image may still exist.
func registryImageExists(ctx context.Context, image string) (bool, error) {
	// split the reference into registry, repository, and tag
	reference, err := registry.ParseReference(image)
	if err != nil {
		return false, fmt.Errorf("failed to parse image %s: %w", image, err)
	}
	repository, err := remote.NewRepository(reference.Registry + "/" + reference.Repository)
	if err != nil {
		return false, fmt.Errorf("failed to open repository for %s: %w", image, err)
	}
	repository.PlainHTTP = isLocalRegistry(reference.Registry)
	repository.Client = &auth.Client{
		Client:     &http.Client{Timeout: registryCheckTimeout},
		Cache:      auth.NewCache(),
		Credential: registryCredential,
	}

	// resolve the tag without pulling the image
	lookup, cancel := context.WithTimeout(ctx, registryCheckTimeout)
	defer cancel()
	if _, err := repository.Resolve(lookup, reference.Reference); err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to look up image %s: %w", image, err)
	}

	return true, nil
}

// isLocalRegistry reports a registry on the local machine, which serves plain HTTP.
func isLocalRegistry(host string) bool {
	return strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")
}

// registryCredential returns the credential for a registry host. Google Artifact
// Registry takes the caller's Google access token. Any other registry takes none,
// which lets a public registry answer anonymously.
func registryCredential(ctx context.Context, host string) (auth.Credential, error) {
	// only Artifact Registry needs a Google token
	if !strings.HasSuffix(host, "pkg.dev") {
		return auth.EmptyCredential, nil
	}

	// a caller with no Google credentials looks up anonymously
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return auth.EmptyCredential, nil
	}
	token, err := tokenSource.Token()
	if err != nil {
		return auth.EmptyCredential, nil
	}

	return auth.Credential{Username: "oauth2accesstoken", Password: token.AccessToken}, nil
}
