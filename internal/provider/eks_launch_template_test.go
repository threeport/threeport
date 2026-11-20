package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const eksLaunchTemplateTypeToken = "aws:ec2/launchTemplate:LaunchTemplate"

// TestNewNodeGroupsKeepPodsFromTheMetadataService covers a new cluster's node
// groups: each is created from a launch template that requires session tokens
// for the instance metadata service and limits its responses to one hop, so a
// pod behind the node can't take the node role's credentials.
func TestNewNodeGroupsKeepPodsFromTheMetadataService(t *testing.T) {
	for _, perZone := range []bool{false, true} {
		infra := testEksInfra(3)
		infra.NodeGroupPerZone = perZone
		infra.keepStackNodeGroups(nil)
		mocks := runEksPulumiProgram(t, infra)

		launchTemplates := mocks.byType(eksLaunchTemplateTypeToken)
		require.Len(t, launchTemplates, 1, "one launch template for every node group")
		assert.Equal(t, "eks-test-node-launch-template", launchTemplates[0].inputs["name"])
		assert.Equal(t, map[string]any{
			"httpEndpoint":            "enabled",
			"httpTokens":              "required",
			"httpPutResponseHopLimit": float64(1),
		}, launchTemplates[0].inputs["metadataOptions"])
		assert.NotContains(t, launchTemplates[0].inputs, "imageId", "EKS picks the node image")
		assert.NotContains(t, launchTemplates[0].inputs, "iamInstanceProfile", "EKS refuses a launch template with one")

		nodeGroups := mocks.byType(eksNodeGroupTypeToken)
		require.NotEmpty(t, nodeGroups)
		for _, nodeGroup := range nodeGroups {
			assert.Equal(t, map[string]any{
				"id":      "eks-test-node-launch-template-id",
				"version": "1",
			}, nodeGroup.inputs["launchTemplate"], nodeGroup.name)
		}
	}
}

// TestNodeGroupsCreatedWithoutALaunchTemplateKeepNone covers a stack that
// holds node groups created before the launch template: EKS replaces a node
// group to give it one, so they are left without, and only a node group the
// stack doesn't hold yet gets it.
func TestNodeGroupsCreatedWithoutALaunchTemplateKeepNone(t *testing.T) {
	infra := testEksInfra(3)
	infra.keepStackNodeGroups([]byte(`{"resources":[
		{"type":"aws:eks/cluster:Cluster","inputs":{"name":"eks-test"}},
		{"type":"aws:eks/nodeGroup:NodeGroup","inputs":{"nodeGroupName":"eks-test-private-node-group"}}
	]}`))
	mocks := runEksPulumiProgram(t, infra)
	assert.Empty(t, mocks.byType(eksLaunchTemplateTypeToken))
	nodeGroups := mocks.byType(eksNodeGroupTypeToken)
	require.Len(t, nodeGroups, 1)
	assert.NotContains(t, nodeGroups[0].inputs, "launchTemplate")

	// a node group per zone, one of which the stack holds without one
	infra = testEksInfra(3)
	infra.NodeGroupPerZone = true
	infra.keepStackNodeGroups([]byte(`{"resources":[
		{"type":"aws:eks/nodeGroup:NodeGroup","inputs":{"nodeGroupName":"eks-test-private-node-group-0"}},
		{"type":"aws:eks/nodeGroup:NodeGroup","inputs":{"nodeGroupName":"eks-test-private-node-group-1","launchTemplate":{"id":"lt-1","version":"1"}}}
	]}`))
	mocks = runEksPulumiProgram(t, infra)
	assert.Len(t, mocks.byType(eksLaunchTemplateTypeToken), 1)
	launchTemplates := map[string]any{}
	for _, nodeGroup := range mocks.byType(eksNodeGroupTypeToken) {
		launchTemplates[nodeGroup.name] = nodeGroup.inputs["launchTemplate"]
	}
	assert.Nil(t, launchTemplates["eks-test-private-node-group-0"])
	assert.NotNil(t, launchTemplates["eks-test-private-node-group-1"])
	assert.NotNil(t, launchTemplates["eks-test-private-node-group-2"])
}

// TestKeepStackNodeGroups covers reading which node groups a deployment holds
// without a launch template.
func TestKeepStackNodeGroups(t *testing.T) {
	for name, tc := range map[string]struct {
		deployment string
		want       map[string]bool
	}{
		"a new stack":   {deployment: ``, want: map[string]bool{}},
		"unreadable":    {deployment: `{"resources":`, want: map[string]bool{}},
		"no node group": {deployment: `{"resources":[{"type":"aws:eks/cluster:Cluster","inputs":{}}]}`, want: map[string]bool{}},
		"one without and one with": {
			deployment: `{"resources":[
				{"type":"aws:eks/nodeGroup:NodeGroup","inputs":{"nodeGroupName":"a"}},
				{"type":"aws:eks/nodeGroup:NodeGroup","inputs":{"nodeGroupName":"b","launchTemplate":{"id":"lt-1"}}}
			]}`,
			want: map[string]bool{"a": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			infra := testEksInfra(2)
			infra.stackNodeGroupsWithoutLaunchTemplate = map[string]bool{"left over": true}
			infra.keepStackNodeGroups([]byte(tc.deployment))
			assert.Equal(t, tc.want, infra.stackNodeGroupsWithoutLaunchTemplate)
		})
	}
}
