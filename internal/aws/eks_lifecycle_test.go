package aws

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestEksInfraFromApiObjects_MapsDefinitionNodeGroupFields checks the runtime
// definition's node group settings reach the infra object the Pulumi program
// provisions from.  A field dropped here provisions a cluster that is sized
// differently from what was asked for, with nothing to indicate it.
func TestEksInfraFromApiObjects_MapsDefinitionNodeGroupFields(t *testing.T) {
	instance := &v0.AwsEksKubernetesRuntimeInstance{
		Instance: v0.Instance{Name: util.Ptr("test-instance")},
		Region:   util.Ptr("us-east-1"),
	}
	definition := &v0.AwsEksKubernetesRuntimeDefinition{
		ZoneCount:                    util.Ptr(3),
		DefaultNodeGroupInstanceType: util.Ptr("t3.large"),
		DefaultNodeGroupInitialSize:  util.Ptr(2),
		DefaultNodeGroupMinimumSize:  util.Ptr(3),
		DefaultNodeGroupMaximumSize:  util.Ptr(7),
	}
	awsConfig := &aws.Config{Region: "us-east-1"}

	infra := eksInfraFromApiObjects(instance, definition, "123456789012", awsConfig, nil)

	assert.Equal(t, "test-instance", infra.RuntimeInstanceName)
	assert.Equal(t, "123456789012", infra.AwsAccountID)
	assert.Equal(t, "us-east-1", infra.Region)
	assert.Same(t, awsConfig, infra.AwsConfig)
	assert.Equal(t, int32(3), infra.ZoneCount)
	assert.Equal(t, "t3.large", infra.DefaultNodeGroupInstanceType)
	assert.Equal(t, int32(2), infra.DefaultNodeGroupInitialNodes)
	assert.Equal(t, int32(3), infra.DefaultNodeGroupMinNodes)
	assert.Equal(t, int32(7), infra.DefaultNodeGroupMaxNodes)
}

// TestEksInfraSatisfiesLifecycleInterfaces checks the infra object plugs into
// the shared lifecycle engine the way GKE and OKE do.  The engine type-asserts
// for the optional interfaces, so a missing one is skipped silently rather
// than reported.
func TestEksInfraSatisfiesLifecycleInterfaces(t *testing.T) {
	var infra any = &provider.KubernetesRuntimeInfraEKS{}

	_, isInfraProvider := infra.(provider.InfraProvider)
	assert.True(t, isInfraProvider, "must implement InfraProvider")

	// without this the engine cannot stream state to the API while the stack
	// is still being provisioned, so an interrupted create cannot resume
	_, isStreamable := infra.(provider.StreamableProvider)
	assert.True(t, isStreamable, "must implement StreamableProvider")

	// without this the stack is not refreshed before a destroy, leaving stale
	// pending operations in the way
	_, isRefreshable := infra.(provider.RefreshableProvider)
	assert.True(t, isRefreshable, "must implement RefreshableProvider")
}

// TestEksLifecycleSatisfiesInfraLifecycleProvider checks the adapter
// implements every method the shared create and delete state machines call.
func TestEksLifecycleSatisfiesInfraLifecycleProvider(t *testing.T) {
	var lifecycle any = &eksLifecycle{}

	_, ok := lifecycle.(provider.InfraLifecycleProvider)
	require.True(t, ok, "must implement InfraLifecycleProvider")
}
