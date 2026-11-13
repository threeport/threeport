package provider

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnEksClustersVersionIsItsOldestNodeGroups covers the version an EKS
// upgrade starts from: the lowest of the control plane's and every node
// group's, read by the names the program gives them.  A cluster with a node
// group per zone has one in each zone, and the one left furthest behind is
// the one an upgrade has to bring up.
func TestAnEksClustersVersionIsItsOldestNodeGroups(t *testing.T) {
	readControlPlane, readNodeGroup := eksControlPlaneVersion, eksNodeGroupVersion
	t.Cleanup(func() { eksControlPlaneVersion, eksNodeGroupVersion = readControlPlane, readNodeGroup })
	versions := map[string]string{
		"eks-test-private-node-group":   "1.36",
		"eks-test-private-node-group-0": "1.36",
		"eks-test-private-node-group-1": "1.35",
		"eks-test-private-node-group-2": "1.36",
	}
	var asked []string
	eksControlPlaneVersion = func(*aws.Config, string) (string, error) { return "1.37", nil }
	eksNodeGroupVersion = func(_ *aws.Config, _ string, nodeGroupName string) (string, error) {
		asked = append(asked, nodeGroupName)
		return versions[nodeGroupName], nil
	}

	perZone := testEksInfra(3)
	perZone.NodeGroupPerZone = true
	version, err := perZone.ClusterKubernetesVersion()
	require.NoError(t, err)
	assert.Equal(t, "1.35", version)
	assert.Equal(t, []string{"eks-test-private-node-group-0", "eks-test-private-node-group-1", "eks-test-private-node-group-2"}, asked)

	// the node groups the program creates are the ones read
	mocks := runEksPulumiProgram(t, perZone)
	var created []string
	for _, nodeGroup := range mocks.byType(eksNodeGroupTypeToken) {
		created = append(created, nodeGroup.inputs["nodeGroupName"].(string))
	}
	assert.ElementsMatch(t, asked, created)

	asked = nil
	version, err = testEksInfra(3).ClusterKubernetesVersion()
	require.NoError(t, err)
	assert.Equal(t, "1.36", version)
	assert.Equal(t, []string{"eks-test-private-node-group"}, asked)
}
