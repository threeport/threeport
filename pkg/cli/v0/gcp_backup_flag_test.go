package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestBackupRegionOnlyForProductionGke covers where --gcp-backup-region
// means something: a production control plane whose GKE cluster tptctl
// creates.  Anywhere else nothing would be backed up, so the flag is refused
// rather than silently ignored.
func TestBackupRegionOnlyForProductionGke(t *testing.T) {
	gke := v0.KubernetesRuntimeInfraProviderGKE
	prod := threeport.ControlPlaneTierProd

	assert.NoError(t, ValidateGcpBackupRegionFlag("", v0.KubernetesRuntimeInfraProviderKind, threeport.ControlPlaneTierDev, false))
	assert.NoError(t, ValidateGcpBackupRegionFlag("us-east1", gke, prod, false))

	assert.Error(t, ValidateGcpBackupRegionFlag("us-east1", gke, threeport.ControlPlaneTierDev, false), "development tier")
	assert.Error(t, ValidateGcpBackupRegionFlag("us-east1", v0.KubernetesRuntimeInfraProviderEKS, prod, false), "another provider")
	assert.Error(t, ValidateGcpBackupRegionFlag("us-east1", gke, prod, true), "an existing cluster")
}
