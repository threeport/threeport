package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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
	// Used for the SDK calls this process makes directly.
	AwsConfig *aws.Config

	// ProviderCredentials are what the Pulumi AWS provider authenticates
	// with.  It runs in its own process and cannot be handed AwsConfig's
	// refreshing credential chain, so it gets the long-lived credentials and
	// the role to assume and renews them itself.
	ProviderCredentials tpaws.AwsProviderCredentials

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
	if err := i.DestroyStack(); err != nil {
		// A destroy that leaves subnets behind is usually the VPC CNI's doing:
		// it allocates secondary interfaces on the worker nodes for pod
		// addresses, Pulumi never knew about them, and AWS does not always
		// reap them before the subnets they sit in are deleted. The subnet
		// delete then fails with DependencyViolation and the stack is left
		// half torn down - which tptctl cannot pick up again afterwards,
		// because its next run asks the Threeport API that lived in the
		// cluster this destroy already removed.
		//
		// Releasing the orphans and destroying again is the whole repair.
		// Anything the stack already deleted is tolerated on the second pass.
		releasedCount, releaseErr := i.releaseOrphanedNetworkInterfaces()
		if releaseErr != nil {
			return fmt.Errorf(
				"failed to destroy EKS stack: %w (and failed to release orphaned network interfaces: %v)",
				err, releaseErr,
			)
		}
		if releasedCount == 0 {
			return fmt.Errorf("failed to destroy EKS stack: %w", err)
		}

		i.logInfo(
			"released network interfaces left by the cluster and retrying the destroy",
			"count", releasedCount,
		)
		if retryErr := i.DestroyStack(); retryErr != nil {
			return fmt.Errorf("failed to destroy EKS stack after releasing orphaned network interfaces: %w", retryErr)
		}
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

	return i.PulumiWorkspace.GetStackState()
}

// SetStackState restores Pulumi state from a JSON object (export or checkpoint
// format).
func (i *KubernetesRuntimeInfraEKS) SetStackState(state *datatypes.JSON) error {

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
	); err != nil && !IsAwsErrorCode(err, nse.ErrorCode()) {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

// RollBackResourceManagerRole undoes a failed CreateResourceManagerRole,
// unless the failure was a refusal to adopt.
//
// The teardown deletes by name and checks nothing. Running it after a refusal
// would delete the role the refusal was protecting - a worse outcome than the
// collision it refused over. Callers pass the error that sent them here rather
// than deciding for themselves, because there is no way to tell by reading the
// call site that the decision was needed at all.
func RollBackResourceManagerRole(instanceName string, awsConfig aws.Config, createErr error) error {
	if errors.Is(createErr, ErrUnownedIamResource) {
		return nil
	}

	return DeleteResourceManagerRole(instanceName, awsConfig)
}

// CreateResourceManagerRole creates the IAM role needed for resource
// management.
//
// The role and its policies are created if they are not there and adopted if
// they are, so an install that failed partway can be run again. Adoption is
// not unconditional: anything already at one of these derived names has to
// carry the tags this install would have given it, or the call refuses rather
// than take over a resource somebody else created. See createRoleIfAbsent and
// createPolicyIfAbsent.
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
	ctx := context.Background()
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
	role, err := createRoleIfAbsent(ctx, svc, &createResourceManagerRoleInput, tags, remediateControlPlaneNameCollision)
	if err != nil {
		return nil, err
	}

	if err := attachResourceManagerPolicies(
		ctx, svc, roleName, accountId, attachAssumeAnyRolePolicy, attachResourceManagerPolicy,
	); err != nil {
		return role, err
	}

	return role, nil
}

// attachResourceManagerPolicies attaches the policies the resource manager
// role is asked for.
//
// It is separate from CreateResourceManagerRole, which builds its own IAM
// client from an aws.Config and so cannot be exercised without an account.
// What has to be exercised is the wrapping: a refusal to adopt a policy
// reaches pkg/cli/v0 through here, and that is where it decides whether
// deleting the role by name is safe. A %v in place of a %w on this path
// compiles, reads the same, and turns that decision off.
func attachResourceManagerPolicies(
	ctx context.Context,
	svc iamRoleClient,
	roleName string,
	accountId string,
	attachAssumeAnyRolePolicy bool,
	attachResourceManagerPolicy bool,
) error {
	// attach assume any role policy
	if attachAssumeAnyRolePolicy {
		if err := AttachPolicy(ctx, AssumeAnyRolePolicyDocument, roleName, "assume-any-role", accountId, svc); err != nil {
			return fmt.Errorf("failed to attach policy to role %s: %w", roleName, err)
		}
	}

	// attach resource manager policy if requested
	if attachResourceManagerPolicy {
		if err := AttachPolicy(ctx, ResourceManagerPolicyDocument, roleName, "resource-manager", accountId, svc); err != nil {
			return fmt.Errorf("failed to attach policy to role %s: %w", roleName, err)
		}
	}

	return nil
}

// iamRoleClient is the part of the IAM API this file uses.
//
// It is an interface so a retry can be exercised without an AWS account: the
// behaviour being added here only shows up on the second attempt, which is
// the attempt nobody runs by hand.
type iamRoleClient interface {
	iamPolicyAttacher
	iamPolicyVersionClient
	CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreatePolicy(context.Context, *iam.CreatePolicyInput, ...func(*iam.Options)) (*iam.CreatePolicyOutput, error)
	UpdateAssumeRolePolicy(context.Context, *iam.UpdateAssumeRolePolicyInput, ...func(*iam.Options)) (*iam.UpdateAssumeRolePolicyOutput, error)
}

// createRoleIfAbsent creates the role, or adopts the one already there.
//
// The role name is derived from the control plane's, so it is the same on
// every attempt. An install that failed after creating it - at the node
// group, or while installing the control plane itself - could not be retried:
// the second run asked for a role that existed and AWS refused, permanently,
// until somebody deleted it by hand in a console.
//
// An existing role is only adopted if it carries the tags this install would
// have given it. A name is not proof of ownership, and taking over somebody
// else's role would be worse than refusing.
//
// createIamRole in aws_bootstrap.go, which the EKS refactor got right on the
// path it introduced while this one was left as it was, is a wrapper around
// this. One copy of the decision means a later fix - a new retryable code, an
// eventual-consistency wait on GetRole - lands on both paths at once.
func createRoleIfAbsent(
	ctx context.Context,
	svc iamRoleClient,
	input *iam.CreateRoleInput,
	wantTags []types.Tag,
	remediation string,
) (*types.Role, error) {
	roleName := aws.ToString(input.RoleName)

	created, err := svc.CreateRole(ctx, input)
	if err == nil {
		return created.Role, nil
	}
	if !IsAwsErrorCode(err, errCodeEntityAlreadyExists) {
		return nil, fmt.Errorf("failed to create role %s: %w", roleName, err)
	}

	existing, err := svc.GetRole(ctx, &iam.GetRoleInput{RoleName: input.RoleName})
	if err != nil {
		return nil, fmt.Errorf("failed to get existing role %s: %w", roleName, err)
	}
	if err := refuseUnownedRole(existing.Role, wantTags, remediation); err != nil {
		return nil, err
	}
	if err := syncIamRoleTrustPolicy(
		ctx, svc, existing.Role, aws.ToString(input.AssumeRolePolicyDocument),
	); err != nil {
		return nil, err
	}

	return existing.Role, nil
}

// syncIamRoleTrustPolicy brings an adopted role's trust policy in line with
// the one this attempt asked for.
//
// Who may assume the role is not fixed the way its name is. tptctl config
// aws-cloud-account puts a freshly generated external ID in it, and an attempt
// that failed after creating the role - while attaching its policies, say -
// deletes the AwsProvider record and leaves the role. The retry generates a
// new external ID, adopts the role, and without this the role still demands
// the one from the attempt that failed: the install reports success and every
// cross-account request is denied afterwards, with nothing pointing back here.
//
// The same holds for the bootstrap's roles whenever a release changes what
// their trust policy says.
func syncIamRoleTrustPolicy(
	ctx context.Context,
	svc iamRoleClient,
	role *types.Role,
	want string,
) error {
	roleName := aws.ToString(role.RoleName)
	if want == "" {
		return nil
	}

	// IAM always answers with one, so an empty document means there is nothing
	// to compare against rather than that the role trusts nobody; writing it
	// is the answer that cannot leave a stale one in place
	if stored := aws.ToString(role.AssumeRolePolicyDocument); stored != "" {
		match, err := iamDocumentsMatch(stored, want)
		if err != nil {
			return fmt.Errorf("failed to compare the trust policy of role %s: %w", roleName, err)
		}
		if match {
			return nil
		}
	}

	if _, err := svc.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       role.RoleName,
		PolicyDocument: aws.String(want),
	}); err != nil {
		return fmt.Errorf("failed to update the trust policy of role %s: %w", roleName, err)
	}

	return nil
}

// resourceManagerPolicyPath is the IAM path the resource manager's policies
// are created under. Empty is the unpathed form, which is what these have
// always used.
//
// It is a constant because the two branches below learn the ARN differently:
// a freshly created policy reports its own, while an adopted one has to have
// it assembled. Reading the path from two places is how adding one later would
// leave the adopt branch building the ARN of a policy that does not exist.
const resourceManagerPolicyPath = ""

// AttachPolicy attaches a given document to a role.
//
// The policy is created if it is not there and adopted if it is, for the same
// reason the role is: the name is derived and a retry asks for it again.
func AttachPolicy(ctx context.Context, document, roleName, policyName, accountId string, svc iamRoleClient) error {
	policyInputName := fmt.Sprintf("%s-%s", roleName, policyName)
	// create role policy
	rolePolicyInput := iam.CreatePolicyInput{
		PolicyName:     &policyInputName,
		Description:    &roleName,
		PolicyDocument: &document,
		Tags:           tpaws.IamTags(policyInputName, ThreeportProviderTags()),
	}
	if resourceManagerPolicyPath != "" {
		rolePolicyInput.Path = aws.String(resourceManagerPolicyPath)
	}

	policyArn, err := createPolicyIfAbsent(ctx, svc, &rolePolicyInput, accountId, remediateControlPlaneNameCollision)
	if err != nil {
		return err
	}

	return attachIamPolicy(ctx, svc, roleName, policyArn)
}

// createPolicyIfAbsent creates the policy, or adopts the one already there,
// and returns its ARN.
//
// Same shape and same reason as createRoleIfAbsent: the name is derived from
// the role's, so a second attempt asks for a policy the first one created and
// AWS refuses it.
//
// An existing policy is only adopted if it carries the tags this install would
// have given it. The role having passed its own check says nothing about this:
// a policy is a separate IAM resource at the account level, and one sitting at
// the name derived here could have been left by a partial teardown or by
// unrelated tooling. Attaching it would hand the role whatever permissions it
// grants.
//
// An adopted policy is also brought in line with the document asked for here,
// because it was created by whichever release first ran this.
func createPolicyIfAbsent(
	ctx context.Context,
	svc iamRoleClient,
	input *iam.CreatePolicyInput,
	accountId string,
	remediation string,
) (string, error) {
	policyName := aws.ToString(input.PolicyName)

	created, err := svc.CreatePolicy(ctx, input)
	if err == nil {
		if created.Policy == nil {
			return "", fmt.Errorf("IAM returned no policy after creating %s", policyName)
		}

		return aws.ToString(created.Policy.Arn), nil
	}
	if !IsAwsErrorCode(err, errCodeEntityAlreadyExists) {
		return "", fmt.Errorf("failed to create role policy %s: %w", policyName, err)
	}

	// the policy is already there from an earlier attempt; its ARN is derived
	// from the account, the path and the name, none of which have changed
	policyArn := tpaws.IamPolicyArn(accountId, resourceManagerPolicyPath, policyName)
	existing, err := svc.GetPolicy(ctx, &iam.GetPolicyInput{PolicyArn: aws.String(policyArn)})
	if err != nil {
		return "", fmt.Errorf("failed to check ownership of existing policy %s: %w", policyArn, err)
	}
	if existing.Policy == nil {
		return "", fmt.Errorf("IAM returned no policy to check ownership of at %s", policyArn)
	}
	if err := refuseUnownedIamResource("policy", policyName, existing.Policy.Tags, input.Tags, remediation); err != nil {
		return "", err
	}

	if err := syncIamPolicyDocument(ctx, svc, policyArn, aws.ToString(input.PolicyDocument)); err != nil {
		return "", err
	}

	return policyArn, nil
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
	return deleteRoleAndItsPolicies(context.Background(), iam.NewFromConfig(awsConfig), roleName)
}

// iamRoleDeleteClient is the part of the IAM API the teardown uses.
type iamRoleDeleteClient interface {
	iamPolicyVersionDeleter
	ListAttachedRolePolicies(context.Context, *iam.ListAttachedRolePoliciesInput, ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
	DetachRolePolicy(context.Context, *iam.DetachRolePolicyInput, ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	DeletePolicy(context.Context, *iam.DeletePolicyInput, ...func(*iam.Options)) (*iam.DeletePolicyOutput, error)
	DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
}

// deleteRoleAndItsPolicies detaches and deletes everything on a role, then the
// role.
//
// Every version but the one in force has to go before IAM will delete a
// policy. That used to be moot here - these policies were created once and
// never changed - but syncIamPolicyDocument adds a version whenever an
// adopted policy's document has moved on, so a control plane that was
// upgraded has policies this would otherwise refuse to delete. The policy is
// already detached by then, so the failure would leave it behind with nothing
// pointing at it.
func deleteRoleAndItsPolicies(ctx context.Context, svc iamRoleDeleteClient, roleName string) error {
	roles, err := svc.ListAttachedRolePolicies(
		ctx,
		&iam.ListAttachedRolePoliciesInput{
			RoleName: &roleName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list attached role policies: %s", err)
	}
	for _, role := range roles.AttachedPolicies {
		_, err := svc.DetachRolePolicy(
			ctx,
			&iam.DetachRolePolicyInput{
				PolicyArn: role.PolicyArn,
				RoleName:  &roleName,
			})
		if err != nil {
			return fmt.Errorf("failed to detach role policy: %s", err)
		}
		if err := deleteIamPolicyVersions(ctx, svc, aws.ToString(role.PolicyArn)); err != nil {
			return err
		}
		_, err = svc.DeletePolicy(
			ctx,
			&iam.DeletePolicyInput{
				PolicyArn: role.PolicyArn,
			})
		if err != nil {
			return fmt.Errorf("failed to delete policy: %s", err)
		}
	}

	_, err = svc.DeleteRole(
		ctx,
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
					"ec2:Describe*",
					"ec2:CreateVpc",
					"ec2:DeleteVpc",
					"ec2:ModifyVpcAttribute",
					"ec2:CreateSubnet",
					"ec2:DeleteSubnet",
					"ec2:ModifySubnetAttribute",
					"ec2:CreateRouteTable",
					"ec2:DeleteRouteTable",
					"ec2:AssociateRouteTable",
					"ec2:CreateRoute",
					"ec2:DeleteRoute",
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
					"ec2:CreateSecurityGroup",
					"ec2:DeleteSecurityGroup",
					"ec2:AuthorizeSecurityGroupIngress",
					"ec2:AuthorizeSecurityGroupEgress",
					"ec2:DeleteNetworkInterface"
				],
				"Resource": "*"
			},
			{
				"Sid": "EKSPermissions",
				"Effect": "Allow",
				"Action": [
					"eks:Describe*",
					"eks:List*",
					"eks:CreateCluster",
					"eks:DeleteCluster",
					"eks:UpdateClusterConfig",
					"eks:CreateNodegroup",
					"eks:DeleteNodegroup",
					"eks:UpdateNodegroupConfig",
					"eks:TagResource",
					"eks:UntagResource",
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
					"iam:Get*",
					"iam:List*",
					"iam:CreateOpenIDConnectProvider",
					"iam:DeleteOpenIDConnectProvider",
					"iam:UpdateOpenIDConnectProviderThumbprint",
					"iam:TagOpenIDConnectProvider",
					"iam:CreatePolicy",
					"iam:DeletePolicy",
					"iam:CreatePolicyVersion",
					"iam:DeletePolicyVersion",
					"iam:SetDefaultPolicyVersion",
					"iam:CreateRole",
					"iam:DeleteRole",
					"iam:UpdateRole",
					"iam:PutRolePolicy",
					"iam:DeleteRolePolicy",
					"iam:AttachRolePolicy",
					"iam:DetachRolePolicy",
					"iam:TagRole",
					"iam:UntagRole",
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
