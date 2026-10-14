package v0

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEksRoleNames checks the role names stay as aws-builder wrote them.  They
// are part of the ARNs annotated onto the add-ons' service accounts, so a
// rename points a running add-on at a role that does not exist - and the
// add-on only fails once it tries to use the credentials.
func TestEksRoleNames(t *testing.T) {
	clusterName := "my-cluster"

	roleNames := map[string]string{
		EksClusterRoleName(clusterName):            "cluster-role-my-cluster",
		EksNodeRoleName(clusterName):               "worker-role-my-cluster",
		EksDnsManagementRoleName(clusterName):      "dns-mgmt-role-my-cluster",
		EksDns01ChallengeRoleName(clusterName):     "dns-chlg-role-my-cluster",
		EksSecretsManagerRoleName(clusterName):     "secrets-manager-role-my-cluster",
		EksClusterAutoscalingRoleName(clusterName): "ca-role-my-cluster",
		EksStorageManagementRoleName(clusterName):  "csi-role-my-cluster",
	}
	for got, want := range roleNames {
		assert.Equal(t, want, got)
	}

	policyNames := map[string]string{
		EksDnsPolicyName(clusterName):                "DNSUpdates-my-cluster",
		EksDns01ChallengePolicyName(clusterName):     "DNS01Challenge-my-cluster",
		EksSecretsManagerPolicyName(clusterName):     "SecretsManager-my-cluster",
		EksClusterAutoscalingPolicyName(clusterName): "ClusterAutoscaler-my-cluster",
	}
	for got, want := range policyNames {
		assert.Equal(t, want, got)
	}
}

// TestEksRoleArns checks the ARNs the add-ons' service accounts are annotated
// with.  Only the DNS management role was created under the cluster's IAM
// path; the rest sit at the root path, and the ARN has to reflect that or the
// add-on cannot assume the role.
func TestEksRoleArns(t *testing.T) {
	accountId := "123456789012"
	clusterName := "my-cluster"

	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/my-cluster/dns-mgmt-role-my-cluster",
		EksDnsManagementRoleArn(accountId, clusterName),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/dns-chlg-role-my-cluster",
		EksDns01ChallengeRoleArn(accountId, clusterName),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/secrets-manager-role-my-cluster",
		EksSecretsManagerRoleArn(accountId, clusterName),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/ca-role-my-cluster",
		EksClusterAutoscalingRoleArn(accountId, clusterName),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/csi-role-my-cluster",
		EksStorageManagementRoleArn(accountId, clusterName),
	)
}

// TestClusterAutoscalingRoleArnMatchesPreviousFormat pins the autoscaler ARN
// to the string the installer used to build by hand from the aws-builder
// constant, so switching to the shared helper cannot have moved it.
func TestClusterAutoscalingRoleArnMatchesPreviousFormat(t *testing.T) {
	accountId := "123456789012"
	clusterName := "my-cluster"

	// what pkg/threeport-installer built before: the "ca-role" constant from
	// aws-builder joined to the cluster name at the root path
	previous := fmt.Sprintf("arn:aws:iam::%s:role/%s-%s", accountId, "ca-role", clusterName)

	assert.Equal(t, previous, EksClusterAutoscalingRoleArn(accountId, clusterName))
}

// TestIamRoleArnPath checks a role at the root path gets no double slash.
func TestIamRoleArnPath(t *testing.T) {
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/some-role",
		IamRoleArn("123456789012", "", "some-role"),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/some-role",
		IamRoleArn("123456789012", "/", "some-role"),
	)
	assert.Equal(
		t,
		"arn:aws:iam::123456789012:role/my-cluster/some-role",
		IamRoleArn("123456789012", "/my-cluster/", "some-role"),
	)
}

// TestCheckIamRoleName checks role names are rejected before AWS rejects
// them, which it does at creation time with a cluster half provisioned.
func TestCheckIamRoleName(t *testing.T) {
	require.NoError(t, CheckIamRoleName(strings.Repeat("a", IamRoleNameMaxLength)))

	err := CheckIamRoleName(strings.Repeat("a", IamRoleNameMaxLength+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "65")
}

// TestEksIamPath checks the path customer managed policies are grouped under.
func TestEksIamPath(t *testing.T) {
	assert.Equal(t, "/my-cluster/", EksIamPath("my-cluster"))
}
