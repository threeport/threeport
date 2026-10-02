package v0

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// AwsResourceInventory is the set of AWS resources an install can create,
// recorded so that what is present after a teardown can be compared with what
// was present before it.
//
// It covers the resource types a Threeport install on EKS creates, including
// the load balancer the control plane's API is served through - which is
// easily forgotten, outlives the workloads that created it, and bills by the
// hour.
type AwsResourceInventory map[string][]string

// GetAwsResourceInventory records the AWS resources present in a region.
func GetAwsResourceInventory(
	ctx context.Context,
	awsConfig aws.Config,
) (AwsResourceInventory, error) {
	inventory := AwsResourceInventory{}
	ec2Client := ec2.NewFromConfig(awsConfig)

	vpcs, err := ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPCs: %w", err)
	}
	for _, vpc := range vpcs.Vpcs {
		// the account's default VPC is not ours to account for
		if aws.ToBool(vpc.IsDefault) {
			continue
		}
		inventory["vpc"] = append(inventory["vpc"], aws.ToString(vpc.VpcId))
	}

	natGateways, err := ec2Client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe NAT gateways: %w", err)
	}
	for _, natGateway := range natGateways.NatGateways {
		// a deleted NAT gateway lingers in the API for a while and bills for
		// none of it
		if natGateway.State == ec2types.NatGatewayStateDeleted {
			continue
		}
		inventory["nat-gateway"] = append(inventory["nat-gateway"], aws.ToString(natGateway.NatGatewayId))
	}

	addresses, err := ec2Client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe elastic IPs: %w", err)
	}
	for _, address := range addresses.Addresses {
		inventory["elastic-ip"] = append(inventory["elastic-ip"], aws.ToString(address.AllocationId))
	}

	volumes, err := ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EBS volumes: %w", err)
	}
	for _, volume := range volumes.Volumes {
		inventory["ebs-volume"] = append(inventory["ebs-volume"], aws.ToString(volume.VolumeId))
	}

	instances, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EC2 instances: %w", err)
	}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			if instance.State != nil && instance.State.Name == ec2types.InstanceStateNameTerminated {
				continue
			}
			inventory["ec2-instance"] = append(inventory["ec2-instance"], aws.ToString(instance.InstanceId))
		}
	}

	// the control plane's API is exposed through a load balancer that the
	// cluster's cloud controller creates, not Pulumi, so it is not part of the
	// stack that gets destroyed
	loadBalancers, err := elbv2.NewFromConfig(awsConfig).DescribeLoadBalancers(
		ctx,
		&elbv2.DescribeLoadBalancersInput{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to describe load balancers: %w", err)
	}
	for _, loadBalancer := range loadBalancers.LoadBalancers {
		inventory["load-balancer"] = append(inventory["load-balancer"], aws.ToString(loadBalancer.LoadBalancerArn))
	}

	clusters, err := eks.NewFromConfig(awsConfig).ListClusters(ctx, &eks.ListClustersInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list EKS clusters: %w", err)
	}
	inventory["eks-cluster"] = append(inventory["eks-cluster"], clusters.Clusters...)

	iamClient := iam.NewFromConfig(awsConfig)

	rolePaginator := iam.NewListRolesPaginator(iamClient, &iam.ListRolesInput{})
	for rolePaginator.HasMorePages() {
		page, err := rolePaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list IAM roles: %w", err)
		}
		for _, role := range page.Roles {
			inventory["iam-role"] = append(
				inventory["iam-role"],
				aws.ToString(role.Path)+aws.ToString(role.RoleName),
			)
		}
	}

	policyPaginator := iam.NewListPoliciesPaginator(iamClient, &iam.ListPoliciesInput{Scope: "Local"})
	for policyPaginator.HasMorePages() {
		page, err := policyPaginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list IAM policies: %w", err)
		}
		for _, policy := range page.Policies {
			inventory["iam-policy"] = append(
				inventory["iam-policy"],
				aws.ToString(policy.Path)+aws.ToString(policy.PolicyName),
			)
		}
	}

	providers, err := iamClient.ListOpenIDConnectProviders(ctx, &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to list OIDC identity providers: %w", err)
	}
	for _, provider := range providers.OpenIDConnectProviderList {
		inventory["oidc-provider"] = append(inventory["oidc-provider"], aws.ToString(provider.Arn))
	}

	for resourceType := range inventory {
		sort.Strings(inventory[resourceType])
	}

	return inventory, nil
}

// LeakedResources returns the resources present after a teardown that were
// not present before the install, as "<type> <id>" lines.
//
// It reports additions only.  A resource that was there at the start and is
// gone at the end was not created by the install, so removing it is somebody
// else's business and not a leak.
func LeakedResources(before, after AwsResourceInventory) []string {
	var leaked []string
	for resourceType, afterIds := range after {
		beforeIds := map[string]bool{}
		for _, id := range before[resourceType] {
			beforeIds[id] = true
		}
		for _, id := range afterIds {
			if !beforeIds[id] {
				leaked = append(leaked, fmt.Sprintf("%s %s", resourceType, id))
			}
		}
	}
	sort.Strings(leaked)

	return leaked
}
