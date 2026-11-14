package provider

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGkeProgram runs the GKE program against mocks for a runtime.
func runGkeProgram(t *testing.T, i *KubernetesRuntimeInfraGKE) *gkeRecordingMocks {
	t.Helper()
	i.ProjectName = "gke"
	i.ProjectID = "test-project"
	i.Region = "us-central1"
	i.WorkerNodeInitialCount = 1
	i.MinNodeCount = 1
	i.MaxNodeCount = 3

	mocks := &gkeRecordingMocks{}
	require.NoError(t, pulumi.RunErr(i.pulumiProgram(), pulumi.WithMocks("gke", "test-stack", mocks)))
	return mocks
}

// TestGkeNodesRunAsTheClustersOwnServiceAccount covers the service account a
// new cluster's nodes run as: one per cluster, granted only the role Google
// provides for node service accounts, in place of the project's default
// Compute Engine account.
func TestGkeNodesRunAsTheClustersOwnServiceAccount(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "threeport-test"}}
	mocks := runGkeProgram(t, i)

	accounts := mocks.ofType(gkeAccountTypeToken)
	require.Len(t, accounts, 1)
	assert.Equal(t, i.nodeServiceAccountID(), accounts[0].inputs["accountId"])
	assert.Equal(t, "test-project", accounts[0].inputs["project"])
	email := i.nodeServiceAccountID() + "@test-project.iam.gserviceaccount.com"

	roles := mocks.ofType(gkeIAMMemberToken)
	require.Len(t, roles, 1, "the account is granted one role and nothing else")
	assert.Equal(t, "roles/container.defaultNodeServiceAccount", roles[0].inputs["role"])
	assert.Equal(t, "serviceAccount:"+email, roles[0].inputs["member"])
	assert.Equal(t, "test-project", roles[0].inputs["project"])

	nodePools := mocks.ofType(gkeNodePoolTypeToken)
	require.Len(t, nodePools, 2)
	var nodeConfig map[string]any
	for _, pool := range nodePools {
		if config, ok := pool.inputs["nodeConfig"].(map[string]any); ok && config["serviceAccount"] != nil {
			nodeConfig = config
		}
	}
	require.NotNil(t, nodeConfig)
	assert.Equal(t, email, nodeConfig["serviceAccount"])
	assert.Equal(t, []any{"https://www.googleapis.com/auth/cloud-platform"}, nodeConfig["oauthScopes"],
		"the role, not the scope, limits the nodes")

	// the nodes start with the role, so their first logs and metrics are kept
	var roleURN string
	for _, pool := range nodePools {
		for _, dependency := range pool.dependencies {
			if strings.HasSuffix(dependency, "::"+roles[0].name) {
				roleURN = dependency
			}
		}
	}
	assert.NotEmpty(t, roleURN, "a node pool waits for the role")
}

// TestAGkeNodePoolCreatedOnTheDefaultAccountKeepsIt covers a create resumed on
// a stack whose node pool was created before the nodes had their own service
// account.  Changing a node pool's service account replaces it, so the program
// leaves it on the default account and adds no account of its own.
func TestAGkeNodePoolCreatedOnTheDefaultAccountKeepsIt(t *testing.T) {
	i := &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:                PulumiWorkspace{RuntimeInstanceName: "threeport-test"},
		nodesKeepDefaultServiceAccount: true,
	}
	mocks := runGkeProgram(t, i)

	assert.Empty(t, mocks.ofType(gkeAccountTypeToken))
	assert.Empty(t, mocks.ofType(gkeIAMMemberToken))
	nodePools := mocks.ofType(gkeNodePoolTypeToken)
	require.Len(t, nodePools, 2)
	for _, pool := range nodePools {
		assert.NotContains(t, pool.inputs["nodeConfig"], "serviceAccount")
	}
}

// TestNodePoolRunsAsDefaultServiceAccount covers how CreateInfra tells a node
// pool created on the default account from one created on the cluster's own,
// or from a stack with no node pool yet.
func TestNodePoolRunsAsDefaultServiceAccount(t *testing.T) {
	for name, tc := range map[string]struct {
		deployment string
		want       bool
	}{
		"a new stack":            {deployment: ``, want: false},
		"an empty stack":         {deployment: `{"manifest":{}}`, want: false},
		"no node pool yet":       {deployment: `{"resources":[{"type":"gcp:container/cluster:Cluster","inputs":{"name":"c"}}]}`, want: false},
		"a node pool of its own": {deployment: `{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"serviceAccount":"threeport-svc-test-node-562f28@p.iam.gserviceaccount.com"}}}]}`, want: false},
		"a node pool on the default account": {
			// GKE reports the default account as "default" in the outputs,
			// while the inputs, what the program asked for, have none
			deployment: `{"resources":[{"type":"gcp:container/nodePool:NodePool","inputs":{"nodeConfig":{"machineType":"e2-medium"}},"outputs":{"nodeConfig":{"serviceAccount":"default"}}}]}`,
			want:       true,
		},
		"a deployment that can't be read": {deployment: `{"resources":`, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, nodePoolRunsAsDefaultServiceAccount([]byte(tc.deployment)))
		})
	}
}

// TestGkeNodeServiceAccountID covers the node account's ID for runtime names
// of any length: a valid GCP ID, under a prefix of its own.  The IDs are
// pinned because Pulumi finds the account by the ID it created it with.
func TestGkeNodeServiceAccountID(t *testing.T) {
	for name, want := range map[string]string{
		"test":                               "threeport-node-test-9f86d0",
		"threeport-genesis-01":               "threeport-node-threepor-6c166c",
		"a-very-long-threeport-runtime-name": "threeport-node-a-very-l-e932d8",
	} {
		i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: name}}
		assert.Equal(t, want, i.nodeServiceAccountID(), name)
	}

	for _, name := range []string{"a", "threeport-test", "!!!", strings.Repeat("x", 200)} {
		i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: name}}
		assert.Regexp(t, `^[a-z][a-z0-9-]{4,28}[a-z0-9]$`, i.nodeServiceAccountID(), name)
	}
}

// TestNoNodeAccountTakesAThreeportAccountsID covers runtimes whose names make
// one's node account and another's Threeport account collide under a shared
// scheme: a runtime named threeport-test-node, and a GcpProvider of that name,
// derive the Threeport account formatServiceAccountID("threeport-test-node").
func TestNoNodeAccountTakesAThreeportAccountsID(t *testing.T) {
	names := []string{"threeport-test", "threeport-test-node", "test", "test-node", "a", "x-node-node"}
	threeportAccounts := map[string]string{}
	for _, name := range names {
		i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: name}}
		threeportAccounts[i.getServiceAccountID()] = name
		threeportAccounts[generateServiceAccountID(name)] = name
	}

	nodeAccounts := map[string]string{}
	for _, name := range names {
		i := &KubernetesRuntimeInfraGKE{PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: name}}
		id := i.nodeServiceAccountID()
		other, taken := threeportAccounts[id]
		assert.False(t, taken, "the node account of %q is the Threeport account of %q: %s", name, other, id)
		other, taken = nodeAccounts[id]
		assert.False(t, taken, "%q and %q have the same node account %s", name, other, id)
		nodeAccounts[id] = name
	}
}
