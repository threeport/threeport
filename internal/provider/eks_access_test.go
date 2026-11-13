package provider

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eksRegistrationMocks records each resource the EKS program registers: its
// inputs, what it depends on and the changes it ignores.
type eksRegistrationMocks struct {
	eksRecordingMocks
	mu            sync.Mutex
	dependencies  map[string][]string
	ignoreChanges map[string][]string
}

func (m *eksRegistrationMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	if m.dependencies == nil {
		m.dependencies, m.ignoreChanges = map[string][]string{}, map[string][]string{}
	}
	m.dependencies[args.TypeToken] = append(m.dependencies[args.TypeToken], args.RegisterRPC.GetDependencies()...)
	m.ignoreChanges[args.TypeToken] = append(m.ignoreChanges[args.TypeToken], args.RegisterRPC.GetIgnoreChanges()...)
	m.mu.Unlock()

	return m.eksRecordingMocks.NewResource(args)
}

// runEksRegistrations runs the EKS program for infra and returns what it
// registered.
func runEksRegistrations(t *testing.T, infra *KubernetesRuntimeInfraEKS) *eksRegistrationMocks {
	t.Helper()
	mocks := &eksRegistrationMocks{}
	require.NoError(t, pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", mocks)))

	return mocks
}

// TestAnEksClustersAuditLogGoesToALogGroupThatExpires covers AuditLog on
// EKS: the cluster sends its API server's audit, API and authenticator logs
// to CloudWatch Logs, into a log group the stack creates before the cluster,
// under the name EKS writes to, with a retention period.  Without it there is
// neither.
func TestAnEksClustersAuditLogGoesToALogGroupThatExpires(t *testing.T) {
	infra := testEksInfra(2)
	infra.AuditLog = true
	mocks := runEksRegistrations(t, infra)

	logGroups := mocks.byType("aws:cloudwatch/logGroup:LogGroup")
	require.Len(t, logGroups, 1)
	assert.Equal(t, "/aws/eks/eks-test/cluster", logGroups[0].inputs["name"])
	assert.Equal(t, float64(90), logGroups[0].inputs["retentionInDays"])
	cluster := mocks.byType(eksClusterTypeToken)[0]
	assert.Equal(t, []any{"api", "audit", "authenticator"}, cluster.inputs["enabledClusterLogTypes"])
	assert.Contains(t, mocks.dependencies[eksClusterTypeToken], "urn:pulumi:test-stack::eks::aws:cloudwatch/logGroup:LogGroup::eks-test-audit-log",
		"the log group is there before EKS would create its own")

	mocks = runEksRegistrations(t, testEksInfra(2))
	assert.Empty(t, mocks.byType("aws:cloudwatch/logGroup:LogGroup"))
	assert.Nil(t, mocks.byType(eksClusterTypeToken)[0].inputs["enabledClusterLogTypes"])
}

// TestANewEksClusterIsReachedThroughAccessEntries covers how an EKS cluster
// authorizes IAM principals.  A new cluster takes access entries alone, with
// no aws-auth ConfigMap, and its creator, the role Threeport reaches it as,
// gets its administrator access as an access entry EKS shows.  A cluster the
// stack holds from before access entries keeps its configuration, and the
// creator's access, which only a cluster's creation sets, is never changed.
func TestANewEksClusterIsReachedThroughAccessEntries(t *testing.T) {
	mocks := runEksRegistrations(t, testEksInfra(2))
	cluster := mocks.byType(eksClusterTypeToken)[0]
	assert.Equal(t, map[string]any{"authenticationMode": "API", "bootstrapClusterCreatorAdminPermissions": true}, cluster.inputs["accessConfig"])
	assert.Equal(t, []string{"accessConfig.bootstrapClusterCreatorAdminPermissions"}, mocks.ignoreChanges[eksClusterTypeToken])

	before := testEksInfra(2)
	before.keepStackAccessEntries(json.RawMessage(`{"resources": [
		{"type": "aws:eks/cluster:Cluster", "inputs": {"name": "eks-test", "version": "1.32"}}
	]}`))
	require.NotNil(t, before.stackAccessEntries)
	assert.False(t, *before.stackAccessEntries)
	mocks = runEksRegistrations(t, before)
	assert.Nil(t, mocks.byType(eksClusterTypeToken)[0].inputs["accessConfig"])

	since := testEksInfra(2)
	since.keepStackAccessEntries(json.RawMessage(`{"resources": [
		{"type": "aws:eks/cluster:Cluster", "inputs": {"name": "eks-test", "accessConfig": {"authenticationMode": "API"}}}
	]}`))
	assert.True(t, *since.stackAccessEntries)
	mocks = runEksRegistrations(t, since)
	assert.NotNil(t, mocks.byType(eksClusterTypeToken)[0].inputs["accessConfig"])

	none := testEksInfra(2)
	none.keepStackAccessEntries(json.RawMessage(`{"resources": []}`))
	assert.Nil(t, none.stackAccessEntries, "an empty stack is a new cluster")
}
