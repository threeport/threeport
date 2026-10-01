package provider

import (
	"fmt"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	eksProviderTypeToken              = "pulumi:providers:aws"
	eksVpcTypeToken                   = "aws:ec2/vpc:Vpc"
	eksInternetGatewayTypeToken       = "aws:ec2/internetGateway:InternetGateway"
	eksSubnetTypeToken                = "aws:ec2/subnet:Subnet"
	eksEipTypeToken                   = "aws:ec2/eip:Eip"
	eksNatGatewayTypeToken            = "aws:ec2/natGateway:NatGateway"
	eksRouteTableTypeToken            = "aws:ec2/routeTable:RouteTable"
	eksRouteTableAssociationTypeToken = "aws:ec2/routeTableAssociation:RouteTableAssociation"
	eksClusterTypeToken               = "aws:eks/cluster:Cluster"
	eksNodeGroupTypeToken             = "aws:eks/nodeGroup:NodeGroup"
)

// testAvailabilityZones are the zones the mocked getAvailabilityZones call
// returns.  There are more than the program can use so that the capping
// behavior is exercised.
var testAvailabilityZones = []string{"us-east-1a", "us-east-1b", "us-east-1c", "us-east-1d"}

type eksRecordedResource struct {
	typeToken string
	name      string
	inputs    map[string]any
}

type eksRecordingMocks struct {
	mu        sync.Mutex
	resources []eksRecordedResource
}

func (m *eksRecordingMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, eksRecordedResource{
		typeToken: args.TypeToken,
		name:      args.Name,
		inputs:    args.Inputs.Mappable(),
	})
	m.mu.Unlock()

	return args.Name + "-id", args.Inputs, nil
}

func (m *eksRecordingMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if args.Token == "aws:index/getAvailabilityZones:getAvailabilityZones" {
		zones := make([]resource.PropertyValue, 0, len(testAvailabilityZones))
		for _, zone := range testAvailabilityZones {
			zones = append(zones, resource.NewStringProperty(zone))
		}
		return resource.PropertyMap{
			"names": resource.NewArrayProperty(zones),
		}, nil
	}

	return resource.PropertyMap{}, nil
}

// byType returns the recorded resources of a given type.
func (m *eksRecordingMocks) byType(typeToken string) []eksRecordedResource {
	var matching []eksRecordedResource
	for _, recorded := range m.resources {
		if recorded.typeToken == typeToken {
			matching = append(matching, recorded)
		}
	}

	return matching
}

// testEksInfra returns an EKS infra object configured for the Pulumi program
// tests.
func testEksInfra(zoneCount int32) *KubernetesRuntimeInfraEKS {
	awsConfig := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"session-token",
		),
	}

	return &KubernetesRuntimeInfraEKS{
		PulumiWorkspace: PulumiWorkspace{
			RuntimeInstanceName: "eks-test",
			ProjectName:         "eks",
		},
		AwsAccountID:                 "123456789012",
		AwsConfig:                    &awsConfig,
		Region:                       "us-east-1",
		ClusterRoleArn:               "arn:aws:iam::123456789012:role/eks-test-cluster-role",
		NodeRoleArn:                  "arn:aws:iam::123456789012:role/eks-test-node-role",
		ZoneCount:                    zoneCount,
		DefaultNodeGroupInstanceType: "t3.medium",
		DefaultNodeGroupInitialNodes: 3,
		DefaultNodeGroupMinNodes:     3,
		DefaultNodeGroupMaxNodes:     250,
	}
}

// runEksPulumiProgram runs the program against mocks and returns what it
// registered.
func runEksPulumiProgram(t *testing.T, infra *KubernetesRuntimeInfraEKS) *eksRecordingMocks {
	t.Helper()

	mocks := &eksRecordingMocks{}
	err := pulumi.RunErr(infra.pulumiProgram(), pulumi.WithMocks("eks", "test-stack", mocks))
	require.NoError(t, err)
	require.NotEmpty(t, mocks.resources, "program registered no resources")

	return mocks
}

// tagsOf returns a resource's tags as a plain string map.
func tagsOf(t *testing.T, recorded eksRecordedResource) map[string]string {
	t.Helper()

	rawTags, ok := recorded.inputs["tags"]
	if !ok {
		return nil
	}
	tagMap, ok := rawTags.(map[string]any)
	require.True(t, ok, "tags on %s are not a map", recorded.name)

	tags := make(map[string]string, len(tagMap))
	for key, value := range tagMap {
		stringValue, ok := value.(string)
		require.True(t, ok, "tag %s on %s is not a string", key, recorded.name)
		tags[key] = stringValue
	}

	return tags
}

// secretValue asserts an input was marked secret and returns the value it
// wraps.  The AWS provider marks its credential inputs secret so they are
// encrypted in the Pulumi state file rather than written in the clear.
func secretValue(t *testing.T, input any) string {
	t.Helper()

	secret, ok := input.(*resource.Secret)
	require.True(t, ok, "expected a secret value, got %T", input)
	stringValue, ok := secret.Element.V.(string)
	require.True(t, ok, "expected a secret string, got %T", secret.Element.V)

	return stringValue
}

// TestEksPulumiProgram_ResourceGraph checks the program registers the
// resources aws-builder's EKS resource stack creates, in the quantities its
// per-zone layout calls for.
func TestEksPulumiProgram_ResourceGraph(t *testing.T) {
	infra := testEksInfra(3)
	mocks := runEksPulumiProgram(t, infra)

	assert.Len(t, mocks.byType(eksVpcTypeToken), 1)
	assert.Len(t, mocks.byType(eksInternetGatewayTypeToken), 1)
	assert.Len(t, mocks.byType(eksClusterTypeToken), 1)
	assert.Len(t, mocks.byType(eksNodeGroupTypeToken), 1)

	// one public and one private subnet per zone, each with a route table
	// association
	assert.Len(t, mocks.byType(eksSubnetTypeToken), 6)
	assert.Len(t, mocks.byType(eksRouteTableAssociationTypeToken), 6)

	// a NAT gateway and its elastic IP per zone, so an outage in one zone
	// does not take outbound traffic from the others with it
	assert.Len(t, mocks.byType(eksEipTypeToken), 3)
	assert.Len(t, mocks.byType(eksNatGatewayTypeToken), 3)

	// one shared public route table, plus one private route table per zone
	assert.Len(t, mocks.byType(eksRouteTableTypeToken), 4)
}

// TestEksPulumiProgram_ZoneCount checks the zone count defaults and is capped
// at the number of subnet CIDRs carved out of the VPC.
func TestEksPulumiProgram_ZoneCount(t *testing.T) {
	zoneCountTests := map[string]struct {
		configured int32
		wantZones  int
	}{
		"unset defaults to two": {configured: 0, wantZones: 2},
		"honors the request":    {configured: 3, wantZones: 3},
		"capped at three":       {configured: 6, wantZones: 3},
	}

	for name, zoneCountTest := range zoneCountTests {
		t.Run(name, func(t *testing.T) {
			mocks := runEksPulumiProgram(t, testEksInfra(zoneCountTest.configured))

			// a public and a private subnet in each zone
			assert.Len(t, mocks.byType(eksSubnetTypeToken), zoneCountTest.wantZones*2)
			assert.Len(t, mocks.byType(eksNatGatewayTypeToken), zoneCountTest.wantZones)
		})
	}
}

// TestEksPulumiProgram_SubnetTagsAndCidrs checks the subnet layout matches
// what aws-builder produced: the Kubernetes role tags the AWS load balancer
// controller discovers subnets by, public IPs on the public subnets only, and
// the public/private CIDR pairing per zone.
func TestEksPulumiProgram_SubnetTagsAndCidrs(t *testing.T) {
	infra := testEksInfra(2)
	mocks := runEksPulumiProgram(t, infra)

	subnetCidrs := eksSubnetCidrs()
	publicSubnets := 0
	privateSubnets := 0
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		tags := tagsOf(t, subnet)
		zoneIndex := -1
		for candidate := 0; candidate < 2; candidate++ {
			if subnet.name == fmt.Sprintf("%s-public-subnet-%d", infra.RuntimeInstanceName, candidate) ||
				subnet.name == fmt.Sprintf("%s-private-subnet-%d", infra.RuntimeInstanceName, candidate) {
				zoneIndex = candidate
			}
		}
		require.NotEqual(t, -1, zoneIndex, "unexpected subnet name %s", subnet.name)
		assert.Equal(t, testAvailabilityZones[zoneIndex], subnet.inputs["availabilityZone"])

		if _, public := tags["kubernetes.io/role/elb"]; public {
			publicSubnets++
			assert.Equal(t, "1", tags["kubernetes.io/role/elb"])
			assert.NotContains(t, tags, "kubernetes.io/role/internal-elb")
			assert.Equal(t, true, subnet.inputs["mapPublicIpOnLaunch"])
			assert.Equal(t, subnetCidrs[zoneIndex*2], subnet.inputs["cidrBlock"])
			continue
		}

		privateSubnets++
		assert.Equal(t, "1", tags["kubernetes.io/role/internal-elb"])
		// a node in a private subnet must not get a public IP
		assert.NotEqual(t, true, subnet.inputs["mapPublicIpOnLaunch"])
		assert.Equal(t, subnetCidrs[zoneIndex*2+1], subnet.inputs["cidrBlock"])
	}

	assert.Equal(t, 2, publicSubnets)
	assert.Equal(t, 2, privateSubnets)
}

// TestEksPulumiProgram_TagsOnEveryResource checks every taggable resource
// carries the threeport ownership tag and the cluster tag the in-cluster AWS
// cloud provider uses to recognize the VPC and its subnets as belonging to
// this cluster.
func TestEksPulumiProgram_TagsOnEveryResource(t *testing.T) {
	infra := testEksInfra(2)
	mocks := runEksPulumiProgram(t, infra)

	clusterTag := fmt.Sprintf("kubernetes.io/cluster/%s", infra.RuntimeInstanceName)
	untaggable := map[string]string{
		eksProviderTypeToken:              "the cloud-provider meta-resource carries no user tags",
		eksRouteTableAssociationTypeToken: "RouteTableAssociationArgs has no tags field",
	}

	for _, recorded := range mocks.resources {
		if reason, exempt := untaggable[recorded.typeToken]; exempt {
			assert.Nil(t, tagsOf(t, recorded), "%s: %s", recorded.typeToken, reason)
			continue
		}

		tags := tagsOf(t, recorded)
		require.NotNil(t, tags, "resource %q (%s) carries no tags", recorded.name, recorded.typeToken)
		assert.Equal(t, recorded.name, tags["Name"])
		assert.Equal(t, "shared", tags[clusterTag], "resource %q is missing the cluster tag", recorded.name)
		for key, value := range ThreeportProviderTags() {
			assert.Equal(t, value, tags[key], "resource %q is missing the %s tag", recorded.name, key)
		}
	}
}

// TestEksPulumiProgram_ClusterAndNodeGroup checks the cluster and its node
// group get the settings aws-builder used: private subnets only, both endpoint
// access modes, and the scaling bounds from the runtime definition.
func TestEksPulumiProgram_ClusterAndNodeGroup(t *testing.T) {
	infra := testEksInfra(2)
	mocks := runEksPulumiProgram(t, infra)

	privateSubnetIds := []any{}
	for _, subnet := range mocks.byType(eksSubnetTypeToken) {
		if _, private := tagsOf(t, subnet)["kubernetes.io/role/internal-elb"]; private {
			privateSubnetIds = append(privateSubnetIds, subnet.name+"-id")
		}
	}
	require.Len(t, privateSubnetIds, 2)

	clusters := mocks.byType(eksClusterTypeToken)
	require.Len(t, clusters, 1)
	cluster := clusters[0]
	assert.Equal(t, infra.RuntimeInstanceName, cluster.inputs["name"])
	assert.Equal(t, infra.ClusterRoleArn, cluster.inputs["roleArn"])
	assert.Equal(t, DefaultEksKubernetesVersion, cluster.inputs["version"])

	vpcConfig, ok := cluster.inputs["vpcConfig"].(map[string]any)
	require.True(t, ok, "cluster vpcConfig is not a map")
	// the cluster's network interfaces belong in the private subnets, and the
	// API endpoint has to stay publicly reachable for a control plane hosted
	// outside this VPC
	assert.ElementsMatch(t, privateSubnetIds, vpcConfig["subnetIds"])
	assert.Equal(t, true, vpcConfig["endpointPrivateAccess"])
	assert.Equal(t, true, vpcConfig["endpointPublicAccess"])

	nodeGroups := mocks.byType(eksNodeGroupTypeToken)
	require.Len(t, nodeGroups, 1)
	nodeGroup := nodeGroups[0]
	assert.Equal(
		t,
		fmt.Sprintf("%s-private-node-group", infra.RuntimeInstanceName),
		nodeGroup.inputs["nodeGroupName"],
	)
	assert.Equal(t, infra.NodeRoleArn, nodeGroup.inputs["nodeRoleArn"])
	assert.ElementsMatch(t, privateSubnetIds, nodeGroup.inputs["subnetIds"])
	assert.Equal(t, []any{infra.DefaultNodeGroupInstanceType}, nodeGroup.inputs["instanceTypes"])

	scalingConfig, ok := nodeGroup.inputs["scalingConfig"].(map[string]any)
	require.True(t, ok, "node group scalingConfig is not a map")
	assert.EqualValues(t, infra.DefaultNodeGroupInitialNodes, scalingConfig["desiredSize"])
	assert.EqualValues(t, infra.DefaultNodeGroupMinNodes, scalingConfig["minSize"])
	assert.EqualValues(t, infra.DefaultNodeGroupMaxNodes, scalingConfig["maxSize"])
}

// TestEksPulumiProgram_UsesConfiguredCredentials checks the AWS provider is
// given the credentials from this object's AWS config.  Those commonly belong
// to an assumed resource manager role scoped to the cluster's account, which
// is not the identity Pulumi would pick up from the ambient environment.
func TestEksPulumiProgram_UsesConfiguredCredentials(t *testing.T) {
	infra := testEksInfra(2)
	mocks := runEksPulumiProgram(t, infra)

	providers := mocks.byType(eksProviderTypeToken)
	require.Len(t, providers, 1)
	provider := providers[0]

	configuredCredentials, err := infra.AwsConfig.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, configuredCredentials.AccessKeyID, secretValue(t, provider.inputs["accessKey"]))
	assert.Equal(t, configuredCredentials.SecretAccessKey, secretValue(t, provider.inputs["secretKey"]))
	assert.Equal(t, configuredCredentials.SessionToken, secretValue(t, provider.inputs["token"]))
	assert.Equal(t, infra.Region, provider.inputs["region"])
}
