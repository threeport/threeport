package kubernetesruntime

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestIsGkeControlPlaneHost covers which runtimes skip the compute space
// component checks. Only the cluster the control plane itself runs on, and only
// on GKE: a managed GKE cluster still gets its components, and a host on any
// other provider keeps the behavior it has always had.
func TestIsGkeControlPlaneHost(t *testing.T) {
	definition := func(provider *string) *v0.KubernetesRuntimeDefinition {
		return &v0.KubernetesRuntimeDefinition{InfraProvider: provider}
	}
	instance := func(host *bool) *v0.KubernetesRuntimeInstance {
		return &v0.KubernetesRuntimeInstance{ThreeportControlPlaneHost: host}
	}

	tests := []struct {
		name       string
		instance   *v0.KubernetesRuntimeInstance
		definition *v0.KubernetesRuntimeDefinition
		want       bool
	}{
		{"GKE host", instance(util.Ptr(true)), definition(util.Ptr(v0.KubernetesRuntimeInfraProviderGKE)), true},
		{"managed GKE cluster", instance(util.Ptr(false)), definition(util.Ptr(v0.KubernetesRuntimeInfraProviderGKE)), false},
		{"host flag unset", instance(nil), definition(util.Ptr(v0.KubernetesRuntimeInfraProviderGKE)), false},
		{"EKS host", instance(util.Ptr(true)), definition(util.Ptr(v0.KubernetesRuntimeInfraProviderEKS)), false},
		{"host without a provider", instance(util.Ptr(true)), definition(nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isGkeControlPlaneHost(tt.instance, tt.definition))
		})
	}
}
