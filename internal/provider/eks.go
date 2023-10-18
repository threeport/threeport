package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/nukleros/eks-cluster/pkg/connection"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gopkg.in/ini.v1"

	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// KubernetesRuntimeInfraEKS represents the infrastructure for a threeport-managed EKS
// cluster.
type KubernetesRuntimeInfraEKS struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// The AWS account ID where the cluster infra is provisioned.
	AwsAccountID string

	// The configuration containing credentials to connect to an AWS account.
	AwsConfig *aws.Config

	// The eks-cluster client used to create AWS EKS resources.
	ResourceClient *resource.ResourceClient

	// The inventory of AWS resources used to run an EKS cluster.
	ResourceInventory *resource.ResourceInventory

	// The number of availability zones the eks-cluster will be deployed across.
	ZoneCount int32

	// The AWS isntance type used for the default node group.
	DefaultNodeGroupInstanceType string

	// The number of nodes initially created for the default node group.
	DefaultNodeGroupInitialNodes int32

	// The minimum number of nodes to maintain in the default node group.
	DefaultNodeGroupMinNodes int32

	// The maximum number of nodes allowed in the default node group.
	DefaultNodeGroupMaxNodes int32
}

// Create installs a Kubernetes cluster using AWS EKS for threeport workloads.
func (i *KubernetesRuntimeInfraEKS) Create() (*kube.KubeConnectionInfo, error) {
	// create a new resource config to configure Kubernetes cluster
	resourceConfig := resource.NewResourceConfig()
	resourceConfig.Name = i.RuntimeInstanceName
	resourceConfig.AWSAccountID = i.AwsAccountID
	resourceConfig.DesiredAZCount = i.ZoneCount
	resourceConfig.InstanceTypes = []string{i.DefaultNodeGroupInstanceType}
	resourceConfig.InitialNodes = i.DefaultNodeGroupInitialNodes
	resourceConfig.MinNodes = i.DefaultNodeGroupMinNodes
	resourceConfig.MaxNodes = i.DefaultNodeGroupMaxNodes
	resourceConfig.DNSManagement = true
	resourceConfig.DNS01Challenge = true
	resourceConfig.DNSManagementServiceAccount = resource.DNSManagementServiceAccount{
		Name:      threeport.DNSManagerServiceAccountName,
		Namespace: threeport.DNSManagerServiceAccountNamepace,
	}
	resourceConfig.DNS01ChallengeServiceAccount = resource.DNS01ChallengeServiceAccount{
		Name:      threeport.DNS01ChallengeServiceAccountName,
		Namespace: threeport.DNS01ChallengeServiceAccountNamepace,
	}
	resourceConfig.ClusterAutoscaling = true
	resourceConfig.ClusterAutoscalingServiceAccount = resource.ClusterAutoscalingServiceAccount{
		Name:      threeport.ClusterAutoscalerServiceAccountName,
		Namespace: threeport.ClusterAutoscalerServiceAccountNamespace,
	}
	resourceConfig.StorageManagementServiceAccount = resource.StorageManagementServiceAccount{
		Name:      threeport.StorageManagerServiceAccountName,
		Namespace: threeport.StorageManagerServiceAccountNamespace,
	}
	resourceConfig.Tags = ThreeportProviderTags()

	// create EKS cluster resource stack in AWS
	if err := i.ResourceClient.CreateResourceStack(resourceConfig); err != nil {
		return nil, fmt.Errorf("failed to create eks resource stack: %w", err)
	}

	// get kubernetes API connection info
	eksClusterConn := connection.EKSClusterConnectionInfo{ClusterName: i.RuntimeInstanceName}
	if err := eksClusterConn.Get(i.AwsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info: %w", err)
	}
	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:        eksClusterConn.APIEndpoint,
		CACertificate:      eksClusterConn.CACertificate,
		EKSToken:           eksClusterConn.Token,
		EKSTokenExpiration: eksClusterConn.TokenExpiration,
	}

	return &kubeConnInfo, nil
}

// Delete deletes an AWS EKS cluster.
func (i *KubernetesRuntimeInfraEKS) Delete() error {
	// delete EKS cluster resources
	if err := i.ResourceClient.DeleteResourceStack(i.ResourceInventory); err != nil {
		return fmt.Errorf("failed to delete eks cluster resource stack: %w", err)
	}

	return nil
}

// RefreshConnection gets a new token for authentication to an EKS cluster.
func (i *KubernetesRuntimeInfraEKS) RefreshConnection() (*kube.KubeConnectionInfo, error) {
	// get connection info
	eksClusterConn := connection.EKSClusterConnectionInfo{
		ClusterName: i.RuntimeInstanceName,
	}
	if err := eksClusterConn.Get(i.AwsConfig); err != nil {
		return nil, fmt.Errorf("failed to retrieve EKS cluster connection info for token refresh: %w", err)
	}

	// construct KubeConnectionInfo object
	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:        eksClusterConn.APIEndpoint,
		CACertificate:      eksClusterConn.CACertificate,
		EKSToken:           eksClusterConn.Token,
		EKSTokenExpiration: eksClusterConn.TokenExpiration,
	}

	return &kubeConnInfo, nil
}

// EKSInventoryFilepath returns a standardized filename and path for the EKS
// inventory file.
func EKSInventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("eks-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}

// GetKeysFromLocalConfig returns the access key ID and secret access key from
// either the environment or local AWS credentials.
func GetKeysFromLocalConfig(profile string) (string, string, error) {
	envAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	envSecretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if envAccessKeyID != "" && envSecretAccessKey != "" {
		return envAccessKeyID, envSecretAccessKey, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	awsCredentials, err := ini.Load(filepath.Join(homeDir, ".aws", "credentials"))
	if err != nil {
		return "", "", fmt.Errorf("failed to load aws credentials: %w", err)
	}
	var accessKeyID string
	var secretAccessKey string
	if awsCredentials.Section(profile).HasKey("aws_access_key_id") &&
		awsCredentials.Section(profile).HasKey("aws_secret_access_key") {
		accessKeyID = awsCredentials.Section(profile).Key("aws_access_key_id").String()
		secretAccessKey = awsCredentials.Section(profile).Key("aws_secret_access_key").String()
	} else {
		return "", "", errors.New("unable to get AWS credentials from environment or local credentials")
	}

	return accessKeyID, secretAccessKey, nil
}

func DeleteThreeportIamResources(instanceName string, awsConfig aws.Config) error {
	var nse *types.NoSuchEntityException
	var err error
	if err = DeleteRole(instanceName, awsConfig); err != nil && !errors.As(err, &nse) {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	if err = DeleteServiceAccountPolicy(instanceName, awsConfig); err != nil && !errors.As(err, &nse) {
		return fmt.Errorf("failed to delete service account policy: %w", err)
	}

	if err = DeleteServiceAccount(instanceName, awsConfig); err != nil && !errors.As(err, &nse) {
		return fmt.Errorf("failed to delete service account: %w", err)
	}
	return nil
}

// CreateServiceAccountPolicy creates the IAM policy to be used for the
// threeport service account policy.
func CreateServiceAccountPolicy(
	tags *[]types.Tag,
	clusterName string,
	runtimeManagementRoleArn string,
	awsConfig aws.Config,
) (*types.Policy, error) {
	svc := iam.NewFromConfig(awsConfig)

	serviceAccountPolicyName := fmt.Sprintf("%s-%s", ServiceAccountPolicyName, clusterName)
	serviceAccountPolicyDescription := "Allow Threeport to manage runtimes."
	serviceAccountPolicyDocument := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
						{
				"Sid": "AssumeRole",
				"Effect": "Allow",
				"Action": [
					"sts:AssumeRole"
				],
				"Resource": [
					"%s"
				]
			}
		]
}`, runtimeManagementRoleArn)

	createServiceAccountPolicyInput := iam.CreatePolicyInput{
		PolicyName:     &serviceAccountPolicyName,
		Description:    &serviceAccountPolicyDescription,
		PolicyDocument: &serviceAccountPolicyDocument,
	}
	serviceAccountPolicyResp, err := svc.CreatePolicy(context.Background(), &createServiceAccountPolicyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster autoscaler management policy %s: %w", serviceAccountPolicyName, err)
	}

	return serviceAccountPolicyResp.Policy, nil
}

func DeleteServiceAccountPolicy(
	clusterName string,
	awsConfig aws.Config,
) error {
	svc := iam.NewFromConfig(awsConfig)
	runtimeServiceAccount := fmt.Sprintf("%s-%s", RuntimeServiceAccount, clusterName)

	attachedPolicies, err := svc.ListAttachedUserPolicies(
		context.Background(),
		&iam.ListAttachedUserPoliciesInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		return fmt.Errorf("failed to list attached user policies: %s\n", err)
	}

	for _, policy := range attachedPolicies.AttachedPolicies {
		_, err := svc.DetachUserPolicy(
			context.Background(),
			&iam.DetachUserPolicyInput{
				PolicyArn: policy.PolicyArn,
				UserName:  &runtimeServiceAccount,
			})
		if err != nil {
			return fmt.Errorf("failed to detach user policy: %s\n", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: policy.PolicyArn,
			})
		if err != nil {
			return fmt.Errorf("failed to delete policy: %s\n", err)
		}
	}

	return nil
}

func CreateServiceAccount(serviceAccountPolicyArn, clusterName string, awsConfig *aws.Config) (*types.User, *types.AccessKey, error) {
	svc := iam.NewFromConfig(*awsConfig)
	runtimeServiceAccount := fmt.Sprintf("%s-%s", RuntimeServiceAccount, clusterName)

	// create the service account
	createUserInput := iam.CreateUserInput{
		UserName: &runtimeServiceAccount,
	}
	createUserOutput, err := svc.CreateUser(context.Background(), &createUserInput)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create IAM user: %w", err)
	}

	// attach the policy to the user
	_, err = svc.AttachUserPolicy(
		context.Background(),
		&iam.AttachUserPolicyInput{
			UserName:  &runtimeServiceAccount,
			PolicyArn: &serviceAccountPolicyArn,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to attach IAM policy to user: %w", err)
	}

	// create an access key for the user
	createAccessKeyOutput, err := svc.CreateAccessKey(
		context.Background(),
		&iam.CreateAccessKeyInput{
			UserName: createUserOutput.User.UserName,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create IAM access key: %w", err)
	}
	return createUserOutput.User, createAccessKeyOutput.AccessKey, nil
}

func DeleteServiceAccount(
	clusterName string,
	awsConfig aws.Config,
) error {
	runtimeServiceAccount := fmt.Sprintf("%s-%s", RuntimeServiceAccount, clusterName)
	svc := iam.NewFromConfig(awsConfig)
	accessKeys, err := svc.ListAccessKeys(
		context.Background(),
		&iam.ListAccessKeysInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		return fmt.Errorf("failed to list access keys: %s\n", err)
	}

	for _, accessKey := range accessKeys.AccessKeyMetadata {
		_, err := svc.DeleteAccessKey(
			context.Background(),
			&iam.DeleteAccessKeyInput{
				AccessKeyId: accessKey.AccessKeyId,
				UserName:    &runtimeServiceAccount,
			})
		if err != nil {
			return fmt.Errorf("failed to delete access key: %s\n", err)
		}
	}

	_, err = svc.DeleteUser(
		context.Background(),
		&iam.DeleteUserInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		return fmt.Errorf("failed to delete service account: %s\n", err)
	}
	return nil
}

// CreateStorageManagementRole creates the IAM role needed for storage
// management by the CSI driver's service account using IRSA (IAM role for
// service accounts).
func CreateRuntimeManagementRole(
	tags *[]types.Tag,
	clusterName string,
	accountId string,
	awsConfig aws.Config,
) (*types.Role, error) {
	svc := iam.NewFromConfig(awsConfig)

	runtimeManagementRoleName := fmt.Sprintf("%s-%s", RuntimeManagementRoleName, clusterName)
	// if err := checkRoleName(runtimeManagementRoleName); err != nil {
	// 	return nil, err
	// }
	runtimeManagerTrustPolicyDocument := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"AWS": "arn:aws:iam::%s:root"
				},
				"Action": "sts:AssumeRole"
			}
		]
	}`, accountId)
	runtimeManagerPolicyDocument := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "EC2andIAMPermissions",
				"Effect": "Allow",
				"Action": [
					"ec2:CreateVpc",
					"ec2:DeleteVpc",
					"ec2:ModifyVpcAttribute",
					"ec2:CreateSubnet",
					"ec2:DeleteSubnet",
					"ec2:ModifySubnetAttribute",
					"ec2:DescribeSubnets",
					"ec2:CreateRouteTable",
					"ec2:DeleteRouteTable",
					"ec2:CreateRoute",
					"ec2:DeleteRoute",
					"ec2:AssociateRouteTable",
					"ec2:DisassociateRouteTable",
					"ec2:AllocateAddress",
					"ec2:ReleaseAddress",
					"ec2:AssociateAddress",
					"ec2:DisassociateAddress",
					"ec2:CreateInternetGateway",
					"ec2:DeleteInternetGateway",
					"ec2:AttachInternetGateway",
					"ec2:DetachInternetGateway",
					"ec2:CreateNatGateway",
					"ec2:DeleteNatGateway",
					"ec2:CreateTags",
					"ec2:DeleteTags",
					"ec2:DescribeTags",
					"ec2:DescribeNatGateways",
					"ec2:DescribeAvailabilityZones",
					"ec2:DescribeSecurityGroups"
				],
				"Resource": "*"
			},
			{
				"Sid": "EKSPermissions",
				"Effect": "Allow",
				"Action": [
					"eks:CreateCluster",
					"eks:DeleteCluster",
					"eks:UpdateClusterConfig",
					"eks:CreateNodegroup",
					"eks:DeleteNodegroup",
					"eks:UpdateNodegroupConfig",
					"eks:DescribeNodegroup",
					"eks:TagResource",
					"eks:UntagResource",
					"eks:DescribeCluster",
					"eks:CreateAddon",
					"eks:DeleteAddon",
					"eks:UpdateAddon"
				],
				"Resource": "*"
			},
			{
				"Sid": "IAMPermissions",
				"Effect": "Allow",
				"Action": [
					"iam:CreateOpenIDConnectProvider",
					"iam:DeleteOpenIDConnectProvider",
					"iam:UpdateOpenIDConnectProviderThumbprint",
					"iam:CreatePolicy",
					"iam:DeletePolicy",
					"iam:CreatePolicyVersion",
					"iam:DeletePolicyVersion",
					"iam:SetDefaultPolicyVersion",
					"iam:GetRole",
					"iam:CreateRole",
					"iam:DeleteRole",
					"iam:UpdateRole",
					"iam:PutRolePolicy",
					"iam:DeleteRolePolicy",
					"iam:AttachRolePolicy",
					"iam:DetachRolePolicy",
					"iam:TagRole",
					"iam:UntagRole",
					"iam:ListAttachedRolePolicies",
					"iam:DescribeSecurityGroups"
				],
				"Resource": "*"
			},
			{
				"Sid": "IAMPassRolePermissions",
				"Effect": "Allow",
				"Action": "iam:PassRole",
				"Resource": "*",
				"Condition": {
					"StringEquals": {
						"iam:PassedToService": [
							"ec2.amazonaws.com",
							"vpc.amazonaws.com",
							"eks.amazonaws.com",
							"ebs.amazonaws.com",
							"route53.amazonaws.com"
						]
					}
				}
			}
		]
	}`

	createRuntimeManagementRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &runtimeManagerTrustPolicyDocument,
		RoleName:                 &runtimeManagementRoleName,
		Tags:                     *tags,
	}
	runtimeManagementRoleResp, err := svc.CreateRole(context.Background(), &createRuntimeManagementRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", runtimeManagementRoleName, err)
	}

	rolePolicyInput := iam.CreatePolicyInput{
		PolicyName:     &runtimeManagementRoleName,
		Description:    &runtimeManagementRoleName,
		PolicyDocument: &runtimeManagerPolicyDocument,
	}

	createdRolePolicy, err := svc.CreatePolicy(context.Background(), &rolePolicyInput)
	if err != nil {
		return runtimeManagementRoleResp.Role, fmt.Errorf("failed to create role policy %s: %w", runtimeManagementRoleName, err)
	}

	attachRuntimeManagementRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: createdRolePolicy.Policy.Arn,
		RoleName:  runtimeManagementRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(context.Background(), &attachRuntimeManagementRolePolicyInput)
	if err != nil {
		return runtimeManagementRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", *createdRolePolicy.Policy.Arn, runtimeManagementRoleName, err)
	}

	return runtimeManagementRoleResp.Role, nil
}

func DeleteRole(
	clusterName string,
	awsConfig aws.Config,
) error {
	svc := iam.NewFromConfig(awsConfig)
	runtimeManagementRoleName := fmt.Sprintf("%s-%s", RuntimeManagementRoleName, clusterName)
	roles, err := svc.ListAttachedRolePolicies(
		context.Background(),
		&iam.ListAttachedRolePoliciesInput{
			RoleName: &runtimeManagementRoleName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list attached role policies: %s\n", err)
	}
	for _, role := range roles.AttachedPolicies {
		_, err := svc.DetachRolePolicy(
			context.Background(),
			&iam.DetachRolePolicyInput{
				PolicyArn: role.PolicyArn,
				RoleName:  &runtimeManagementRoleName,
			})
		if err != nil {
			return fmt.Errorf("failed to detach role policy: %s\n", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: role.PolicyArn,
			})
		if err != nil {
			return fmt.Errorf("failed to delete policy: %s\n", err)
		}
	}

	_, err = svc.DeleteRole(
		context.Background(),
		&iam.DeleteRoleInput{
			RoleName: &runtimeManagementRoleName,
		})
	if err != nil {
		return fmt.Errorf("failed to delete role: %s\n", err)
	}
	return nil
}

const (
	ServiceAccountPolicyName  = "ThreeportServiceAccount"
	RuntimeServiceAccount     = "ThreeportRuntime"
	RuntimeManagementRoleName = "ThreeportRuntimeManagement"
)
