package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsPulumiCheckpointRejectsForeignState checks that state left by the
// previous provisioning engine is not mistaken for Pulumi state.
//
// Both live in the same ResourceInventory column, and the restore path writes
// whatever it is given straight to the stack's state file, so bytes that are
// not a checkpoint corrupt the stack instead of reporting the mismatch.
func TestIsPulumiCheckpointRejectsForeignState(t *testing.T) {
	// what aws-builder stored for an EKS cluster
	awsBuilderInventory := []byte(`{
		"vpcId": "vpc-0123456789abcdef0",
		"cluster": {"clusterName": "threeport-test", "oidcProviderUrl": "https://oidc.eks..."},
		"availabilityZones": [{"zone": "us-east-1a"}]
	}`)
	assert.False(t, isPulumiCheckpoint(awsBuilderInventory))

	assert.False(t, isPulumiCheckpoint([]byte(`{}`)))
	assert.False(t, isPulumiCheckpoint([]byte(`null`)))
	assert.False(t, isPulumiCheckpoint([]byte(`not json at all`)))
	// a version alone is not enough - the resources have to be in there
	assert.False(t, isPulumiCheckpoint([]byte(`{"version": 3}`)))

	assert.True(t, isPulumiCheckpoint([]byte(`{"version":3,"checkpoint":{"stack":"eks","latest":{}}}`)))
	assert.True(t, isPulumiCheckpoint([]byte(`{"version":3,"latest":{"resources":[]}}`)))
}
