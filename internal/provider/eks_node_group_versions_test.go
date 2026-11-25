package provider

import (
	"encoding/json"
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

// TestAnUpgradeKeepsNodeGroupsThatAreAhead covers an EKS upgrade of a cluster
// whose node groups run different versions: the upgrade's step is one past
// the oldest, and a node group already past it keeps its own version, as the
// control plane does, rather than ask EKS for a downgrade it refuses.
func TestAnUpgradeKeepsNodeGroupsThatAreAhead(t *testing.T) {
	readControlPlane, readNodeGroup := eksControlPlaneVersion, eksNodeGroupVersion
	t.Cleanup(func() { eksControlPlaneVersion, eksNodeGroupVersion = readControlPlane, readNodeGroup })
	eksControlPlaneVersion = func(*aws.Config, string) (string, error) { return "1.37", nil }
	versions := map[string]string{
		"eks-test-private-node-group-0": "1.37",
		"eks-test-private-node-group-1": "1.35",
		"eks-test-private-node-group-2": "1.36",
	}
	eksNodeGroupVersion = func(_ *aws.Config, _ string, nodeGroupName string) (string, error) {
		return versions[nodeGroupName], nil
	}

	infra := testEksInfra(3)
	infra.NodeGroupPerZone = true
	infra.AwsConfig = nil
	// the IAM lookup after the versions fails without an AWS config
	_ = infra.UpgradeInfra("1.36")
	assert.Equal(t, "1.37", infra.controlPlaneVersion)
	assert.Equal(t, map[string]string{"eks-test-private-node-group-0": "1.37"}, infra.nodeGroupVersions)

	mocks := runEksPulumiProgram(t, infra)
	asked := map[string]any{}
	for _, nodeGroup := range mocks.byType(eksNodeGroupTypeToken) {
		asked[nodeGroup.inputs["nodeGroupName"].(string)] = nodeGroup.inputs["version"]
	}
	assert.Equal(t, map[string]any{
		"eks-test-private-node-group-0": "1.37",
		"eks-test-private-node-group-1": "1.36",
		"eks-test-private-node-group-2": "1.36",
	}, asked)
}

// TestAnEksClusterKeepsTheZonesOfItsSubnets covers an up of a stack that
// already holds the cluster's subnets: the cluster stays in their zones,
// rather than the ones the region would offer now, since a subnet can't move
// to another zone without being replaced.  A stack missing a subnet from the
// middle, or holding none, lets the program choose.
func TestAnEksClusterKeepsTheZonesOfItsSubnets(t *testing.T) {
	deployment := json.RawMessage(`{"resources": [
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-private-subnet-1", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1d"}},
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-public-subnet-0", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1z"}},
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-private-subnet-0", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1c"}},
		{"urn": "urn:pulumi:stack::eks::aws:ec2/vpc:Vpc::eks-test-vpc", "type": "aws:ec2/vpc:Vpc", "inputs": {}}
	]}`)
	infra := testEksInfra(2)
	infra.keepStackZones(deployment)
	assert.Equal(t, []string{"us-east-1c", "us-east-1d"}, infra.stackZones)

	mocks := runEksPulumiProgram(t, infra)
	zones := map[string]bool{}
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		zones[subnet.inputs["availabilityZone"].(string)] = true
	}
	assert.Equal(t, map[string]bool{"us-east-1c": true, "us-east-1d": true}, zones, "not the region's first zones, us-east-1a and -b")

	// a create that stopped after the first zone's subnets: its zone is
	// kept, and the rest are chosen as a new cluster's would be, so every
	// node group has a subnet
	partial := testEksInfra(3)
	partial.NodeGroupPerZone = true
	partial.keepStackZones(json.RawMessage(`{"resources": [
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-private-subnet-0", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1d"}}
	]}`))
	assert.Equal(t, []string{"us-east-1d"}, partial.stackZones)
	mocks = runEksPulumiProgram(t, partial)
	assert.Len(t, mocks.byType(eksNodeGroupTypeToken), 3)
	privateZones := map[string]string{}
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		privateZones[subnet.name] = subnet.inputs["availabilityZone"].(string)
	}
	assert.Equal(t, "us-east-1d", privateZones["eks-test-private-subnet-0"])
	assert.NotEqual(t, "us-east-1d", privateZones["eks-test-private-subnet-1"])
	assert.NotEqual(t, "us-east-1d", privateZones["eks-test-private-subnet-2"])
	assert.NotEqual(t, privateZones["eks-test-private-subnet-1"], privateZones["eks-test-private-subnet-2"])

	// a subnet missing from the middle: the others keep their zones, and its
	// zone is chosen from the rest
	gap := testEksInfra(3)
	gap.keepStackZones(json.RawMessage(`{"resources": [
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-private-subnet-0", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1c"}},
		{"urn": "urn:pulumi:stack::eks::aws:ec2/subnet:Subnet::eks-test-private-subnet-2", "type": "aws:ec2/subnet:Subnet", "inputs": {"availabilityZone": "us-east-1a"}}
	]}`))
	assert.Equal(t, []string{"us-east-1c", "", "us-east-1a"}, gap.stackZones)
	mocks = runEksPulumiProgram(t, gap)
	privateZones = map[string]string{}
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		privateZones[subnet.name] = subnet.inputs["availabilityZone"].(string)
	}
	assert.Equal(t, "us-east-1c", privateZones["eks-test-private-subnet-0"])
	assert.Equal(t, "us-east-1b", privateZones["eks-test-private-subnet-1"], "the region's first zone not taken")
	assert.Equal(t, "us-east-1a", privateZones["eks-test-private-subnet-2"])

	none := testEksInfra(3)
	none.keepStackZones(json.RawMessage(`{"resources": []}`))
	assert.Nil(t, none.stackZones)
}
