package provider

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	awseks "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// DefaultEksKubernetesVersion is the Kubernetes version used for an EKS
	// cluster when the runtime definition does not pin one.
	DefaultEksKubernetesVersion = "1.32"

	// eksVpcCidr is the CIDR block for the VPC the cluster runs in.
	eksVpcCidr = "10.0.0.0/16"

	// eksDefaultZoneCount is the number of availability zones used when the
	// runtime definition does not ask for a specific count.
	eksDefaultZoneCount = 2

	// eksMaxZoneCount caps how many availability zones the cluster spans.  It
	// is bounded by the number of subnet CIDRs carved out of the VPC.
	eksMaxZoneCount = 3

	// eksDefaultRouteCidr is the destination that sends traffic to the
	// internet gateway from public subnets and to a NAT gateway from private
	// ones.
	eksDefaultRouteCidr = "0.0.0.0/0"
)

// eksSubnetCidrs returns the CIDR blocks carved out of the VPC for subnets.
// Each availability zone takes two: the public subnet first, then the private
// one.
func eksSubnetCidrs() []string {
	return []string{
		"10.0.0.0/22",
		"10.0.4.0/22",
		"10.0.8.0/22",
		"10.0.12.0/22",
		"10.0.16.0/22",
		"10.0.20.0/22",
	}
}

// InitPulumiProject fills in the Pulumi project identity and stack config, and
// returns the receiver so a caller can set it at construction.
//
// It has to be called before the infra is handed to the lifecycle, not from
// CreateInfra. executeInfraCreate starts the fsnotify watcher that streams
// partial state to the API before it calls DeployInfra, and the watched path is
// built from ProjectName: setting it later means the watcher reads an empty
// name, watches one directory above where Pulumi writes, and never fires - so a
// controller that crashes mid-provision has persisted nothing and a retry
// starts over, leaving the first attempt's resources behind. Setting it here
// also keeps the two goroutines from racing on the same field.
const (
	eksPulumiProjectName        = "eks"
	eksPulumiProjectDescription = "Elastic Kubernetes Service (EKS) cluster for Threeport"
)

// InitPulumiProject fills in the Pulumi project identity and stack config, and
// returns the receiver so a caller can set it at construction.
//
// It has to be called before the infra is handed to the lifecycle, not from
// CreateInfra. executeInfraCreate starts the fsnotify watcher that streams
// partial state to the API before it calls DeployInfra, and the watched path is
// built from ProjectName: setting it later means the watcher reads an empty
// name, watches one directory above where Pulumi writes, and never fires - so a
// controller that crashes mid-provision has persisted nothing and a retry
// starts over, leaving the first attempt's resources behind. Setting it here
// also keeps the two goroutines from racing on the same field.
func (i *KubernetesRuntimeInfraEKS) InitPulumiProject() *KubernetesRuntimeInfraEKS {
	if i.ProjectName == "" {
		i.ProjectName = eksPulumiProjectName
	}
	if i.ProjectDescription == "" {
		i.ProjectDescription = eksPulumiProjectDescription
	}
	// The AWS provider is configured through stack config rather than through
	// a provider resource declared in the program. A destroy or a refresh does
	// not run the program, so credentials baked into a provider resource are
	// read back from state as they were when the stack was built - and a
	// rotated, revoked key then leaves the cluster impossible to delete. Stack
	// config is rewritten on every operation.
	i.StackConfigs = map[string]string{"aws:region": i.Region}
	i.StackSecretConfigs = map[string]string{}
	i.StackPathConfigs = map[string]string{}

	// every key this function can write, cleared first. Stack config persists
	// between operations, so one left unwritten keeps whatever the last run
	// put there: a control plane moved from a named profile to environment
	// credentials would go on being handed the old profile, and in CI, where
	// that profile does not exist, that is a failure rather than a difference.
	i.StackConfigRemovals = []string{
		"aws:profile", "aws:accessKey", "aws:secretKey", "aws:token", "aws:assumeRole",
	}

	if i.ProviderCredentials.Profile != "" {
		// the provider reads the profile from the shared config itself, so an
		// SSO or credential_process session is renewed rather than captured
		i.StackConfigs["aws:profile"] = i.ProviderCredentials.Profile
	}
	if i.ProviderCredentials.AccessKeyId != "" {
		i.StackConfigs["aws:accessKey"] = i.ProviderCredentials.AccessKeyId
		i.StackSecretConfigs["aws:secretKey"] = i.ProviderCredentials.SecretAccessKey
		if i.ProviderCredentials.SessionToken != "" {
			i.StackSecretConfigs["aws:token"] = i.ProviderCredentials.SessionToken
		}
	}
	// with nothing configured the provider resolves credentials from its own
	// environment, which is what a control plane running in EKS wants: there
	// the pod is authenticated through IRSA and that identity refreshes itself
	if i.ProviderCredentials.AssumeRoleArn != "" {
		// the role rather than credentials already assumed from it: a session
		// lasts an hour and creating a cluster and its node group routinely
		// takes longer, which would strand the stack half built and leave the
		// destroy unable to authenticate either
		i.StackPathConfigs["aws:assumeRole.roleArn"] = i.ProviderCredentials.AssumeRoleArn
		if i.ProviderCredentials.ExternalId != "" {
			i.StackPathConfigs["aws:assumeRole.externalId"] = i.ProviderCredentials.ExternalId
		}
	}

	return i
}

// zoneCount returns how many availability zones the cluster spans, bounded by
// the number of subnet CIDRs available.
func (i *KubernetesRuntimeInfraEKS) zoneCount() int {
	zoneCount := int(i.ZoneCount)
	if zoneCount == 0 {
		zoneCount = eksDefaultZoneCount
	}
	if zoneCount > eksMaxZoneCount {
		zoneCount = eksMaxZoneCount
	}

	return zoneCount
}

// kubernetesVersion returns the Kubernetes version to provision the cluster
// with.
func (i *KubernetesRuntimeInfraEKS) kubernetesVersion() string {
	if i.KubernetesVersion == "" {
		return DefaultEksKubernetesVersion
	}

	return i.KubernetesVersion
}

// resourceTags returns the tags applied to every AWS resource in the stack,
// with a Name identifying the individual resource.
func (i *KubernetesRuntimeInfraEKS) resourceTags(name string) pulumi.StringMap {
	tags := pulumi.StringMap{
		"Name": pulumi.String(name),
		// the AWS cloud provider and load balancer controller running in the
		// cluster discover the VPC and its subnets by this tag
		fmt.Sprintf("kubernetes.io/cluster/%s", i.RuntimeInstanceName): pulumi.String("shared"),
	}
	for key, value := range ThreeportProviderTags() {
		tags[key] = pulumi.String(value)
	}

	return tags
}

// pulumiProgram defines the Pulumi resources for the EKS stack: the VPC and
// its networking, the cluster, and its node group.  IAM, the OIDC provider and
// the IRSA roles are bootstrapped outside this program with the AWS SDK,
// matching how the GCP and OCI providers keep their IAM work out of Pulumi.
func (i *KubernetesRuntimeInfraEKS) pulumiProgram() pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		// Give the Pulumi AWS provider the long-lived credentials and the
		// role to assume rather than credentials already assumed by this
		// process.  The provider runs out of process and can only be handed
		// values, so assumed-role credentials would be frozen at the moment
		// they were read - and an assumed role lasts an hour while creating
		// a cluster and its node group routinely takes longer, which strands
		// the stack half built and leaves the destroy unable to authenticate
		// either.  Given the role, the provider renews as it goes.
		availabilityZones, err := aws.GetAvailabilityZones(
			ctx,
			&aws.GetAvailabilityZonesArgs{State: pulumi.StringRef("available")},
		)
		if err != nil {
			return fmt.Errorf("failed to get availability zones for region %s: %w", i.Region, err)
		}
		zoneCount := i.zoneCount()
		if len(availabilityZones.Names) < zoneCount {
			return fmt.Errorf(
				"region %s has %d availability zones available, %d required",
				i.Region, len(availabilityZones.Names), zoneCount,
			)
		}
		zones := availabilityZones.Names[:zoneCount]

		vpc, err := ec2.NewVpc(ctx, fmt.Sprintf("%s-vpc", i.RuntimeInstanceName), &ec2.VpcArgs{
			CidrBlock: pulumi.String(eksVpcCidr),
			// the cluster's nodes resolve one another, and the AWS services
			// they talk to, by DNS name
			EnableDnsSupport:   pulumi.Bool(true),
			EnableDnsHostnames: pulumi.Bool(true),
			Tags:               i.resourceTags(fmt.Sprintf("%s-vpc", i.RuntimeInstanceName)),
		})
		if err != nil {
			return fmt.Errorf("failed to create VPC: %w", err)
		}

		internetGateway, err := ec2.NewInternetGateway(
			ctx,
			fmt.Sprintf("%s-igw", i.RuntimeInstanceName),
			&ec2.InternetGatewayArgs{
				VpcId: vpc.ID(),
				Tags:  i.resourceTags(fmt.Sprintf("%s-igw", i.RuntimeInstanceName)),
			},
		)
		if err != nil {
			return fmt.Errorf("failed to create internet gateway: %w", err)
		}

		// the public route table is shared by every public subnet - they all
		// reach the internet through the single internet gateway
		publicRouteTable, err := ec2.NewRouteTable(
			ctx,
			fmt.Sprintf("%s-public-route-table", i.RuntimeInstanceName),
			&ec2.RouteTableArgs{
				VpcId: vpc.ID(),
				Routes: ec2.RouteTableRouteArray{
					ec2.RouteTableRouteArgs{
						CidrBlock: pulumi.String(eksDefaultRouteCidr),
						GatewayId: internetGateway.ID(),
					},
				},
				Tags: i.resourceTags(fmt.Sprintf("%s-public-route-table", i.RuntimeInstanceName)),
			},
		)
		if err != nil {
			return fmt.Errorf("failed to create public route table: %w", err)
		}

		subnetCidrs := eksSubnetCidrs()
		privateSubnetIds := pulumi.StringArray{}
		for zoneIndex, zone := range zones {
			publicSubnetName := fmt.Sprintf("%s-public-subnet-%d", i.RuntimeInstanceName, zoneIndex)
			publicSubnetTags := i.resourceTags(publicSubnetName)
			// the load balancer controller places internet-facing load
			// balancers in subnets carrying this tag
			publicSubnetTags["kubernetes.io/role/elb"] = pulumi.String("1")
			publicSubnet, err := ec2.NewSubnet(ctx, publicSubnetName, &ec2.SubnetArgs{
				VpcId:               vpc.ID(),
				AvailabilityZone:    pulumi.String(zone),
				CidrBlock:           pulumi.String(subnetCidrs[zoneIndex*2]),
				MapPublicIpOnLaunch: pulumi.Bool(true),
				Tags:                publicSubnetTags,
			})
			if err != nil {
				return fmt.Errorf("failed to create public subnet in zone %s: %w", zone, err)
			}

			if _, err := ec2.NewRouteTableAssociation(
				ctx,
				fmt.Sprintf("%s-public-route-table-association-%d", i.RuntimeInstanceName, zoneIndex),
				&ec2.RouteTableAssociationArgs{
					SubnetId:     publicSubnet.ID(),
					RouteTableId: publicRouteTable.ID(),
				},
			); err != nil {
				return fmt.Errorf("failed to associate public route table in zone %s: %w", zone, err)
			}

			privateSubnetName := fmt.Sprintf("%s-private-subnet-%d", i.RuntimeInstanceName, zoneIndex)
			privateSubnetTags := i.resourceTags(privateSubnetName)
			// the load balancer controller places internal load balancers in
			// subnets carrying this tag
			privateSubnetTags["kubernetes.io/role/internal-elb"] = pulumi.String("1")
			privateSubnet, err := ec2.NewSubnet(ctx, privateSubnetName, &ec2.SubnetArgs{
				VpcId:            vpc.ID(),
				AvailabilityZone: pulumi.String(zone),
				CidrBlock:        pulumi.String(subnetCidrs[zoneIndex*2+1]),
				Tags:             privateSubnetTags,
			})
			if err != nil {
				return fmt.Errorf("failed to create private subnet in zone %s: %w", zone, err)
			}
			privateSubnetIds = append(privateSubnetIds, privateSubnet.ID())

			// each zone gets its own NAT gateway so that a zone outage does
			// not take outbound traffic from the others with it
			elasticIpName := fmt.Sprintf("%s-nat-eip-%d", i.RuntimeInstanceName, zoneIndex)
			elasticIp, err := ec2.NewEip(ctx, elasticIpName, &ec2.EipArgs{
				Domain: pulumi.String("vpc"),
				Tags:   i.resourceTags(elasticIpName),
			})
			if err != nil {
				return fmt.Errorf("failed to create elastic IP in zone %s: %w", zone, err)
			}

			natGatewayName := fmt.Sprintf("%s-nat-gateway-%d", i.RuntimeInstanceName, zoneIndex)
			natGateway, err := ec2.NewNatGateway(ctx, natGatewayName, &ec2.NatGatewayArgs{
				AllocationId: elasticIp.ID(),
				SubnetId:     publicSubnet.ID(),
				Tags:         i.resourceTags(natGatewayName),
			}, pulumi.DependsOn([]pulumi.Resource{internetGateway}))
			if err != nil {
				return fmt.Errorf("failed to create NAT gateway in zone %s: %w", zone, err)
			}

			privateRouteTableName := fmt.Sprintf("%s-private-route-table-%d", i.RuntimeInstanceName, zoneIndex)
			privateRouteTable, err := ec2.NewRouteTable(ctx, privateRouteTableName, &ec2.RouteTableArgs{
				VpcId: vpc.ID(),
				Routes: ec2.RouteTableRouteArray{
					ec2.RouteTableRouteArgs{
						CidrBlock:    pulumi.String(eksDefaultRouteCidr),
						NatGatewayId: natGateway.ID(),
					},
				},
				Tags: i.resourceTags(privateRouteTableName),
			})
			if err != nil {
				return fmt.Errorf("failed to create private route table in zone %s: %w", zone, err)
			}

			if _, err := ec2.NewRouteTableAssociation(
				ctx,
				fmt.Sprintf("%s-private-route-table-association-%d", i.RuntimeInstanceName, zoneIndex),
				&ec2.RouteTableAssociationArgs{
					SubnetId:     privateSubnet.ID(),
					RouteTableId: privateRouteTable.ID(),
				},
			); err != nil {
				return fmt.Errorf("failed to associate private route table in zone %s: %w", zone, err)
			}
		}

		// the cluster's elastic network interfaces live in the private
		// subnets; the API endpoint is reachable both from inside the VPC and
		// from the internet so that a control plane hosted elsewhere can
		// reach it
		cluster, err := awseks.NewCluster(ctx, i.RuntimeInstanceName, &awseks.ClusterArgs{
			Name:    pulumi.String(i.RuntimeInstanceName),
			RoleArn: pulumi.String(i.ClusterRoleArn),
			Version: pulumi.String(i.kubernetesVersion()),
			VpcConfig: &awseks.ClusterVpcConfigArgs{
				SubnetIds:             privateSubnetIds,
				EndpointPrivateAccess: pulumi.Bool(true),
				EndpointPublicAccess:  pulumi.Bool(true),
			},
			Tags: i.resourceTags(i.RuntimeInstanceName),
		})
		if err != nil {
			return fmt.Errorf("failed to create EKS cluster: %w", err)
		}

		nodeGroupName := fmt.Sprintf("%s-private-node-group", i.RuntimeInstanceName)
		if _, err := awseks.NewNodeGroup(ctx, nodeGroupName, &awseks.NodeGroupArgs{
			ClusterName:   cluster.Name,
			NodeGroupName: pulumi.String(nodeGroupName),
			NodeRoleArn:   pulumi.String(i.NodeRoleArn),
			SubnetIds:     privateSubnetIds,
			InstanceTypes: pulumi.StringArray{pulumi.String(i.DefaultNodeGroupInstanceType)},
			Version:       pulumi.String(i.kubernetesVersion()),
			ScalingConfig: &awseks.NodeGroupScalingConfigArgs{
				DesiredSize: pulumi.Int(i.DefaultNodeGroupInitialNodes),
				MinSize:     pulumi.Int(i.DefaultNodeGroupMinNodes),
				MaxSize:     pulumi.Int(i.DefaultNodeGroupMaxNodes),
			},
			Tags: i.resourceTags(nodeGroupName),
		}); err != nil {
			return fmt.Errorf("failed to create EKS node group: %w", err)
		}

		return nil
	}
}
