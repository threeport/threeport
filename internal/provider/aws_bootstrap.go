package provider

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"net"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// Trust policy documents for the roles AWS services assume.
const (
	// eksClusterTrustPolicyDocument lets the EKS service assume the cluster
	// role.
	eksClusterTrustPolicyDocument = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Service": [
                    "eks.amazonaws.com"
                ]
            },
            "Action": "sts:AssumeRole"
        }
    ]
}`

	// eksNodeTrustPolicyDocument lets EC2 instances assume the worker node
	// role.
	eksNodeTrustPolicyDocument = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Service": [
                    "ec2.amazonaws.com"
                ]
            },
            "Action": "sts:AssumeRole"
        }
    ]
}`
)

// Permission policy documents for the add-ons that assume a role through the
// cluster's OIDC provider.  Carried over from aws-builder unchanged.
const (
	// eksDnsManagementPolicyDocument lets external-dns manage the Route53
	// records for the workloads exposed by the cluster.
	eksDnsManagementPolicyDocument = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "route53:ChangeResourceRecordSets"
            ],
            "Resource": [
                "arn:aws:route53:::hostedzone/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "route53:ListHostedZones",
                "route53:ListResourceRecordSets"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}`

	// eksDns01ChallengePolicyDocument lets cert-manager complete the DNS01
	// challenge for certificates it requests.
	eksDns01ChallengePolicyDocument = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "route53:ChangeResourceRecordSets"
            ],
            "Resource": [
                "arn:aws:route53:::hostedzone/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "route53:GetChange",
                "route53:ListHostedZones",
                "route53:ListResourceRecordSets",
                "route53:ListHostedZonesByName"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}`

	// eksSecretsManagerPolicyDocument lets external-secrets sync secrets from
	// AWS Secrets Manager into the cluster.
	eksSecretsManagerPolicyDocument = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Sid": "SecretsManagerPermissions",
            "Action": [
                "secretsmanager:BatchGetSecretValue",
                "secretsmanager:ListSecrets",
                "secretsmanager:CreateSecret",
                "secretsmanager:DeleteSecret",
                "secretsmanager:GetSecretValue"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}`

	// eksClusterAutoscalingPolicyDocumentFormat lets the cluster autoscaler
	// resize this cluster's node groups, and only this cluster's.
	eksClusterAutoscalingPolicyDocumentFormat = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "autoscaling:SetDesiredCapacity",
                "autoscaling:TerminateInstanceInAutoScalingGroup"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "aws:ResourceTag/k8s.io/cluster-autoscaler/%s": "owned"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "autoscaling:DescribeAutoScalingInstances",
                "autoscaling:DescribeAutoScalingGroups",
                "ec2:DescribeLaunchTemplateVersions",
                "autoscaling:DescribeTags",
                "autoscaling:DescribeLaunchConfigurations",
                "ec2:DescribeInstanceTypes"
            ],
            "Resource": "*"
        }
    ]
}`

	// eksIrsaTrustPolicyFormat lets a single Kubernetes service account assume
	// a role through the cluster's OIDC provider.  The arguments are the AWS
	// account ID, the OIDC provider without its scheme, and the service
	// account's namespace and name.
	eksIrsaTrustPolicyFormat = `{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::%[1]s:oidc-provider/%[2]s"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "%[2]s:sub": "system:serviceaccount:%[3]s:%[4]s",
                    "%[2]s:aud": "sts.amazonaws.com"
                }
            }
        }
    ]
}`
)

// eksIrsaRole is a role an add-on assumes through the cluster's OIDC provider,
// bound to the Kubernetes service account the add-on runs as.
type eksIrsaRole struct {
	// roleName is the name of the IAM role.
	roleName string

	// rolePath is the IAM path the role is created under, empty for the root
	// path.
	rolePath string

	// policyName names the customer managed policy created for this role.
	// Empty when managedPolicyArn is set instead.
	policyName string

	// policyDocument is the permission policy attached to the role.  Empty
	// when managedPolicyArn is set instead.
	policyDocument string

	// policyDescription describes the customer managed policy.
	policyDescription string

	// managedPolicyArn is an AWS managed policy attached to the role in place
	// of creating one.
	managedPolicyArn string

	// serviceAccountNamespace is the namespace of the Kubernetes service
	// account allowed to assume the role.
	serviceAccountNamespace string

	// serviceAccountName is the name of the Kubernetes service account allowed
	// to assume the role.
	serviceAccountName string
}

// irsaRoles returns the roles the cluster's add-ons assume.  Threeport enables
// all of them on every cluster it provisions.
func (i *KubernetesRuntimeInfraEKS) irsaRoles() []eksIrsaRole {
	clusterName := i.RuntimeInstanceName

	return []eksIrsaRole{
		{
			roleName: tpaws.EksDnsManagementRoleName(clusterName),
			// this role is the one aws-builder created under the cluster's IAM
			// path; the rest were left at the root path and stay there so the
			// ARNs stay as they were
			rolePath:                tpaws.EksIamPath(clusterName),
			policyName:              tpaws.EksDnsPolicyName(clusterName),
			policyDocument:          eksDnsManagementPolicyDocument,
			policyDescription:       "Allow Kubernetes service accounts to manage Route53 DNS records",
			serviceAccountNamespace: threeport.DNSManagerServiceAccountNamepace,
			serviceAccountName:      threeport.DNSManagerServiceAccountName,
		},
		{
			roleName:                tpaws.EksDns01ChallengeRoleName(clusterName),
			policyName:              tpaws.EksDns01ChallengePolicyName(clusterName),
			policyDocument:          eksDns01ChallengePolicyDocument,
			policyDescription:       "Allow Kubernetes service accounts to complete DNS01 challenges",
			serviceAccountNamespace: threeport.DNS01ChallengeServiceAccountNamepace,
			serviceAccountName:      threeport.DNS01ChallengeServiceAccountName,
		},
		{
			roleName:                tpaws.EksSecretsManagerRoleName(clusterName),
			policyName:              tpaws.EksSecretsManagerPolicyName(clusterName),
			policyDocument:          eksSecretsManagerPolicyDocument,
			policyDescription:       "Allow Kubernetes service accounts to manage secrets",
			serviceAccountNamespace: threeport.SecretsManagerServiceAccountNamespace,
			serviceAccountName:      threeport.SecretsManagerServiceAccountName,
		},
		{
			roleName:   tpaws.EksClusterAutoscalingRoleName(clusterName),
			policyName: tpaws.EksClusterAutoscalingPolicyName(clusterName),
			policyDocument: fmt.Sprintf(
				eksClusterAutoscalingPolicyDocumentFormat,
				clusterName,
			),
			policyDescription:       "Allow cluster autoscaler to manage node pool sizes",
			serviceAccountNamespace: threeport.ClusterAutoscalerNamespace,
			serviceAccountName:      threeport.ClusterAutoscalerServiceAccountName,
		},
		{
			roleName:                tpaws.EksStorageManagementRoleName(clusterName),
			managedPolicyArn:        tpaws.EksCsiDriverPolicyArn,
			serviceAccountNamespace: threeport.StorageManagerServiceAccountNamespace,
			serviceAccountName:      threeport.StorageManagerServiceAccountName,
		},
	}
}

// CreateClusterIam creates the IAM roles the EKS cluster and its worker nodes
// assume, and records their ARNs on the infra object for the Pulumi program to
// provision with.  It runs before the stack because EKS will not create a
// cluster without them.
func (i *KubernetesRuntimeInfraEKS) CreateClusterIam() error {
	if i.AwsConfig == nil {
		return fmt.Errorf("cannot create the cluster IAM roles without an AWS config")
	}

	// a deadline for the whole bootstrap, not per call: there is no context to
	// inherit, and without one a single stuck IAM call hangs the reconciler
	ctx, cancel := context.WithTimeout(context.Background(), iamBootstrapTimeout)
	defer cancel()

	iamClient := iam.NewFromConfig(*i.AwsConfig)

	clusterRoleName := tpaws.EksClusterRoleName(i.RuntimeInstanceName)
	clusterRole, err := i.createIamRole(ctx,
		iamClient,
		clusterRoleName,
		"",
		eksClusterTrustPolicyDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to create EKS cluster role: %w", err)
	}
	if err := attachIamPolicy(ctx, iamClient, clusterRoleName, tpaws.EksClusterPolicyArn); err != nil {
		return err
	}
	i.ClusterRoleArn = *clusterRole.Arn

	nodeRoleName := tpaws.EksNodeRoleName(i.RuntimeInstanceName)
	nodeRole, err := i.createIamRole(ctx,
		iamClient,
		nodeRoleName,
		"",
		eksNodeTrustPolicyDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to create EKS node role: %w", err)
	}
	nodePolicyArns := []string{
		tpaws.EksWorkerNodePolicyArn,
		tpaws.EksContainerRegistryPolicyArn,
		tpaws.EksCniPolicyArn,
	}
	for _, policyArn := range nodePolicyArns {
		if err := attachIamPolicy(ctx, iamClient, nodeRoleName, policyArn); err != nil {
			return err
		}
	}
	i.NodeRoleArn = *nodeRole.Arn

	return nil
}

// CreateIrsaIam registers the cluster's OIDC issuer as an identity provider in
// IAM, creates the roles the cluster's add-ons assume through it, and installs
// the EBS CSI addon bound to the storage management role.  It runs after the
// stack because the issuer URL only exists once the cluster does.
func (i *KubernetesRuntimeInfraEKS) CreateIrsaIam() error {
	if i.AwsConfig == nil {
		return fmt.Errorf("cannot create the IRSA roles without an AWS config")
	}

	// a deadline for the whole bootstrap, not per call: there is no context to
	// inherit, and without one a single stuck IAM call hangs the reconciler
	ctx, cancel := context.WithTimeout(context.Background(), iamBootstrapTimeout)
	defer cancel()

	oidcIssuerUrl, err := tpaws.EksOidcIssuerUrl(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return err
	}

	iamClient := iam.NewFromConfig(*i.AwsConfig)
	if _, err := i.createOidcProvider(ctx, iamClient, oidcIssuerUrl); err != nil {
		return fmt.Errorf("failed to create OIDC identity provider: %w", err)
	}

	// the trust policy names the provider by host and path, without a scheme
	oidcProvider := strings.TrimPrefix(oidcIssuerUrl, "https://")

	var storageManagementRoleArn string
	for _, irsaRole := range i.irsaRoles() {
		trustPolicyDocument := fmt.Sprintf(
			eksIrsaTrustPolicyFormat,
			i.AwsAccountID,
			oidcProvider,
			irsaRole.serviceAccountNamespace,
			irsaRole.serviceAccountName,
		)
		role, err := i.createIamRole(ctx,
			iamClient,
			irsaRole.roleName,
			irsaRole.rolePath,
			trustPolicyDocument,
		)
		if err != nil {
			return fmt.Errorf("failed to create IRSA role %s: %w", irsaRole.roleName, err)
		}

		policyArn := irsaRole.managedPolicyArn
		if policyArn == "" {
			policyArn, err = i.createIamPolicy(ctx, iamClient, irsaRole)
			if err != nil {
				return fmt.Errorf("failed to create policy %s: %w", irsaRole.policyName, err)
			}
		}
		if err := attachIamPolicy(ctx, iamClient, irsaRole.roleName, policyArn); err != nil {
			return err
		}

		if irsaRole.roleName == tpaws.EksStorageManagementRoleName(i.RuntimeInstanceName) {
			storageManagementRoleArn = *role.Arn
		}
	}

	if err := i.createEbsStorageAddon(ctx, storageManagementRoleArn); err != nil {
		return fmt.Errorf("failed to create EBS storage addon: %w", err)
	}

	return nil
}

// DeleteEksIam removes the IAM resources created for the cluster: the add-on
// roles and their customer managed policies, the OIDC identity provider, and
// the cluster and node roles.  The EBS CSI addon needs no cleanup of its own
// because deleting the cluster takes its addons with it.
func (i *KubernetesRuntimeInfraEKS) DeleteEksIam() error {
	if i.AwsConfig == nil {
		return fmt.Errorf("cannot delete the EKS IAM resources without an AWS config")
	}

	// a deadline for the whole bootstrap, not per call: there is no context to
	// inherit, and without one a single stuck IAM call hangs the reconciler
	ctx, cancel := context.WithTimeout(context.Background(), iamBootstrapTimeout)
	defer cancel()

	iamClient := iam.NewFromConfig(*i.AwsConfig)

	// collect failures rather than returning on the first one, so that one
	// resource already gone, or held by something else, does not strand every
	// resource after it
	var deleteErrors []error

	for _, irsaRole := range i.irsaRoles() {
		if err := deleteIamRole(ctx, iamClient, irsaRole.roleName, tpaws.IamTags(irsaRole.roleName, ThreeportProviderTags())); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
		if irsaRole.policyName == "" {
			continue
		}
		policyArn := i.iamPolicyArn(irsaRole)
		if err := deleteIamPolicy(ctx, iamClient, policyArn, tpaws.IamTags(irsaRole.policyName, ThreeportProviderTags())); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}

	if err := i.deleteOidcProvider(ctx, iamClient); err != nil {
		deleteErrors = append(deleteErrors, err)
	}

	for _, roleName := range []string{
		tpaws.EksNodeRoleName(i.RuntimeInstanceName),
		tpaws.EksClusterRoleName(i.RuntimeInstanceName),
	} {
		if err := deleteIamRole(ctx, iamClient, roleName, tpaws.IamTags(roleName, ThreeportProviderTags())); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}

	return errors.Join(deleteErrors...)
}

// createIamRole creates a role with the given trust policy, returning the
// existing role when one by that name is already there.
//
// The create-or-adopt decision itself lives in createRoleIfAbsent, which the
// EKS resource manager role goes through too; this adds the name check and the
// path and tags this bootstrap gives its roles.
func (i *KubernetesRuntimeInfraEKS) createIamRole(ctx context.Context, iamClient *iam.Client,
	roleName string,
	rolePath string,
	trustPolicyDocument string,
) (*types.Role, error) {
	if err := tpaws.CheckIamRoleName(roleName); err != nil {
		return nil, err
	}

	createRoleInput := iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustPolicyDocument),
		Tags:                     tpaws.IamTags(roleName, ThreeportProviderTags()),
	}
	if rolePath != "" {
		createRoleInput.Path = aws.String(rolePath)
	}

	return createRoleIfAbsent(ctx, iamClient, &createRoleInput, createRoleInput.Tags, remediateRuntimeNameCollision)
}

// oidcIssuerDialTimeout bounds the TLS dial to the cluster's OIDC issuer.
const oidcIssuerDialTimeout = 10 * time.Second

// iamBootstrapTimeout bounds a whole IAM bootstrap or teardown.
//
// None of CreateInfra, Delete, DeployInfra or DestroyInfra takes a context -
// that is the shared InfraProvider interface GCP and OCI implement too - so
// there is no external cancellation to propagate. Deriving a deadline at each
// entry point is the part that can be done without changing that interface,
// and it keeps a stuck AWS call from hanging the reconciler forever: DeleteEksIam
// alone makes fifteen to twenty sequential IAM calls.
const iamBootstrapTimeout = 10 * time.Minute

// refuseUnownedIamResource reports whether an IAM resource that already exists
// carries threeport's tags, and refuses it when it does not.
//
// The remediation is the caller's because the way out depends on where the
// name came from. A cluster role is named after a kubernetes runtime instance;
// the resource manager role is named after a control plane, or given outright
// on the command line. Telling someone to rename something that does not exist
// in their situation is worse than saying nothing.
//
// The names and paths this file builds are deterministic per cluster, so a
// resource left by an unrelated process - or by a partially torn down cluster
// of the same name - sits exactly where a new one would go. Reusing it attaches
// a stranger's permissions to this cluster's identities, and the teardown then
// deletes it out from under whoever does own it.
func refuseUnownedIamResource(kind, name string, have, want []types.Tag, remediation string) error {
	if iamTagsMatch(have, want) {
		return nil
	}

	return fmt.Errorf(
		"IAM %s %s already exists and was %w; %s",
		kind, name, ErrUnownedIamResource, remediation,
	)
}

// Remediation hints for a name collision. They differ because the names do:
// the bootstrap's roles and policies are derived from a kubernetes runtime
// instance, while the resource manager role is derived from a control plane
// or given outright with --role-name.
const (
	remediateRuntimeNameCollision      = "rename the kubernetes runtime instance or remove it"
	remediateControlPlaneNameCollision = "rename the control plane, pass a different role name, or remove it"
)

// ErrUnownedIamResource marks the refusal to touch an IAM resource sitting at
// a name this install derives but carrying tags it did not write.
//
// It is a sentinel because the callers that roll back a failed create delete
// by name. Running that rollback after a refusal would delete the very
// resource the refusal was protecting, so those callers have to be able to
// tell this apart from every other failure.
var ErrUnownedIamResource = errors.New("not created by threeport")

// refuseUnownedRole rejects an existing role that threeport did not create.
//
// Role names are derived from the cluster name, so one can collide with a role
// that happens to be named the same.  Reusing it would attach this cluster's
// add-on policies to whatever principals that role already trusts - the
// secrets manager role grants account-wide Secrets Manager access - and the
// teardown would later delete somebody else's role.  The GCP bootstrap refuses
// an unowned service account for the same reason.
func refuseUnownedRole(role *types.Role, wantTags []types.Tag, remediation string) error {
	if role == nil {
		return errors.New("IAM returned no role to check ownership of")
	}
	if iamTagsMatch(role.Tags, wantTags) {
		return nil
	}

	return fmt.Errorf(
		"IAM role %s already exists and was %w; %s",
		aws.ToString(role.RoleName), ErrUnownedIamResource, remediation,
	)
}

// iamPolicyArn returns the ARN of the customer managed policy for an IRSA
// role.
func (i *KubernetesRuntimeInfraEKS) iamPolicyArn(irsaRole eksIrsaRole) string {
	return tpaws.IamPolicyArn(
		i.AwsAccountID,
		tpaws.EksIamPath(i.RuntimeInstanceName),
		irsaRole.policyName,
	)
}

// createIamPolicy creates the customer managed policy for an IRSA role,
// returning the ARN of the existing policy when one by that name is already
// there.
func (i *KubernetesRuntimeInfraEKS) createIamPolicy(ctx context.Context, iamClient *iam.Client,
	irsaRole eksIrsaRole,
) (string, error) {
	createPolicyOutput, err := iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(irsaRole.policyName),
		Path:           aws.String(tpaws.EksIamPath(i.RuntimeInstanceName)),
		Description:    aws.String(irsaRole.policyDescription),
		PolicyDocument: aws.String(irsaRole.policyDocument),
		Tags:           tpaws.IamTags(irsaRole.policyName, ThreeportProviderTags()),
	})
	if err == nil {
		return *createPolicyOutput.Policy.Arn, nil
	}
	if !IsAwsErrorCode(err, errCodeEntityAlreadyExists) {
		return "", err
	}

	// a policy already at this name is only safe to attach if threeport put it
	// there; otherwise this would hand the new IRSA role whatever permissions a
	// stranger's policy carries
	policyArn := i.iamPolicyArn(irsaRole)
	getPolicyOutput, err := iamClient.GetPolicy(
		ctx,
		&iam.GetPolicyInput{PolicyArn: aws.String(policyArn)},
	)
	if err != nil {
		return "", fmt.Errorf("failed to check ownership of existing policy %s: %w", policyArn, err)
	}
	if getPolicyOutput.Policy == nil {
		return "", fmt.Errorf("IAM returned no policy to check ownership of at %s", policyArn)
	}
	if err := refuseUnownedIamResource(
		"policy", irsaRole.policyName, getPolicyOutput.Policy.Tags,
		tpaws.IamTags(irsaRole.policyName, ThreeportProviderTags()), remediateRuntimeNameCollision,
	); err != nil {
		return "", err
	}

	// the policy predates this run, so its document is whatever the release
	// that created it asked for
	if err := syncIamPolicyDocument(ctx, iamClient, policyArn, irsaRole.policyDocument); err != nil {
		return "", err
	}

	return policyArn, nil
}

// iamPolicyMaxVersions is IAM's limit on versions of a customer managed
// policy. The sixth CreatePolicyVersion is refused, so room has to be made
// before adding one.
const iamPolicyMaxVersions = 5

// iamPolicyVersionClient is the part of the IAM API syncIamPolicyDocument
// uses. It is an interface so the adopt path can be exercised without an AWS
// account; that path only runs on a second install, which nobody runs by hand.
type iamPolicyVersionClient interface {
	GetPolicy(context.Context, *iam.GetPolicyInput, ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
	GetPolicyVersion(context.Context, *iam.GetPolicyVersionInput, ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error)
	ListPolicyVersions(context.Context, *iam.ListPolicyVersionsInput, ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error)
	CreatePolicyVersion(context.Context, *iam.CreatePolicyVersionInput, ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error)
	DeletePolicyVersion(context.Context, *iam.DeletePolicyVersionInput, ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error)
}

// syncIamPolicyDocument brings an adopted policy's document in line with the
// one threeport would have created it with.
//
// A policy at a derived name is only ever created once. Every later run finds
// it there and adopts it, so without this a change to a policy document would
// never reach an account installed before the change: the install would report
// success and the role would quietly be missing the permission, surfacing much
// later as an AccessDenied with nothing pointing back here.
//
// The comparison is on the parsed JSON rather than the text. IAM reformats
// what it stores, so comparing bytes would find a difference on every run and
// write a new version until the five-version limit stopped it.
func syncIamPolicyDocument(
	ctx context.Context,
	iamClient iamPolicyVersionClient,
	policyArn string,
	document string,
) error {
	policy, err := iamClient.GetPolicy(ctx, &iam.GetPolicyInput{PolicyArn: aws.String(policyArn)})
	if err != nil {
		return fmt.Errorf("failed to read policy %s to compare its document: %w", policyArn, err)
	}
	if policy.Policy == nil || policy.Policy.DefaultVersionId == nil {
		return fmt.Errorf("IAM reported no version in force for policy %s", policyArn)
	}
	versionInForce := aws.ToString(policy.Policy.DefaultVersionId)

	current, err := iamClient.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
		PolicyArn: aws.String(policyArn),
		VersionId: aws.String(versionInForce),
	})
	if err != nil {
		return fmt.Errorf("failed to read version %s of policy %s: %w", versionInForce, policyArn, err)
	}
	if current.PolicyVersion == nil || current.PolicyVersion.Document == nil {
		return fmt.Errorf("IAM returned no document for version %s of policy %s", versionInForce, policyArn)
	}

	match, err := iamDocumentsMatch(aws.ToString(current.PolicyVersion.Document), document)
	if err != nil {
		return fmt.Errorf("failed to compare the document of policy %s: %w", policyArn, err)
	}
	if match {
		return nil
	}

	if err := pruneIamPolicyVersions(ctx, iamClient, policyArn, versionInForce); err != nil {
		return err
	}

	if _, err := iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(policyArn),
		PolicyDocument: aws.String(document),
		SetAsDefault:   true,
	}); err != nil {
		return fmt.Errorf("failed to update the document of policy %s: %w", policyArn, err)
	}

	return nil
}

// iamDocumentsMatch reports whether a document IAM is storing says the same
// thing as one this package would create. It serves both policy documents and
// the trust policy a role carries.
//
// What IAM returns is percent-encoded, and reformatted from whatever was sent
// to it, so the two have to be decoded and parsed before they can be compared.
// PathUnescape rather than QueryUnescape: a literal '+' appears in IAM ARNs
// and conditions, and QueryUnescape would read it as a space.
func iamDocumentsMatch(stored, want string) (bool, error) {
	decoded, err := url.PathUnescape(stored)
	if err != nil {
		return false, fmt.Errorf("failed to decode the stored document: %w", err)
	}

	var storedDocument, wantDocument any
	if err := json.Unmarshal([]byte(decoded), &storedDocument); err != nil {
		return false, fmt.Errorf("failed to parse the stored document: %w", err)
	}
	if err := json.Unmarshal([]byte(want), &wantDocument); err != nil {
		return false, fmt.Errorf("failed to parse the document to compare against: %w", err)
	}

	return reflect.DeepEqual(storedDocument, wantDocument), nil
}

// pruneIamPolicyVersions removes the oldest version that is not in force, so
// a new one can be added to a policy already at the limit.
func pruneIamPolicyVersions(
	ctx context.Context,
	iamClient iamPolicyVersionClient,
	policyArn string,
	versionInForce string,
) error {
	versions, err := iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return fmt.Errorf("failed to list the versions of policy %s: %w", policyArn, err)
	}
	if len(versions.Versions) < iamPolicyMaxVersions {
		return nil
	}

	var oldest *types.PolicyVersion
	for i := range versions.Versions {
		version := &versions.Versions[i]
		if version.IsDefaultVersion || aws.ToString(version.VersionId) == versionInForce {
			continue
		}
		if oldest == nil || (version.CreateDate != nil && oldest.CreateDate != nil &&
			version.CreateDate.Before(*oldest.CreateDate)) {
			oldest = version
		}
	}
	if oldest == nil {
		return fmt.Errorf(
			"policy %s holds its %d version limit with nothing but the version in force; "+
				"remove an old version of it by hand",
			policyArn, iamPolicyMaxVersions,
		)
	}

	if _, err := iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
		PolicyArn: aws.String(policyArn),
		VersionId: oldest.VersionId,
	}); err != nil {
		return fmt.Errorf(
			"failed to remove version %s of policy %s to make room for a new one: %w",
			aws.ToString(oldest.VersionId), policyArn, err,
		)
	}

	return nil
}

// createOidcProvider registers the cluster's OIDC issuer as an identity
// provider in IAM, which is what lets a Kubernetes service account assume an
// IAM role.
func (i *KubernetesRuntimeInfraEKS) createOidcProvider(ctx context.Context, iamClient *iam.Client,
	oidcIssuerUrl string,
) (string, error) {
	thumbprint, err := oidcIssuerThumbprint(ctx, oidcIssuerUrl)
	if err != nil {
		return "", err
	}

	createProviderOutput, err := iamClient.CreateOpenIDConnectProvider(
		ctx,
		&iam.CreateOpenIDConnectProviderInput{
			Url:            aws.String(oidcIssuerUrl),
			ClientIDList:   []string{"sts.amazonaws.com"},
			ThumbprintList: []string{thumbprint},
			Tags:           tpaws.IamTags(i.RuntimeInstanceName, ThreeportProviderTags()),
		},
	)
	if err == nil {
		return *createProviderOutput.OpenIDConnectProviderArn, nil
	}
	if !IsAwsErrorCode(err, errCodeEntityAlreadyExists) {
		return "", err
	}

	return i.findOidcProviderArn(ctx, iamClient, oidcIssuerUrl)
}

// findOidcProviderArn returns the ARN of the identity provider registered for
// an issuer URL.
func (i *KubernetesRuntimeInfraEKS) findOidcProviderArn(ctx context.Context, iamClient *iam.Client,
	oidcIssuerUrl string,
) (string, error) {
	// the provider's ARN ends in the issuer URL without its scheme, so the
	// provider can be recognized without describing each one
	oidcProvider := strings.TrimPrefix(oidcIssuerUrl, "https://")

	listProvidersOutput, err := iamClient.ListOpenIDConnectProviders(
		ctx,
		&iam.ListOpenIDConnectProvidersInput{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to list OIDC identity providers: %w", err)
	}
	for _, provider := range listProvidersOutput.OpenIDConnectProviderList {
		if provider.Arn != nil && strings.HasSuffix(*provider.Arn, oidcProvider) {
			return *provider.Arn, nil
		}
	}

	return "", fmt.Errorf("failed to find OIDC identity provider for issuer %s", oidcIssuerUrl)
}

// findOidcProviderArnsByTag returns the ARNs of every identity provider tagged
// as belonging to this cluster.
//
// All of them, not the first: OIDC providers have no names, only tags, so a
// cluster recreated after an incomplete teardown can leave two carrying the
// same tags. Returning one would delete one and orphan the other - which the
// leak detection in pkg/aws/v0/inventory.go exists to catch, and which it
// would then report on every run with nothing able to clear it.
func (i *KubernetesRuntimeInfraEKS) findOidcProviderArnsByTag(ctx context.Context, iamClient *iam.Client) ([]string, error) {
	listProvidersOutput, err := iamClient.ListOpenIDConnectProviders(
		ctx,
		&iam.ListOpenIDConnectProvidersInput{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list OIDC identity providers: %w", err)
	}

	var arns []string
	for _, provider := range listProvidersOutput.OpenIDConnectProviderList {
		if provider.Arn == nil {
			continue
		}
		getProviderOutput, err := iamClient.GetOpenIDConnectProvider(
			ctx,
			&iam.GetOpenIDConnectProviderInput{OpenIDConnectProviderArn: provider.Arn},
		)
		if err != nil {
			if isIamNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get OIDC identity provider %s: %w", *provider.Arn, err)
		}
		if iamTagsMatch(getProviderOutput.Tags, tpaws.IamTags(i.RuntimeInstanceName, ThreeportProviderTags())) {
			arns = append(arns, *provider.Arn)
		}
	}

	return arns, nil
}

// iamTagsMatch reports whether every tag in want is present in have with the
// same value.
func iamTagsMatch(have, want []types.Tag) bool {
	haveTags := make(map[string]string, len(have))
	for _, tag := range have {
		if tag.Key == nil || tag.Value == nil {
			continue
		}
		haveTags[*tag.Key] = *tag.Value
	}

	for _, tag := range want {
		if tag.Key == nil || tag.Value == nil {
			continue
		}
		if haveTags[*tag.Key] != *tag.Value {
			return false
		}
	}

	return true
}

// deleteOidcProvider removes the cluster's OIDC identity provider from IAM.
// The provider is found by its tags rather than by the cluster's issuer URL,
// because by the time IAM is cleaned up the cluster that would report that URL
// is usually already destroyed.
func (i *KubernetesRuntimeInfraEKS) deleteOidcProvider(ctx context.Context, iamClient *iam.Client) error {
	oidcProviderArns, err := i.findOidcProviderArnsByTag(ctx, iamClient)
	if err != nil {
		return err
	}

	// every match, so an earlier incomplete teardown cannot leave one behind
	var deleteErrors []error
	for _, oidcProviderArn := range oidcProviderArns {
		_, err := iamClient.DeleteOpenIDConnectProvider(
			ctx,
			&iam.DeleteOpenIDConnectProviderInput{OpenIDConnectProviderArn: aws.String(oidcProviderArn)},
		)
		if err != nil && !isIamNotFound(err) {
			deleteErrors = append(deleteErrors, fmt.Errorf(
				"failed to delete OIDC identity provider %s: %w", oidcProviderArn, err,
			))
		}
	}

	return errors.Join(deleteErrors...)
}

// createEbsStorageAddon installs the EBS CSI driver on the cluster, running as
// the storage management role.
func (i *KubernetesRuntimeInfraEKS) createEbsStorageAddon(ctx context.Context, storageManagementRoleArn string) error {
	if i.AwsConfig == nil {
		return errors.New("cannot create the EBS storage addon without an AWS config")
	}
	if storageManagementRoleArn == "" {
		return errors.New("no storage management role to bind the EBS storage addon to")
	}

	eksClient := awseks.NewFromConfig(*i.AwsConfig)
	_, err := eksClient.CreateAddon(ctx, &awseks.CreateAddonInput{
		AddonName:             aws.String(tpaws.EksEbsStorageAddonName),
		ClusterName:           aws.String(i.RuntimeInstanceName),
		ServiceAccountRoleArn: aws.String(storageManagementRoleArn),
		Tags:                  ThreeportProviderTags(),
	})
	if err != nil && !IsAwsErrorCode(err, "ResourceInUseException") {
		return err
	}

	return nil
}

// oidcIssuerThumbprint returns the SHA1 thumbprint of the root certificate the
// OIDC issuer presents, which IAM requires to register the issuer as an
// identity provider.
func oidcIssuerThumbprint(ctx context.Context, oidcIssuerUrl string) (string, error) {
	issuerUrl, err := url.Parse(oidcIssuerUrl)
	if err != nil {
		return "", fmt.Errorf("failed to parse OIDC issuer URL: %w", err)
	}

	// bounded: an unreachable issuer - a security group or route that black
	// holes egress - otherwise hangs for the OS's full TCP retransmit timeout,
	// stalling the whole cluster create with nothing able to cancel it
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: oidcIssuerDialTimeout},
		"tcp",
		fmt.Sprintf("%s:443", issuerUrl.Hostname()),
		&tls.Config{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to connect to OIDC issuer %s: %w", issuerUrl.Hostname(), err)
	}
	defer conn.Close()

	certificates := conn.ConnectionState().PeerCertificates
	if len(certificates) == 0 {
		return "", fmt.Errorf("OIDC issuer %s presented no certificates", issuerUrl.Hostname())
	}
	// the last certificate in the chain is the root, which is the one IAM
	// pins
	thumbprint := sha1.Sum(certificates[len(certificates)-1].Raw)

	return fmt.Sprintf("%x", thumbprint), nil
}

// iamPolicyAttacher is the part of the IAM API attachIamPolicy uses.
type iamPolicyAttacher interface {
	iam.ListAttachedRolePoliciesAPIClient
	AttachRolePolicy(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
}

// attachIamPolicy attaches a policy to a role unless it is attached already.
// AWS does not document AttachRolePolicy as idempotent, and its own examples
// check before attaching, so a role that already carries the policy is left
// alone rather than relying on the call being harmless.
func attachIamPolicy(ctx context.Context, iamClient iamPolicyAttacher, roleName, policyArn string) error {
	attachedPolicies, err := listAttachedPolicies(ctx, iamClient, roleName)
	if err != nil {
		return err
	}
	for _, attachedPolicy := range attachedPolicies {
		if attachedPolicy.PolicyArn != nil && *attachedPolicy.PolicyArn == policyArn {
			return nil
		}
	}

	_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return fmt.Errorf("failed to attach policy %s to role %s: %w", policyArn, roleName, err)
	}

	return nil
}

// listAttachedPolicies returns every managed policy attached to a role.
func listAttachedPolicies(
	ctx context.Context,
	iamClient iam.ListAttachedRolePoliciesAPIClient,
	roleName string,
) ([]types.AttachedPolicy, error) {
	var attachedPolicies []types.AttachedPolicy

	paginator := iam.NewListAttachedRolePoliciesPaginator(
		iamClient,
		&iam.ListAttachedRolePoliciesInput{RoleName: aws.String(roleName)},
	)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list policies attached to role %s: %w", roleName, err)
		}
		attachedPolicies = append(attachedPolicies, page.AttachedPolicies...)
	}

	return attachedPolicies, nil
}

// deleteIamRole detaches every policy attached to a role and deletes it.  A
// role that is already gone is not an error, and a role threeport did not
// create is left alone rather than deleted out from under its owner.
func deleteIamRole(ctx context.Context, iamClient *iam.Client, roleName string, wantTags []types.Tag) error {
	getRoleOutput, err := iamClient.GetRole(
		ctx,
		&iam.GetRoleInput{RoleName: aws.String(roleName)},
	)
	if err != nil {
		if isIamNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get role %s before deleting it: %w", roleName, err)
	}
	if err := refuseUnownedRole(getRoleOutput.Role, wantTags, remediateRuntimeNameCollision); err != nil {
		return err
	}

	return deleteOwnedIamRole(ctx, iamClient, roleName)
}

// deleteOwnedIamRole removes a role once its ownership has been established.
func deleteOwnedIamRole(ctx context.Context, iamClient *iam.Client, roleName string) error {
	attachedPolicies, err := listAttachedPolicies(ctx, iamClient, roleName)
	if err != nil {
		if isIamNotFound(err) {
			return nil
		}
		return err
	}

	// a role cannot be deleted while a policy is attached to it, and the
	// attached set is read from AWS rather than from a record of what was
	// attached, so a policy added out of band does not block the delete
	for _, attachedPolicy := range attachedPolicies {
		_, err := iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: attachedPolicy.PolicyArn,
		})
		if err != nil && !isIamNotFound(err) {
			return fmt.Errorf(
				"failed to detach policy %s from role %s: %w",
				*attachedPolicy.PolicyArn, roleName, err,
			)
		}
	}

	_, err = iamClient.DeleteRole(
		ctx,
		&iam.DeleteRoleInput{RoleName: aws.String(roleName)},
	)
	if err != nil && !isIamNotFound(err) {
		return fmt.Errorf("failed to delete role %s: %w", roleName, err)
	}

	return nil
}

// deleteIamPolicy deletes a customer managed policy along with any versions
// it accumulated.  A policy that is already gone is not an error.
func deleteIamPolicy(ctx context.Context, iamClient *iam.Client, policyArn string, wantTags []types.Tag) error {
	// the ARN is deterministic per cluster, so a policy sitting at it may not
	// be the one this cluster created - deleting that would take a policy away
	// from whoever does own it
	getPolicyOutput, err := iamClient.GetPolicy(
		ctx,
		&iam.GetPolicyInput{PolicyArn: aws.String(policyArn)},
	)
	if err != nil {
		if isIamNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to check ownership of policy %s: %w", policyArn, err)
	}
	if getPolicyOutput.Policy == nil {
		return fmt.Errorf("IAM returned no policy to check ownership of at %s", policyArn)
	}
	if err := refuseUnownedIamResource(
		"policy", policyArn, getPolicyOutput.Policy.Tags, wantTags, remediateRuntimeNameCollision,
	); err != nil {
		return err
	}

	if err := deleteIamPolicyVersions(ctx, iamClient, policyArn); err != nil {
		return err
	}

	_, err = iamClient.DeletePolicy(
		ctx,
		&iam.DeletePolicyInput{PolicyArn: aws.String(policyArn)},
	)
	if err != nil && !isIamNotFound(err) {
		return fmt.Errorf("failed to delete policy %s: %w", policyArn, err)
	}

	return nil
}

// iamPolicyVersionDeleter is the part of the IAM API deleteIamPolicyVersions
// uses.
type iamPolicyVersionDeleter interface {
	ListPolicyVersions(context.Context, *iam.ListPolicyVersionsInput, ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error)
	DeletePolicyVersion(context.Context, *iam.DeletePolicyVersionInput, ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error)
}

// deleteIamPolicyVersions removes every version of a policy but the one in
// force, which is what DeletePolicy needs before it will accept the policy.
//
// IAM refuses to delete a policy that still has other versions, so a policy
// whose document was ever updated - which syncIamPolicyDocument does on an
// adopted policy - cannot be torn down without this. The teardown detaches it
// first, so a failure here leaves a detached policy behind that no later run
// looks at.
func deleteIamPolicyVersions(ctx context.Context, iamClient iamPolicyVersionDeleter, policyArn string) error {
	listVersionsOutput, err := iamClient.ListPolicyVersions(
		ctx,
		&iam.ListPolicyVersionsInput{PolicyArn: aws.String(policyArn)},
	)
	if err != nil {
		if isIamNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to list versions of policy %s: %w", policyArn, err)
	}

	// the default version is deleted with the policy itself, but the others
	// have to go first
	for _, policyVersion := range listVersionsOutput.Versions {
		if policyVersion.IsDefaultVersion {
			continue
		}
		_, err := iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
			PolicyArn: aws.String(policyArn),
			VersionId: policyVersion.VersionId,
		})
		if err != nil && !isIamNotFound(err) {
			return fmt.Errorf(
				"failed to delete version %s of policy %s: %w",
				aws.ToString(policyVersion.VersionId), policyArn, err,
			)
		}
	}

	return nil
}

// errCodeEntityAlreadyExists is what IAM answers when the name is taken.
//
// Every create in this package is retryable by name, so each one tests for
// this code. A typo in a literal would compile and simply never match, which
// would quietly return that one path to failing on retry - the exact bug this
// package has had. One constant makes a mistake break all of them at once.
const errCodeEntityAlreadyExists = "EntityAlreadyExists"

// IsAwsErrorCode reports whether an error is the named AWS API error.
func IsAwsErrorCode(err error, code string) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == code
	}

	return false
}

// isIamNotFound reports whether an error says the resource does not exist.
func isIamNotFound(err error) bool {
	var noSuchEntityErr *types.NoSuchEntityException

	return errors.As(err, &noSuchEntityErr)
}

// releaseOrphanedNetworkInterfaces deletes detached network interfaces in the
// VPC this stack created, and reports how many it removed.
//
// The VPC CNI allocates secondary interfaces on worker nodes to hand pod
// addresses out of the subnet. They belong to the cluster rather than to the
// Pulumi stack, so a destroy does not account for them, and AWS does not
// always reap them before the subnets they sit in are deleted - which fails
// with DependencyViolation and leaves the stack half torn down.
//
// The VPC is read out of the stack's own state rather than looked up by tag.
// A tag is a label anyone can reuse, and this deletes things: searching the
// account by name would reach another workload's interfaces, where a VPC
// recorded in this stack's state is one this stack created.
//
// Only detached interfaces are touched. One still in use belongs to something
// that is meant to be there, and removing it would break whatever that is
// rather than unblock anything.
func (i *KubernetesRuntimeInfraEKS) releaseOrphanedNetworkInterfaces() (int, error) {
	if i.AwsConfig == nil {
		return 0, errors.New("cannot release network interfaces without an AWS config")
	}

	vpcId, err := i.vpcIdFromStackState()
	if err != nil {
		return 0, err
	}
	if vpcId == "" {
		// the VPC is gone, or the state records none, so nothing in it blocks
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), iamBootstrapTimeout)
	defer cancel()

	ec2Client := ec2.NewFromConfig(*i.AwsConfig)
	describeOutput, err := ec2Client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcId}},
			{Name: aws.String("status"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list network interfaces in VPC %s: %w", vpcId, err)
	}

	var released int
	var releaseErrors []error
	for _, networkInterface := range describeOutput.NetworkInterfaces {
		if networkInterface.NetworkInterfaceId == nil {
			continue
		}
		if _, err := ec2Client.DeleteNetworkInterface(ctx, &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: networkInterface.NetworkInterfaceId,
		}); err != nil {
			// one that went away on its own between the describe and here is
			// the outcome wanted, not a failure
			if IsAwsErrorCode(err, "InvalidNetworkInterfaceID.NotFound") {
				continue
			}
			releaseErrors = append(releaseErrors, fmt.Errorf(
				"failed to delete network interface %s: %w", *networkInterface.NetworkInterfaceId, err,
			))

			continue
		}
		released++
	}

	return released, errors.Join(releaseErrors...)
}

// eksVpcResourceType is the Pulumi type token of the VPC the program creates.
const eksVpcResourceType = "aws:ec2/vpc:Vpc"

// vpcIdFromStackState returns the id of the VPC this stack created, or an empty
// string when the state records none.
func (i *KubernetesRuntimeInfraEKS) vpcIdFromStackState() (string, error) {
	state, err := i.ReadStateFile()
	if err != nil {
		return "", fmt.Errorf("failed to read stack state to find the cluster's VPC: %w", err)
	}
	if state == nil {
		return "", nil
	}

	var checkpoint struct {
		Checkpoint struct {
			Latest struct {
				Resources []struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"resources"`
			} `json:"latest"`
		} `json:"checkpoint"`
	}
	if err := json.Unmarshal(*state, &checkpoint); err != nil {
		return "", fmt.Errorf("failed to parse stack state to find the cluster's VPC: %w", err)
	}

	for _, resource := range checkpoint.Checkpoint.Latest.Resources {
		if resource.Type == eksVpcResourceType && resource.ID != "" {
			return resource.ID, nil
		}
	}

	return "", nil
}
