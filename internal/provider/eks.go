package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// KubernetesRuntimeInfraEKS represents the infrastructure for a threeport-managed EKS
// cluster.
type KubernetesRuntimeInfraEKS struct {
	// PulumiWorkspace provides workspace, stack, state, and automation API
	// helpers (aligned with KubernetesRuntimeInfraGKE and
	// KubernetesRuntimeInfraOKE).  It carries RuntimeInstanceName.
	PulumiWorkspace

	// The AWS account ID where the cluster infra is provisioned.
	AwsAccountID string

	// The AWS region where the cluster infra is provisioned.
	Region string

	// Kubernetes version of the EKS cluster.
	KubernetesVersion string

	// The ARN of the IAM role the EKS control plane assumes.  Created by the
	// IAM bootstrap that runs outside the Pulumi program.
	ClusterRoleArn string

	// The ARN of the IAM role the worker nodes assume.  Created by the IAM
	// bootstrap that runs outside the Pulumi program.
	NodeRoleArn string

	// The configuration containing credentials to connect to an AWS account.
	AwsConfig *aws.Config

	// The number of availability zones the EKS cluster will be deployed across.
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

// Create provisions an AWS EKS cluster for threeport workloads and returns the
// connection info for its Kubernetes API.
func (i *KubernetesRuntimeInfraEKS) Create() (*kube.KubeConnectionInfo, error) {
	return i.CreateInfra()
}

// CreateInfra creates the IAM the cluster needs, provisions the cluster's
// networking, the cluster and its node group with Pulumi, and then creates the
// IAM that only exists once the cluster does.
func (i *KubernetesRuntimeInfraEKS) CreateInfra() (*kube.KubeConnectionInfo, error) {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	// EKS will not create a cluster or a node group without the roles they
	// assume, so these come before the stack
	if err := i.CreateClusterIam(); err != nil {
		return nil, fmt.Errorf("failed to create EKS cluster IAM roles: %w", err)
	}

	stack, err := i.SetupStack(i.pulumiProgram())
	if err != nil {
		return nil, fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}
	if _, err := i.RunUp(context.Background(), stack); err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	// the OIDC issuer the add-ons' roles are bound to is stood up with the
	// cluster, so these come after the stack
	if err := i.CreateIrsaIam(); err != nil {
		return nil, fmt.Errorf("failed to create IRSA IAM resources: %w", err)
	}

	return i.GetConnection()
}

// DeployInfra runs CreateInfra and discards the connection info.  It satisfies
// InfraProvider for the shared infrastructure lifecycle.
func (i *KubernetesRuntimeInfraEKS) DeployInfra() error {
	_, err := i.CreateInfra()

	return err
}

// DestroyInfra tears down EKS infrastructure via Delete.  It satisfies
// InfraProvider.
func (i *KubernetesRuntimeInfraEKS) DestroyInfra() error {
	return i.Delete()
}

// Delete destroys an AWS EKS cluster and the IAM resources created alongside
// it.
func (i *KubernetesRuntimeInfraEKS) Delete() error {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	if err := i.DestroyStack(); err != nil {
		return fmt.Errorf("failed to destroy EKS stack: %w", err)
	}

	// IAM is created outside the stack, so it is removed outside it too.
	// Unlike GKE, which logs a cleanup failure and confirms the delete anyway,
	// this returns the error so that deletion is not confirmed while roles,
	// policies and an identity provider are still in the account.  Both this
	// and the stack destroy tolerate resources that are already gone, so the
	// lifecycle engine retrying costs nothing.
	if err := i.DeleteEksIam(); err != nil {
		return fmt.Errorf("failed to delete EKS IAM resources: %w", err)
	}

	return nil
}

// GetStackState returns the state of the EKS stack as a JSON object.
func (i *KubernetesRuntimeInfraEKS) GetStackState() (*datatypes.JSON, error) {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	return i.PulumiWorkspace.GetStackState()
}

// SetStackState restores Pulumi state from a JSON object (export or checkpoint
// format).
func (i *KubernetesRuntimeInfraEKS) SetStackState(state *datatypes.JSON) error {
	i.ensurePulumiProjectDefaults()
	i.syncStackConfigs()

	return i.PulumiWorkspace.SetStackState(state)
}

// GetConnection gets the latest connection infor for authentication to an EKS cluster.
func (i *KubernetesRuntimeInfraEKS) GetConnection() (*kube.KubeConnectionInfo, error) {
	// get connection info
	eksClusterConn := tpaws.EksClusterConnectionInfo{
		ClusterName: i.RuntimeInstanceName,
	}
	if err := eksClusterConn.Get(i.AwsConfig); err != nil {
		return nil, fmt.Errorf("failed to retrieve EKS cluster connection info for token refresh: %w", err)
	}

	// construct KubeConnectionInfo object
	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:     eksClusterConn.APIEndpoint,
		CACertificate:   eksClusterConn.CACertificate,
		Token:           eksClusterConn.Token,
		TokenExpiration: eksClusterConn.TokenExpiration,
	}

	return &kubeConnInfo, nil
}

// DeleteResourceManagerRole deletes the IAM resources created by threeport
// for a given cluster.
func DeleteResourceManagerRole(instanceName string, awsConfig aws.Config) error {
	var nse types.NoSuchEntityException
	var err error
	if err = deleteRole(
		GetResourceManagerRoleName(instanceName),
		awsConfig,
	); err != nil && !IsException(&err, nse.ErrorCode()) {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

// IsException returns true if the error is a specific exception,
// otherwise it returns false and updates the error with additional context.
func IsException(err *error, exception string) bool {
	var ae smithy.APIError
	var oe *smithy.OperationError
	if errors.As(*err, &ae) {
		if exception != "" && strings.Contains((*err).Error(), exception) {
			return true
		}
		newError := fmt.Errorf("code: %s, message: %s, fault: %s", ae.ErrorCode(), ae.ErrorMessage(), ae.ErrorFault().String())
		*err = newError
	}
	if errors.As(*err, &oe) {
		if exception != "" && strings.Contains((*err).Error(), exception) {
			return true
		}
		newError := fmt.Errorf("failed to call service: %s, operation: %s, error: %v", oe.Service(), oe.Operation(), oe.Unwrap())
		*err = newError
	}
	return false
}

// CreateServiceAccountPolicy creates the IAM policy to be used for the
// threeport service account policy.
func CreateServiceAccountPolicy(
	tags *[]types.Tag,
	clusterName string,
	resourceManagerRoleArn string,
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
}`, resourceManagerRoleArn)

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

// DeleteServiceAccountPolicy deletes the IAM policy used by the threeport
// service account.
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
		return fmt.Errorf("failed to list attached user policies: %s", err)
	}

	for _, policy := range attachedPolicies.AttachedPolicies {
		_, err := svc.DetachUserPolicy(
			context.Background(),
			&iam.DetachUserPolicyInput{
				PolicyArn: policy.PolicyArn,
				UserName:  &runtimeServiceAccount,
			})
		if err != nil {
			return fmt.Errorf("failed to detach user policy: %s", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: policy.PolicyArn,
			})
		if err != nil {
			return fmt.Errorf("failed to delete policy: %s", err)
		}
	}

	return nil
}

// CreateServiceAccount creates the IAM user and access key for the threeport
// service account.
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

// DeleteServiceAccount deletes the IAM user and access key for the threeport
// service account.
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
		return fmt.Errorf("failed to list access keys: %s", err)
	}

	for _, accessKey := range accessKeys.AccessKeyMetadata {
		_, err := svc.DeleteAccessKey(
			context.Background(),
			&iam.DeleteAccessKeyInput{
				AccessKeyId: accessKey.AccessKeyId,
				UserName:    &runtimeServiceAccount,
			})
		if err != nil {
			return fmt.Errorf("failed to delete access key: %s", err)
		}
	}

	_, err = svc.DeleteUser(
		context.Background(),
		&iam.DeleteUserInput{
			UserName: &runtimeServiceAccount,
		})
	if err != nil {
		return fmt.Errorf("failed to delete service account: %s", err)
	}
	return nil
}

// CreateResourceManagerRole creates the IAM role needed for resource
// management.
func CreateResourceManagerRole(
	namespace string,
	tags []types.Tag,
	roleName,
	accountId,
	externalAccountId,
	principalRoleName,
	externalId string,
	attachAssumeAnyRolePolicy bool,
	attachResourceManagerPolicy bool,
	awsConfig aws.Config,
	additionalIrsaConditions []string,
) (*types.Role, error) {
	svc := iam.NewFromConfig(awsConfig)

	// ensure role name is valid
	if err := tpaws.CheckIamRoleName(roleName); err != nil {
		return nil, err
	}

	// create trust policy document
	resourceManagerTrustPolicyDocument, err := getResourceManagerTrustPolicyDocument(namespace, principalRoleName, accountId, externalAccountId, externalId, "", additionalIrsaConditions)
	if err != nil {
		return nil, fmt.Errorf("failed to get role trust policy document: %w", err)
	}
	createResourceManagerRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &resourceManagerTrustPolicyDocument,
		RoleName:                 &roleName,
		Tags:                     tags,
	}

	// create the role
	resourceManagerRoleResp, err := svc.CreateRole(context.Background(), &createResourceManagerRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", roleName, err)
	}

	// attach assume any role policy
	if attachAssumeAnyRolePolicy {
		if err := AttachPolicy(AssumeAnyRolePolicyDocument, roleName, "assume-any-role", svc); err != nil {
			return resourceManagerRoleResp.Role, fmt.Errorf("failed to attach policy to role %s: %w", roleName, err)
		}
	}

	// attach resource manager policy if requested
	if attachResourceManagerPolicy {
		if err := AttachPolicy(ResourceManagerPolicyDocument, roleName, "resource-manager", svc); err != nil {
			return resourceManagerRoleResp.Role, fmt.Errorf("failed to attach policy to role %s: %w", roleName, err)
		}
	}

	return resourceManagerRoleResp.Role, nil
}

// AttachPolicy attaches a given document to a role.
func AttachPolicy(document, roleName, policyName string, svc *iam.Client) error {
	policyInputName := fmt.Sprintf("%s-%s", roleName, policyName)
	// create role policy
	rolePolicyInput := iam.CreatePolicyInput{
		PolicyName:     &policyInputName,
		Description:    &roleName,
		PolicyDocument: &document,
	}
	createdRolePolicy, err := svc.CreatePolicy(context.Background(), &rolePolicyInput)
	if err != nil {
		return fmt.Errorf("failed to create role policy %s: %w", roleName, err)
	}

	// attach role policy
	attachResourceManagerRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: createdRolePolicy.Policy.Arn,
		RoleName:  &roleName,
	}
	_, err = svc.AttachRolePolicy(context.Background(), &attachResourceManagerRolePolicyInput)
	if err != nil {
		return fmt.Errorf("failed to attach role policy %s to %s: %w", *createdRolePolicy.Policy.Arn, roleName, err)
	}

	return nil
}

// UpdateResourceManagerRoleTrustPolicy updates the IAM role needed for resource
// management.
func UpdateResourceManagerRoleTrustPolicy(namespace, clusterName, accountId, externalId, oidcProviderUrl string, awsConfig aws.Config, additionalIrsaConditions []string) error {
	svc := iam.NewFromConfig(awsConfig)

	resourceManagerRoleName := GetResourceManagerRoleName(clusterName)

	// update trust policy document
	runtimeManagerTrustPolicyDocument, err := getResourceManagerTrustPolicyDocument(namespace, "", accountId, "", externalId, oidcProviderUrl, additionalIrsaConditions)
	if err != nil {
		return fmt.Errorf("failed to get role trust policy document: %w", err)
	}

	// update role trust policy
	updateResourceManagerRoleInput := iam.UpdateAssumeRolePolicyInput{
		RoleName:       &resourceManagerRoleName,
		PolicyDocument: &runtimeManagerTrustPolicyDocument,
	}
	_, err = svc.UpdateAssumeRolePolicy(context.Background(), &updateResourceManagerRoleInput)
	if err != nil {
		return fmt.Errorf("failed to update role %s: %w", resourceManagerRoleName, err)
	}

	return nil
}

// deleteRole deletes the runtime manager IAM role.
func deleteRole(
	roleName string,
	awsConfig aws.Config,
) error {
	svc := iam.NewFromConfig(awsConfig)
	roles, err := svc.ListAttachedRolePolicies(
		context.Background(),
		&iam.ListAttachedRolePoliciesInput{
			RoleName: &roleName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list attached role policies: %s", err)
	}
	for _, role := range roles.AttachedPolicies {
		_, err := svc.DetachRolePolicy(
			context.Background(),
			&iam.DetachRolePolicyInput{
				PolicyArn: role.PolicyArn,
				RoleName:  &roleName,
			})
		if err != nil {
			return fmt.Errorf("failed to detach role policy: %s", err)
		}
		_, err = svc.DeletePolicy(
			context.Background(),
			&iam.DeletePolicyInput{
				PolicyArn: role.PolicyArn,
			})
		if err != nil {
			return fmt.Errorf("failed to delete policy: %s", err)
		}
	}

	_, err = svc.DeleteRole(
		context.Background(),
		&iam.DeleteRoleInput{
			RoleName: &roleName,
		})
	if err != nil {
		return fmt.Errorf("failed to delete role: %s", err)
	}
	return nil
}

// GetResourceManagerRoleName returns the name of the runtime manager role.
func GetResourceManagerRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", ResourceManagerRoleName, clusterName)
}

// GetResourceManagerRoleArn returns the ARN for the runtime manager role.
func GetResourceManagerRoleArn(clusterName, accountId string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, GetResourceManagerRoleName(clusterName))
}

// GetCallerIdentity returns the caller identity for the AWS account.
func GetCallerIdentity(awsConfig *aws.Config) (*sts.GetCallerIdentityOutput, error) {
	svc := sts.NewFromConfig(*awsConfig)
	callerIdentity, err := svc.GetCallerIdentity(
		context.Background(),
		&sts.GetCallerIdentityInput{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get caller identity: %w", err)
	}

	return callerIdentity, nil
}

// getAllowAccountAccessStatement returns a statement for allowing account access.
func getAllowAccountAccessStatement(identityService, accountId, accountEntity string) map[string]interface{} {
	// construct statement for allowing account access
	allowAccountAccessStatement := map[string]interface{}{
		"Effect": "Allow",
		"Principal": map[string]interface{}{
			"AWS": "arn:aws:" + identityService + "::" + accountId + ":" + accountEntity,
		},
		"Action": "sts:AssumeRole",
	}

	return allowAccountAccessStatement
}

// getResourceManagerTrustPolicyDocument returns the trust policy document for the
// runtime manager role.
func getResourceManagerTrustPolicyDocument(namespace, externalRoleName, accountId, externalAccountId, externalId, oidcProviderUrl string, additionalIrsaConditions []string) (string, error) {

	statements := []interface{}{}

	// default account entity to root account
	accountEntity := "root"

	// default identity service to iam
	identityService := "iam"

	// add default statement for allowing all entities within current account to assume this one
	statements = append(statements, getAllowAccountAccessStatement(identityService, accountId, accountEntity))

	//  if role name is provided, set identity service to sts
	// and set account entity to the expected role and session name
	// and add an additional allow account access statement
	if externalRoleName != "" {
		identityService = "sts"
		accountEntity = "assumed-role/" + externalRoleName + "/" + util.AwsResourceManagerRoleSessionName

		allowAccountAccessStatement := getAllowAccountAccessStatement(identityService, externalAccountId, accountEntity)

		// if externalId is provided, add a conditional statement
		// that requires the externalId to be provided
		if externalId != "" {
			allowAccountAccessStatement["Condition"] = map[string]interface{}{
				"StringEquals": map[string]interface{}{
					"sts:ExternalId": externalId,
				},
			}
		}

		// append the allow account access statement
		statements = append(statements, allowAccountAccessStatement)
	}

	// if oidcProviderUrl is provided, add a statement that allows
	// a kubernetes service account to assume the role
	if oidcProviderUrl != "" {

		// remove scheme prefix from url
		url, err := url.Parse(oidcProviderUrl)
		if err != nil {
			return "", fmt.Errorf("failed to parse oidc provider url: %w", err)
		}
		basenameAndPath := url.Hostname() + url.Path

		// build list of valid condition values
		conditionValues := []interface{}{}
		for _, serviceAccount := range IrsaControllerNames() {
			conditionValue := "system:serviceaccount:" + namespace + ":" + serviceAccount
			conditionValues = append(conditionValues, conditionValue)
		}

		for _, ic := range additionalIrsaConditions {
			conditionValues = append(conditionValues, ic)
		}

		// construct statement for allowing a kubernetes service account
		// to assume the role via a federated OIDC provider
		allowServiceAccountStatement := map[string]interface{}{
			"Effect": "Allow",
			"Principal": map[string]interface{}{
				"Federated": "arn:aws:iam::" + accountId + ":oidc-provider/" + basenameAndPath,
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": map[string]interface{}{
				"StringEquals": map[string]interface{}{
					basenameAndPath + ":sub": conditionValues,
				},
			},
		}

		statements = append(statements, allowServiceAccountStatement)
	}

	// construct the trust policy document
	document := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": statements,
	}

	documentJson, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("failed to marshall trust policy document: %w", err)
	}

	return string(documentJson), nil
}

// IrsaControllerNames returns a list of controllers
// which are configured for IRSA authentication.
func IrsaControllerNames() []string {
	return []string{
		threeport.ThreeportAwsControllerName,
		threeport.ThreeportSecretControllerName,
		threeport.ThreeportKubernetesWorkloadControllerName,
		threeport.ThreeportHelmWorkloadControllerName,
		threeport.ThreeportControlPlaneControllerName,
		threeport.ThreeportTerraformControllerName,
	}
}

// UpdateIrsaControllerList updates the list of control plane components
// to be configured for IRSA authentication.
func UpdateIrsaControllerList(list []*v0.ControlPlaneComponent) {
	serviceAccounts := IrsaControllerNames()
	for _, controller := range list {
		if util.StringListContains(controller.Name, serviceAccounts) {
			controller.ServiceAccountName = controller.Name
		}
	}
}

// GetIrsaServiceAccounts returns the service account
// configured for IRSA authentication.
func GetIrsaServiceAccounts(namespace, accountId, roleName string) []*unstructured.Unstructured {
	serviceAccounts := IrsaControllerNames()

	output := []*unstructured.Unstructured{}
	for _, name := range serviceAccounts {
		output = append(output, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ServiceAccount",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": namespace,
					"annotations": map[string]interface{}{
						"eks.amazonaws.com/role-arn": fmt.Sprintf(
							"arn:aws:iam::%s:role/%s",
							accountId,
							roleName,
						),
					},
				},
			},
		})
	}
	return output
}

const (
	ServiceAccountPolicyName    = "ThreeportServiceAccount"
	RuntimeServiceAccount       = "ThreeportRuntime"
	ResourceManagerRoleName     = "resource-manager-threeport"
	AssumeAnyRolePolicyDocument = `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "AssumeAnyRole",
				"Effect": "Allow",
				"Action": "sts:AssumeRole",
				"Resource": "arn:aws:iam::*:role/*"
			}
		]
	}`
	ResourceManagerPolicyDocument = `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "EC2Permissions",
				"Effect": "Allow",
				"Action": [
					"ec2:CreateVpc",
					"ec2:DeleteVpc",
					"ec2:DescribeVpcs",
					"ec2:ModifyVpcAttribute",
					"ec2:DescribeVpcAttribute",
					"ec2:CreateSubnet",
					"ec2:DeleteSubnet",
					"ec2:ModifySubnetAttribute",
					"ec2:DescribeSubnets",
					"ec2:CreateRouteTable",
					"ec2:DeleteRouteTable",
					"ec2:DescribeRouteTables",
					"ec2:AssociateRouteTable",
					"ec2:CreateRoute",
					"ec2:DeleteRoute",
					"ec2:DisassociateRouteTable",
					"ec2:AllocateAddress",
					"ec2:ReleaseAddress",
					"ec2:AssociateAddress",
					"ec2:DisassociateAddress",
					"ec2:DescribeAddresses",
					"ec2:CreateInternetGateway",
					"ec2:DeleteInternetGateway",
					"ec2:AttachInternetGateway",
					"ec2:DetachInternetGateway",
					"ec2:DescribeInternetGateways",
					"ec2:CreateNatGateway",
					"ec2:DeleteNatGateway",
					"ec2:DescribeNatGateways",
					"ec2:CreateTags",
					"ec2:DeleteTags",
					"ec2:DescribeTags",
					"ec2:DescribeAvailabilityZones",
					"ec2:CreateSecurityGroup",
					"ec2:DeleteSecurityGroup",
					"ec2:DescribeSecurityGroups",
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:AuthorizeSecurityGroupEgress"
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
				"Sid": "RDSPermissions",
				"Effect": "Allow",
				"Action": [
					"rds:CreateDBSubnetGroup",
					"rds:DeleteDBSubnetGroup",
					"rds:DescribeDBSubnetGroups",
					"rds:AddTagsToResource",
					"rds:ListTagsForResource",
					"rds:CreateDBInstance",
					"rds:DeleteDBInstance",
					"rds:DescribeDBInstances"
				],
				"Resource": "*"
			},
			{
				"Sid": "S3Permissions",
				"Effect": "Allow",
				"Action": [
					"s3:*"
				],
				"Resource": "*"
			},
			{
				"Sid": "SecretsManagerPermissions",
				"Effect": "Allow",
				"Action": [
					"secretsmanager:BatchGetSecretValue",
					"secretsmanager:ListSecrets",
					"secretsmanager:CreateSecret",
					"secretsmanager:DeleteSecret",
					"secretsmanager:GetSecretValue"
				],
				"Resource": "*"
			},
			{
				"Sid": "IAMPermissions",
				"Effect": "Allow",
				"Action": [
					"iam:ListOpenIDConnectProviders",
					"iam:CreateOpenIDConnectProvider",
					"iam:DeleteOpenIDConnectProvider",
					"iam:UpdateOpenIDConnectProviderThumbprint",
					"iam:CreatePolicy",
					"iam:DeletePolicy",
					"iam:ListPolicies",
					"iam:ListRolePolicies",
					"iam:ListInstanceProfilesForRole",
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
					"iam:CreateServiceLinkedRole",
					"iam:TagPolicy",
					"iam:UpdateAssumeRolePolicy"
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
							"route53.amazonaws.com",
							"rds.amazonaws.com",
							"s3.amazonaws.com"
						]
					}
				}
			}
		]
	}`
)
