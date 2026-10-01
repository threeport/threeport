package v0

import (
	"fmt"
	"unicode/utf8"
)

const (
	// IamRoleNameMaxLength is the longest name AWS accepts for an IAM role.
	IamRoleNameMaxLength = 64

	// EksClusterPolicyArn is the AWS managed policy the EKS control plane
	// needs to manage resources on the cluster's behalf.
	EksClusterPolicyArn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"

	// EksWorkerNodePolicyArn is the AWS managed policy a worker node needs to
	// join the cluster.
	EksWorkerNodePolicyArn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"

	// EksContainerRegistryPolicyArn is the AWS managed policy a worker node
	// needs to pull images from ECR.
	EksContainerRegistryPolicyArn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"

	// EksCniPolicyArn is the AWS managed policy the VPC CNI plugin needs to
	// attach network interfaces to nodes.
	EksCniPolicyArn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"

	// EksCsiDriverPolicyArn is the AWS managed policy the EBS CSI driver needs
	// to manage volumes.
	EksCsiDriverPolicyArn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"

	// EksEbsStorageAddonName is the name of the EKS addon that installs the
	// EBS CSI driver.
	EksEbsStorageAddonName = "aws-ebs-csi-driver"
)

// IAM role and policy name prefixes.  A cluster's role is the prefix and the
// cluster name joined by a dash.  These names are part of the ARNs annotated
// onto the add-ons' service accounts, so they are kept as aws-builder wrote
// them.
const (
	eksClusterRoleNamePrefix            = "cluster-role"
	eksNodeRoleNamePrefix               = "worker-role"
	eksDnsManagementRoleNamePrefix      = "dns-mgmt-role"
	eksDns01ChallengeRoleNamePrefix     = "dns-chlg-role"
	eksSecretsManagerRoleNamePrefix     = "secrets-manager-role"
	eksClusterAutoscalingRoleNamePrefix = "ca-role"
	eksStorageManagementRoleNamePrefix  = "csi-role"

	eksDnsPolicyNamePrefix                = "DNSUpdates"
	eksDns01ChallengePolicyNamePrefix     = "DNS01Challenge"
	eksSecretsManagerPolicyNamePrefix     = "SecretsManager"
	eksClusterAutoscalingPolicyNamePrefix = "ClusterAutoscaler"
)

// CheckIamRoleName returns an error when a role name exceeds what AWS accepts.
func CheckIamRoleName(roleName string) error {
	if utf8.RuneCountInString(roleName) > IamRoleNameMaxLength {
		return fmt.Errorf(
			"role name %s is %d characters, must be %d or less",
			roleName, utf8.RuneCountInString(roleName), IamRoleNameMaxLength,
		)
	}

	return nil
}

// EksIamPath is the IAM path the cluster's customer managed policies are
// created under, which keeps them grouped per cluster.
func EksIamPath(clusterName string) string {
	return fmt.Sprintf("/%s/", clusterName)
}

// EksClusterRoleName returns the name of the role the EKS control plane
// assumes.
func EksClusterRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksClusterRoleNamePrefix, clusterName)
}

// EksNodeRoleName returns the name of the role the cluster's worker nodes
// assume.
func EksNodeRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksNodeRoleNamePrefix, clusterName)
}

// EksDnsManagementRoleName returns the name of the role external-dns assumes
// to manage Route53 records.
func EksDnsManagementRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksDnsManagementRoleNamePrefix, clusterName)
}

// EksDns01ChallengeRoleName returns the name of the role cert-manager assumes
// to complete DNS01 challenges.
func EksDns01ChallengeRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksDns01ChallengeRoleNamePrefix, clusterName)
}

// EksSecretsManagerRoleName returns the name of the role external-secrets
// assumes to read from Secrets Manager.
func EksSecretsManagerRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksSecretsManagerRoleNamePrefix, clusterName)
}

// EksClusterAutoscalingRoleName returns the name of the role the cluster
// autoscaler assumes to resize node groups.
func EksClusterAutoscalingRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksClusterAutoscalingRoleNamePrefix, clusterName)
}

// EksStorageManagementRoleName returns the name of the role the EBS CSI driver
// assumes to manage volumes.
func EksStorageManagementRoleName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksStorageManagementRoleNamePrefix, clusterName)
}

// EksDnsPolicyName returns the name of the customer managed policy attached to
// the DNS management role.
func EksDnsPolicyName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksDnsPolicyNamePrefix, clusterName)
}

// EksDns01ChallengePolicyName returns the name of the customer managed policy
// attached to the DNS01 challenge role.
func EksDns01ChallengePolicyName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksDns01ChallengePolicyNamePrefix, clusterName)
}

// EksSecretsManagerPolicyName returns the name of the customer managed policy
// attached to the secrets manager role.
func EksSecretsManagerPolicyName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksSecretsManagerPolicyNamePrefix, clusterName)
}

// EksClusterAutoscalingPolicyName returns the name of the customer managed
// policy attached to the cluster autoscaling role.
func EksClusterAutoscalingPolicyName(clusterName string) string {
	return fmt.Sprintf("%s-%s", eksClusterAutoscalingPolicyNamePrefix, clusterName)
}

// IamRoleArn assembles the ARN of a role from the account it belongs to, the
// IAM path it was created under and its name.  Every component that annotates
// a service account with a role ARN derives it here, so a change to a name or
// a path cannot leave one of them pointing at a role that does not exist.
func IamRoleArn(accountId, path, roleName string) string {
	if path == "" || path == "/" {
		return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, roleName)
	}

	return fmt.Sprintf("arn:aws:iam::%s:role%s%s", accountId, path, roleName)
}

// EksDnsManagementRoleArn returns the ARN annotated onto the external-dns
// service account.  This role is the one aws-builder created under the
// cluster's IAM path; the others were left at the root path and stay there so
// that existing ARNs keep resolving.
func EksDnsManagementRoleArn(accountId, clusterName string) string {
	return IamRoleArn(accountId, EksIamPath(clusterName), EksDnsManagementRoleName(clusterName))
}

// EksDns01ChallengeRoleArn returns the ARN annotated onto the cert-manager
// service account.
func EksDns01ChallengeRoleArn(accountId, clusterName string) string {
	return IamRoleArn(accountId, "", EksDns01ChallengeRoleName(clusterName))
}

// EksSecretsManagerRoleArn returns the ARN annotated onto the
// external-secrets service account.
func EksSecretsManagerRoleArn(accountId, clusterName string) string {
	return IamRoleArn(accountId, "", EksSecretsManagerRoleName(clusterName))
}

// EksClusterAutoscalingRoleArn returns the ARN annotated onto the cluster
// autoscaler service account.
func EksClusterAutoscalingRoleArn(accountId, clusterName string) string {
	return IamRoleArn(accountId, "", EksClusterAutoscalingRoleName(clusterName))
}

// EksStorageManagementRoleArn returns the ARN the EBS CSI addon is created
// with.
func EksStorageManagementRoleArn(accountId, clusterName string) string {
	return IamRoleArn(accountId, "", EksStorageManagementRoleName(clusterName))
}
