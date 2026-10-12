package v0

import (
	"strings"
	"testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestRestoredRuntimeLocation covers the location recorded after a database drop.
func TestRestoredRuntimeLocation(t *testing.T) {
	// kind records Local and does not need a region
	location, err := restoredRuntimeLocation(v0.KubernetesRuntimeInfraProviderKind, "")
	if err != nil || location != localRuntimeLocation {
		t.Fatalf("kind location = %q, %v", location, err)
	}

	// gke without a region fails before anything is written
	if _, err := restoredRuntimeLocation(v0.KubernetesRuntimeInfraProviderGKE, ""); err == nil {
		t.Fatal("expected an empty gke region to fail")
	}

	// gke maps the region onto a location
	location, err = restoredRuntimeLocation(v0.KubernetesRuntimeInfraProviderGKE, "us-central1")
	if err != nil || location != "NorthAmerica:Denver" {
		t.Fatalf("gke location = %q, %v", location, err)
	}

	// eks and oke stay supported, and a drop cannot rebuild their runtime record
	for _, providerName := range []string{
		v0.KubernetesRuntimeInfraProviderEKS,
		v0.KubernetesRuntimeInfraProviderOKE,
	} {
		_, err := restoredRuntimeLocation(providerName, "")
		if err == nil || strings.Contains(err.Error(), "not supported") {
			t.Fatalf("provider %s error = %v", providerName, err)
		}
	}
}
