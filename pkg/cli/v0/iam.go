package v0

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// CreateServiceAccountPolicy creates the IAM policy to be used for the
// threeport service account policy.
func CreateServiceAccountPolicy(
	tags *[]types.Tag,
	clusterName string,
	runtimeManagementRoleArn string,
	awsConfig *aws.Config,
) (*types.Policy, error) {
	svc := iam.NewFromConfig(*awsConfig)

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
	awsConfig *aws.Config,
) error {
	svc := iam.NewFromConfig(*awsConfig)
	runtimeServiceAccount := fmt.Sprintf("%s-%s", RuntimeServiceAccount, clusterName)

	attachedPolicies, err := svc.ListAttachedUserPolicies(
		context.Background(),
		&iam.ListAttachedUserPoliciesInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		fmt.Printf("failed to list attached user policies: %s\n", err)
	}

	for _, policy := range attachedPolicies.AttachedPolicies {
		_, err := svc.DetachUserPolicy(
			context.Background(),
			&iam.DetachUserPolicyInput{
				PolicyArn: policy.PolicyArn,
				UserName:  &runtimeServiceAccount,
			})
		if err != nil {
			fmt.Printf("failed to detach user policy: %s\n", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: policy.PolicyArn,
			})
		if err != nil {
			fmt.Printf("failed to delete policy: %s\n", err)
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
	awsConfig *aws.Config,
) error {
	runtimeServiceAccount := fmt.Sprintf("%s-%s", RuntimeServiceAccount, clusterName)
	svc := iam.NewFromConfig(*awsConfig)
	accessKeys, err := svc.ListAccessKeys(
		context.Background(),
		&iam.ListAccessKeysInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		fmt.Printf("failed to list access keys: %s\n", err)
	}

	for _, accessKey := range accessKeys.AccessKeyMetadata {
		_, err := svc.DeleteAccessKey(
			context.Background(),
			&iam.DeleteAccessKeyInput{
				AccessKeyId: accessKey.AccessKeyId,
				UserName:    &runtimeServiceAccount,
			})
		if err != nil {
			fmt.Printf("failed to delete access key: %s\n", err)
		}
	}

	_, err = svc.DeleteUser(
		context.Background(),
		&iam.DeleteUserInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		fmt.Printf("failed to delete service account: %s\n", err)
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
	awsConfig *aws.Config,
) (*types.Role, error) {
	svc := iam.NewFromConfig(*awsConfig)

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
	awsConfig *aws.Config,
) error {
	svc := iam.NewFromConfig(*awsConfig)
	runtimeManagementRoleName := fmt.Sprintf("%s-%s", RuntimeManagementRoleName, clusterName)
	roles, err := svc.ListAttachedRolePolicies(
		context.Background(),
		&iam.ListAttachedRolePoliciesInput{
			RoleName: &runtimeManagementRoleName,
		},
	)
	if err != nil {
		fmt.Printf("failed to list attached role policies: %s\n", err)
	}
	for _, role := range roles.AttachedPolicies {
		_, err := svc.DetachRolePolicy(
			context.Background(),
			&iam.DetachRolePolicyInput{
				PolicyArn: role.PolicyArn,
				RoleName:  &runtimeManagementRoleName,
			})
		if err != nil {
			fmt.Printf("failed to detach role policy: %s\n", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: role.PolicyArn,
			})
		if err != nil {
			fmt.Printf("failed to delete policy: %s\n", err)
		}
	}

	_, err = svc.DeleteRole(
		context.Background(),
		&iam.DeleteRoleInput{
			RoleName: &runtimeManagementRoleName,
		})
	if err != nil {
		fmt.Printf("failed to delete role: %s\n", err)
	}
	return nil
}

const (
	ServiceAccountPolicyName  = "ThreeportServiceAccount"
	RuntimeServiceAccount     = "ThreeportRuntime"
	RuntimeManagementRoleName = "ThreeportRuntimeManagement"
)
