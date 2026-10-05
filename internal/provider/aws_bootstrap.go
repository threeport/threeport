package provider

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"

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

// iamTags returns the tags applied to the IAM resources created for the
// cluster.
func (i *KubernetesRuntimeInfraEKS) iamTags(name string) []types.Tag {
	tags := []types.Tag{{Key: aws.String("Name"), Value: aws.String(name)}}
	for key, value := range ThreeportProviderTags() {
		tags = append(tags, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}

	return tags
}

// CreateClusterIam creates the IAM roles the EKS cluster and its worker nodes
// assume, and records their ARNs on the infra object for the Pulumi program to
// provision with.  It runs before the stack because EKS will not create a
// cluster without them.
func (i *KubernetesRuntimeInfraEKS) CreateClusterIam() error {
	iamClient := iam.NewFromConfig(*i.AwsConfig)

	clusterRoleName := tpaws.EksClusterRoleName(i.RuntimeInstanceName)
	clusterRole, err := i.createIamRole(
		iamClient,
		clusterRoleName,
		"",
		eksClusterTrustPolicyDocument,
	)
	if err != nil {
		return fmt.Errorf("failed to create EKS cluster role: %w", err)
	}
	if err := attachIamPolicy(iamClient, clusterRoleName, tpaws.EksClusterPolicyArn); err != nil {
		return err
	}
	i.ClusterRoleArn = *clusterRole.Arn

	nodeRoleName := tpaws.EksNodeRoleName(i.RuntimeInstanceName)
	nodeRole, err := i.createIamRole(
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
		if err := attachIamPolicy(iamClient, nodeRoleName, policyArn); err != nil {
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
	oidcIssuerUrl, err := tpaws.EksOidcIssuerUrl(i.AwsConfig, i.RuntimeInstanceName)
	if err != nil {
		return err
	}

	iamClient := iam.NewFromConfig(*i.AwsConfig)
	if _, err := i.createOidcProvider(iamClient, oidcIssuerUrl); err != nil {
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
		role, err := i.createIamRole(
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
			policyArn, err = i.createIamPolicy(iamClient, irsaRole)
			if err != nil {
				return fmt.Errorf("failed to create policy %s: %w", irsaRole.policyName, err)
			}
		}
		if err := attachIamPolicy(iamClient, irsaRole.roleName, policyArn); err != nil {
			return err
		}

		if irsaRole.roleName == tpaws.EksStorageManagementRoleName(i.RuntimeInstanceName) {
			storageManagementRoleArn = *role.Arn
		}
	}

	if err := i.createEbsStorageAddon(storageManagementRoleArn); err != nil {
		return fmt.Errorf("failed to create EBS storage addon: %w", err)
	}

	return nil
}

// DeleteEksIam removes the IAM resources created for the cluster: the add-on
// roles and their customer managed policies, the OIDC identity provider, and
// the cluster and node roles.  The EBS CSI addon needs no cleanup of its own
// because deleting the cluster takes its addons with it.
func (i *KubernetesRuntimeInfraEKS) DeleteEksIam() error {
	iamClient := iam.NewFromConfig(*i.AwsConfig)

	// collect failures rather than returning on the first one, so that one
	// resource already gone, or held by something else, does not strand every
	// resource after it
	var deleteErrors []error

	for _, irsaRole := range i.irsaRoles() {
		if err := deleteIamRole(iamClient, irsaRole.roleName, i.iamTags(irsaRole.roleName)); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
		if irsaRole.policyName == "" {
			continue
		}
		policyArn := i.iamPolicyArn(irsaRole)
		if err := deleteIamPolicy(iamClient, policyArn); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}

	if err := i.deleteOidcProvider(iamClient); err != nil {
		deleteErrors = append(deleteErrors, err)
	}

	for _, roleName := range []string{
		tpaws.EksNodeRoleName(i.RuntimeInstanceName),
		tpaws.EksClusterRoleName(i.RuntimeInstanceName),
	} {
		if err := deleteIamRole(iamClient, roleName, i.iamTags(roleName)); err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}

	return errors.Join(deleteErrors...)
}

// createIamRole creates a role with the given trust policy, returning the
// existing role when one by that name is already there.
func (i *KubernetesRuntimeInfraEKS) createIamRole(
	iamClient *iam.Client,
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
		Tags:                     i.iamTags(roleName),
	}
	if rolePath != "" {
		createRoleInput.Path = aws.String(rolePath)
	}

	createRoleOutput, err := iamClient.CreateRole(context.Background(), &createRoleInput)
	if err == nil {
		return createRoleOutput.Role, nil
	}
	if !isAwsErrorCode(err, "EntityAlreadyExists") {
		return nil, fmt.Errorf("failed to create role %s: %w", roleName, err)
	}

	getRoleOutput, err := iamClient.GetRole(
		context.Background(),
		&iam.GetRoleInput{RoleName: aws.String(roleName)},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing role %s: %w", roleName, err)
	}
	if err := refuseUnownedRole(getRoleOutput.Role, i.iamTags(roleName)); err != nil {
		return nil, err
	}

	return getRoleOutput.Role, nil
}

// refuseUnownedRole rejects an existing role that threeport did not create.
//
// Role names are derived from the cluster name, so one can collide with a role
// that happens to be named the same.  Reusing it would attach this cluster's
// add-on policies to whatever principals that role already trusts - the
// secrets manager role grants account-wide Secrets Manager access - and the
// teardown would later delete somebody else's role.  The GCP bootstrap refuses
// an unowned service account for the same reason.
func refuseUnownedRole(role *types.Role, wantTags []types.Tag) error {
	if role == nil {
		return errors.New("IAM returned no role to check ownership of")
	}
	if iamTagsMatch(role.Tags, wantTags) {
		return nil
	}

	return fmt.Errorf(
		"IAM role %s already exists and was not created by threeport; "+
			"rename the kubernetes runtime instance or remove the role",
		aws.ToString(role.RoleName),
	)
}

// iamPolicyArn returns the ARN of the customer managed policy for an IRSA
// role.
func (i *KubernetesRuntimeInfraEKS) iamPolicyArn(irsaRole eksIrsaRole) string {
	return fmt.Sprintf(
		"arn:aws:iam::%s:policy%s%s",
		i.AwsAccountID,
		tpaws.EksIamPath(i.RuntimeInstanceName),
		irsaRole.policyName,
	)
}

// createIamPolicy creates the customer managed policy for an IRSA role,
// returning the ARN of the existing policy when one by that name is already
// there.
func (i *KubernetesRuntimeInfraEKS) createIamPolicy(
	iamClient *iam.Client,
	irsaRole eksIrsaRole,
) (string, error) {
	createPolicyOutput, err := iamClient.CreatePolicy(context.Background(), &iam.CreatePolicyInput{
		PolicyName:     aws.String(irsaRole.policyName),
		Path:           aws.String(tpaws.EksIamPath(i.RuntimeInstanceName)),
		Description:    aws.String(irsaRole.policyDescription),
		PolicyDocument: aws.String(irsaRole.policyDocument),
		Tags:           i.iamTags(irsaRole.policyName),
	})
	if err == nil {
		return *createPolicyOutput.Policy.Arn, nil
	}
	if !isAwsErrorCode(err, "EntityAlreadyExists") {
		return "", err
	}

	return i.iamPolicyArn(irsaRole), nil
}

// createOidcProvider registers the cluster's OIDC issuer as an identity
// provider in IAM, which is what lets a Kubernetes service account assume an
// IAM role.
func (i *KubernetesRuntimeInfraEKS) createOidcProvider(
	iamClient *iam.Client,
	oidcIssuerUrl string,
) (string, error) {
	thumbprint, err := oidcIssuerThumbprint(oidcIssuerUrl)
	if err != nil {
		return "", err
	}

	createProviderOutput, err := iamClient.CreateOpenIDConnectProvider(
		context.Background(),
		&iam.CreateOpenIDConnectProviderInput{
			Url:            aws.String(oidcIssuerUrl),
			ClientIDList:   []string{"sts.amazonaws.com"},
			ThumbprintList: []string{thumbprint},
			Tags:           i.iamTags(i.RuntimeInstanceName),
		},
	)
	if err == nil {
		return *createProviderOutput.OpenIDConnectProviderArn, nil
	}
	if !isAwsErrorCode(err, "EntityAlreadyExists") {
		return "", err
	}

	return i.findOidcProviderArn(iamClient, oidcIssuerUrl)
}

// findOidcProviderArn returns the ARN of the identity provider registered for
// an issuer URL.
func (i *KubernetesRuntimeInfraEKS) findOidcProviderArn(
	iamClient *iam.Client,
	oidcIssuerUrl string,
) (string, error) {
	// the provider's ARN ends in the issuer URL without its scheme, so the
	// provider can be recognized without describing each one
	oidcProvider := strings.TrimPrefix(oidcIssuerUrl, "https://")

	listProvidersOutput, err := iamClient.ListOpenIDConnectProviders(
		context.Background(),
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

// findOidcProviderArnByTag returns the ARN of the identity provider tagged as
// belonging to this cluster, or an empty string when there is none.
func (i *KubernetesRuntimeInfraEKS) findOidcProviderArnByTag(iamClient *iam.Client) (string, error) {
	listProvidersOutput, err := iamClient.ListOpenIDConnectProviders(
		context.Background(),
		&iam.ListOpenIDConnectProvidersInput{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to list OIDC identity providers: %w", err)
	}

	for _, provider := range listProvidersOutput.OpenIDConnectProviderList {
		if provider.Arn == nil {
			continue
		}
		getProviderOutput, err := iamClient.GetOpenIDConnectProvider(
			context.Background(),
			&iam.GetOpenIDConnectProviderInput{OpenIDConnectProviderArn: provider.Arn},
		)
		if err != nil {
			if isIamNotFound(err) {
				continue
			}
			return "", fmt.Errorf("failed to get OIDC identity provider %s: %w", *provider.Arn, err)
		}
		if iamTagsMatch(getProviderOutput.Tags, i.iamTags(i.RuntimeInstanceName)) {
			return *provider.Arn, nil
		}
	}

	return "", nil
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
func (i *KubernetesRuntimeInfraEKS) deleteOidcProvider(iamClient *iam.Client) error {
	oidcProviderArn, err := i.findOidcProviderArnByTag(iamClient)
	if err != nil {
		return err
	}
	if oidcProviderArn == "" {
		// nothing was registered for this cluster, or it is already gone
		return nil
	}

	_, err = iamClient.DeleteOpenIDConnectProvider(
		context.Background(),
		&iam.DeleteOpenIDConnectProviderInput{OpenIDConnectProviderArn: aws.String(oidcProviderArn)},
	)
	if err != nil && !isIamNotFound(err) {
		return fmt.Errorf("failed to delete OIDC identity provider %s: %w", oidcProviderArn, err)
	}

	return nil
}

// createEbsStorageAddon installs the EBS CSI driver on the cluster, running as
// the storage management role.
func (i *KubernetesRuntimeInfraEKS) createEbsStorageAddon(storageManagementRoleArn string) error {
	if storageManagementRoleArn == "" {
		return errors.New("no storage management role to bind the EBS storage addon to")
	}

	eksClient := awseks.NewFromConfig(*i.AwsConfig)
	_, err := eksClient.CreateAddon(context.Background(), &awseks.CreateAddonInput{
		AddonName:             aws.String(tpaws.EksEbsStorageAddonName),
		ClusterName:           aws.String(i.RuntimeInstanceName),
		ServiceAccountRoleArn: aws.String(storageManagementRoleArn),
		Tags:                  ThreeportProviderTags(),
	})
	if err != nil && !isAwsErrorCode(err, "ResourceInUseException") {
		return err
	}

	return nil
}

// oidcIssuerThumbprint returns the SHA1 thumbprint of the root certificate the
// OIDC issuer presents, which IAM requires to register the issuer as an
// identity provider.
func oidcIssuerThumbprint(oidcIssuerUrl string) (string, error) {
	issuerUrl, err := url.Parse(oidcIssuerUrl)
	if err != nil {
		return "", fmt.Errorf("failed to parse OIDC issuer URL: %w", err)
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:443", issuerUrl.Hostname()), &tls.Config{})
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

// attachIamPolicy attaches a policy to a role unless it is attached already.
// AWS does not document AttachRolePolicy as idempotent, and its own examples
// check before attaching, so a role that already carries the policy is left
// alone rather than relying on the call being harmless.
func attachIamPolicy(iamClient *iam.Client, roleName, policyArn string) error {
	attachedPolicies, err := listAttachedPolicies(iamClient, roleName)
	if err != nil {
		return err
	}
	for _, attachedPolicy := range attachedPolicies {
		if attachedPolicy.PolicyArn != nil && *attachedPolicy.PolicyArn == policyArn {
			return nil
		}
	}

	_, err = iamClient.AttachRolePolicy(context.Background(), &iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		return fmt.Errorf("failed to attach policy %s to role %s: %w", policyArn, roleName, err)
	}

	return nil
}

// listAttachedPolicies returns every managed policy attached to a role.
func listAttachedPolicies(iamClient *iam.Client, roleName string) ([]types.AttachedPolicy, error) {
	var attachedPolicies []types.AttachedPolicy

	paginator := iam.NewListAttachedRolePoliciesPaginator(
		iamClient,
		&iam.ListAttachedRolePoliciesInput{RoleName: aws.String(roleName)},
	)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
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
func deleteIamRole(iamClient *iam.Client, roleName string, wantTags []types.Tag) error {
	getRoleOutput, err := iamClient.GetRole(
		context.Background(),
		&iam.GetRoleInput{RoleName: aws.String(roleName)},
	)
	if err != nil {
		if isIamNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get role %s before deleting it: %w", roleName, err)
	}
	if err := refuseUnownedRole(getRoleOutput.Role, wantTags); err != nil {
		return err
	}

	return deleteOwnedIamRole(iamClient, roleName)
}

// deleteOwnedIamRole removes a role once its ownership has been established.
func deleteOwnedIamRole(iamClient *iam.Client, roleName string) error {
	attachedPolicies, err := listAttachedPolicies(iamClient, roleName)
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
		_, err := iamClient.DetachRolePolicy(context.Background(), &iam.DetachRolePolicyInput{
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
		context.Background(),
		&iam.DeleteRoleInput{RoleName: aws.String(roleName)},
	)
	if err != nil && !isIamNotFound(err) {
		return fmt.Errorf("failed to delete role %s: %w", roleName, err)
	}

	return nil
}

// deleteIamPolicy deletes a customer managed policy along with any versions
// it accumulated.  A policy that is already gone is not an error.
func deleteIamPolicy(iamClient *iam.Client, policyArn string) error {
	listVersionsOutput, err := iamClient.ListPolicyVersions(
		context.Background(),
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
		_, err := iamClient.DeletePolicyVersion(context.Background(), &iam.DeletePolicyVersionInput{
			PolicyArn: aws.String(policyArn),
			VersionId: policyVersion.VersionId,
		})
		if err != nil && !isIamNotFound(err) {
			return fmt.Errorf(
				"failed to delete version %s of policy %s: %w",
				*policyVersion.VersionId, policyArn, err,
			)
		}
	}

	_, err = iamClient.DeletePolicy(
		context.Background(),
		&iam.DeletePolicyInput{PolicyArn: aws.String(policyArn)},
	)
	if err != nil && !isIamNotFound(err) {
		return fmt.Errorf("failed to delete policy %s: %w", policyArn, err)
	}

	return nil
}

// isAwsErrorCode reports whether an error is the named AWS API error.
func isAwsErrorCode(err error, code string) bool {
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
