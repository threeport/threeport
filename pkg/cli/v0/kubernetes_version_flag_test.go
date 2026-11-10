package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestTheKubernetesVersionFlagNamesAVersionTptctlCreatesAClusterOn covers
// --kubernetes-version: a supported version on a provider whose cluster
// tptctl creates.
func TestTheKubernetesVersionFlagNamesAVersionTptctlCreatesAClusterOn(t *testing.T) {
	eks := v0.KubernetesRuntimeInfraProviderEKS
	assert.NoError(t, ValidateKubernetesVersionFlag("", eks, false))
	assert.NoError(t, ValidateKubernetesVersionFlag("", eks, true))
	assert.NoError(t, ValidateKubernetesVersionFlag("1.35", eks, false))

	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.32", eks, false), `--kubernetes-version: KubernetesVersion "1.32" is not supported`)
	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.35", v0.KubernetesRuntimeInfraProviderKind, false), "cannot be set on a kind cluster")
	assert.ErrorContains(t, ValidateKubernetesVersionFlag("1.35", eks, true), "does not apply with --control-plane-only")
}

// TestTheGenesisDefinitionRecordsTheVersionTptctlCreatesItsClusterOn covers
// the version on the definition of the cluster tptctl up creates.
func TestTheGenesisDefinitionRecordsTheVersionTptctlCreatesItsClusterOn(t *testing.T) {
	assert.Equal(t, "1.35", *genesisKubernetesVersion(threeport.Options{
		InfraProvider: v0.KubernetesRuntimeInfraProviderGKE, KubernetesVersion: "1.35",
	}))
	assert.Equal(t, v0.DefaultKubernetesVersion, *genesisKubernetesVersion(threeport.Options{
		InfraProvider: v0.KubernetesRuntimeInfraProviderOKE,
	}))
	assert.Nil(t, genesisKubernetesVersion(threeport.Options{InfraProvider: v0.KubernetesRuntimeInfraProviderKind}))
}
