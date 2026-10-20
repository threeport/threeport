package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// testEksIamInfra returns an EKS infra object for the IAM bootstrap tests.
func testEksIamInfra() *KubernetesRuntimeInfraEKS {
	return &KubernetesRuntimeInfraEKS{
		PulumiWorkspace: PulumiWorkspace{RuntimeInstanceName: "my-cluster"},
		AwsAccountID:    "123456789012",
	}
}

// policyStatements unmarshals a policy document and returns its statements.
func policyStatements(t *testing.T, document string) []map[string]any {
	t.Helper()

	var policy struct {
		Version   string           `json:"Version"`
		Statement []map[string]any `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal([]byte(document), &policy), "policy document is not valid JSON")
	assert.Equal(t, "2012-10-17", policy.Version)
	require.NotEmpty(t, policy.Statement)

	return policy.Statement
}

// TestIrsaRoles checks every add-on role is described completely: a role name,
// a service account to bind it to, and exactly one source of permissions.
func TestIrsaRoles(t *testing.T) {
	infra := testEksIamInfra()
	irsaRoles := infra.irsaRoles()

	// external-dns, cert-manager, external-secrets, the cluster autoscaler and
	// the EBS CSI driver
	require.Len(t, irsaRoles, 5)

	seenRoleNames := map[string]bool{}
	for _, irsaRole := range irsaRoles {
		require.NotEmpty(t, irsaRole.roleName)
		assert.False(t, seenRoleNames[irsaRole.roleName], "duplicate role %s", irsaRole.roleName)
		seenRoleNames[irsaRole.roleName] = true

		assert.NoError(t, tpaws.CheckIamRoleName(irsaRole.roleName))
		assert.NotEmpty(t, irsaRole.serviceAccountName, "%s has no service account", irsaRole.roleName)
		assert.NotEmpty(t, irsaRole.serviceAccountNamespace, "%s has no namespace", irsaRole.roleName)

		// a role gets its permissions either from a policy this code creates
		// or from an AWS managed one, never both and never neither
		hasOwnPolicy := irsaRole.policyDocument != "" && irsaRole.policyName != ""
		hasManagedPolicy := irsaRole.managedPolicyArn != ""
		assert.NotEqual(
			t,
			hasOwnPolicy,
			hasManagedPolicy,
			"%s must have exactly one source of permissions", irsaRole.roleName,
		)

		if hasOwnPolicy {
			assert.NotEmpty(t, irsaRole.policyDescription, "%s policy has no description", irsaRole.roleName)
			policyStatements(t, irsaRole.policyDocument)
		}
	}
}

// TestIrsaRolesBindThreeportServiceAccounts checks each role is bound to the
// service account the add-on actually runs as.  A mismatch produces a role
// nothing can assume, and it only shows up once the add-on tries.
func TestIrsaRolesBindThreeportServiceAccounts(t *testing.T) {
	infra := testEksIamInfra()
	clusterName := infra.RuntimeInstanceName

	type serviceAccount struct {
		namespace string
		name      string
	}
	wantServiceAccounts := map[string]serviceAccount{
		tpaws.EksDnsManagementRoleName(clusterName): {
			namespace: threeport.DNSManagerServiceAccountNamepace,
			name:      threeport.DNSManagerServiceAccountName,
		},
		tpaws.EksDns01ChallengeRoleName(clusterName): {
			namespace: threeport.DNS01ChallengeServiceAccountNamepace,
			name:      threeport.DNS01ChallengeServiceAccountName,
		},
		tpaws.EksSecretsManagerRoleName(clusterName): {
			namespace: threeport.SecretsManagerServiceAccountNamespace,
			name:      threeport.SecretsManagerServiceAccountName,
		},
		tpaws.EksClusterAutoscalingRoleName(clusterName): {
			namespace: threeport.ClusterAutoscalerNamespace,
			name:      threeport.ClusterAutoscalerServiceAccountName,
		},
		tpaws.EksStorageManagementRoleName(clusterName): {
			namespace: threeport.StorageManagerServiceAccountNamespace,
			name:      threeport.StorageManagerServiceAccountName,
		},
	}

	for _, irsaRole := range infra.irsaRoles() {
		want, ok := wantServiceAccounts[irsaRole.roleName]
		require.True(t, ok, "unexpected role %s", irsaRole.roleName)
		assert.Equal(t, want.namespace, irsaRole.serviceAccountNamespace)
		assert.Equal(t, want.name, irsaRole.serviceAccountName)
		delete(wantServiceAccounts, irsaRole.roleName)
	}
	assert.Empty(t, wantServiceAccounts, "roles missing from the bootstrap")
}

// TestIrsaRoleArnsMatchHelpers checks the roles this code creates are the ones
// the helpers hand out ARNs for.  Those ARNs are annotated onto service
// accounts by other packages, which never see the roles being created.
func TestIrsaRoleArnsMatchHelpers(t *testing.T) {
	infra := testEksIamInfra()
	clusterName := infra.RuntimeInstanceName
	accountId := infra.AwsAccountID

	wantArns := map[string]string{
		tpaws.EksDnsManagementRoleName(clusterName):      tpaws.EksDnsManagementRoleArn(accountId, clusterName),
		tpaws.EksDns01ChallengeRoleName(clusterName):     tpaws.EksDns01ChallengeRoleArn(accountId, clusterName),
		tpaws.EksSecretsManagerRoleName(clusterName):     tpaws.EksSecretsManagerRoleArn(accountId, clusterName),
		tpaws.EksClusterAutoscalingRoleName(clusterName): tpaws.EksClusterAutoscalingRoleArn(accountId, clusterName),
		tpaws.EksStorageManagementRoleName(clusterName):  tpaws.EksStorageManagementRoleArn(accountId, clusterName),
	}

	for _, irsaRole := range infra.irsaRoles() {
		wantArn, ok := wantArns[irsaRole.roleName]
		require.True(t, ok, "no ARN helper for role %s", irsaRole.roleName)
		gotArn := tpaws.IamRoleArn(accountId, irsaRole.rolePath, irsaRole.roleName)
		assert.Equal(
			t,
			wantArn,
			gotArn,
			"role %s is created at a path the ARN helper does not use", irsaRole.roleName,
		)
	}
}

// TestIrsaTrustPolicy checks the trust policy names the cluster's OIDC
// provider and restricts the role to a single service account.  Getting the
// subject condition wrong would let any service account in the cluster assume
// the role.
func TestIrsaTrustPolicy(t *testing.T) {
	oidcProvider := "oidc.eks.us-east-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E"
	trustPolicy := fmt.Sprintf(
		eksIrsaTrustPolicyFormat,
		"123456789012",
		oidcProvider,
		"kube-system",
		"external-dns",
	)

	statements := policyStatements(t, trustPolicy)
	require.Len(t, statements, 1)
	statement := statements[0]

	assert.Equal(t, "Allow", statement["Effect"])
	assert.Equal(t, "sts:AssumeRoleWithWebIdentity", statement["Action"])

	principal, ok := statement["Principal"].(map[string]any)
	require.True(t, ok)
	assert.Equal(
		t,
		fmt.Sprintf("arn:aws:iam::123456789012:oidc-provider/%s", oidcProvider),
		principal["Federated"],
	)

	condition, ok := statement["Condition"].(map[string]any)
	require.True(t, ok)
	stringEquals, ok := condition["StringEquals"].(map[string]any)
	require.True(t, ok)
	assert.Equal(
		t,
		"system:serviceaccount:kube-system:external-dns",
		stringEquals[fmt.Sprintf("%s:sub", oidcProvider)],
	)
	assert.Equal(t, "sts.amazonaws.com", stringEquals[fmt.Sprintf("%s:aud", oidcProvider)])
}

// TestServiceTrustPolicies checks the cluster and node roles trust the AWS
// services that assume them.
func TestServiceTrustPolicies(t *testing.T) {
	servicePrincipals := map[string]string{
		eksClusterTrustPolicyDocument: "eks.amazonaws.com",
		eksNodeTrustPolicyDocument:    "ec2.amazonaws.com",
	}

	for trustPolicy, wantService := range servicePrincipals {
		statements := policyStatements(t, trustPolicy)
		require.Len(t, statements, 1)
		statement := statements[0]

		assert.Equal(t, "Allow", statement["Effect"])
		assert.Equal(t, "sts:AssumeRole", statement["Action"])

		principal, ok := statement["Principal"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, []any{wantService}, principal["Service"])
	}
}

// TestClusterAutoscalingPolicyIsScopedToCluster checks the autoscaler can only
// resize node groups belonging to this cluster.  Without the condition the
// role would let it resize any node group in the account.
func TestClusterAutoscalingPolicyIsScopedToCluster(t *testing.T) {
	infra := testEksIamInfra()

	var autoscalingPolicy string
	for _, irsaRole := range infra.irsaRoles() {
		if irsaRole.roleName == tpaws.EksClusterAutoscalingRoleName(infra.RuntimeInstanceName) {
			autoscalingPolicy = irsaRole.policyDocument
		}
	}
	require.NotEmpty(t, autoscalingPolicy)

	statements := policyStatements(t, autoscalingPolicy)
	mutatingStatementFound := false
	for _, statement := range statements {
		actions, ok := statement["Action"].([]any)
		require.True(t, ok)
		mutating := false
		for _, action := range actions {
			if action == "autoscaling:SetDesiredCapacity" ||
				action == "autoscaling:TerminateInstanceInAutoScalingGroup" {
				mutating = true
			}
		}
		if !mutating {
			continue
		}
		mutatingStatementFound = true

		condition, ok := statement["Condition"].(map[string]any)
		require.True(t, ok, "the mutating statement has no condition on it")
		stringEquals, ok := condition["StringEquals"].(map[string]any)
		require.True(t, ok)
		assert.Equal(
			t,
			"owned",
			stringEquals[fmt.Sprintf("aws:ResourceTag/k8s.io/cluster-autoscaler/%s", infra.RuntimeInstanceName)],
		)
	}
	assert.True(t, mutatingStatementFound)
}

// TestIamTagsMatch checks a provider is recognized by its tags even when AWS
// reports tags this code did not set.
func TestIamTagsMatch(t *testing.T) {
	infra := testEksIamInfra()
	want := tpaws.IamTags(infra.RuntimeInstanceName, ThreeportProviderTags())

	have := append(
		tpaws.IamTags(infra.RuntimeInstanceName, ThreeportProviderTags()),
		types.Tag{Key: aws.String("CostCenter"), Value: aws.String("platform")},
	)
	assert.True(t, iamTagsMatch(have, want))

	assert.False(t, iamTagsMatch(tpaws.IamTags("some-other-cluster", ThreeportProviderTags()), want))
	assert.False(t, iamTagsMatch(nil, want))
}

// TestIamTags checks the tags carry the resource name and threeport ownership.
func TestIamTags(t *testing.T) {
	tags := map[string]string{}
	for _, tag := range tpaws.IamTags("some-role", ThreeportProviderTags()) {
		tags[*tag.Key] = *tag.Value
	}

	assert.Equal(t, "some-role", tags["Name"])
	for key, value := range ThreeportProviderTags() {
		assert.Equal(t, value, tags[key])
	}
}

// TestRefuseUnownedRole checks that a role threeport did not create is
// refused rather than reused or deleted.
//
// Role names are derived from the cluster name, so one can collide with an
// existing role. Reusing it would attach this cluster's add-on policies to
// whatever that role already trusts — the secrets manager role carries
// account-wide Secrets Manager access — and the teardown would delete it.
func TestRefuseUnownedRole(t *testing.T) {
	wantTags := tpaws.IamTags("secrets-manager-role-my-cluster", ThreeportProviderTags())

	t.Run("a role we created is reused", func(t *testing.T) {
		ours := &types.Role{
			RoleName: aws.String("secrets-manager-role-my-cluster"),
			Tags:     wantTags,
		}
		assert.NoError(t, refuseUnownedRole(ours, wantTags, remediateRuntimeNameCollision))
	})

	t.Run("a role carrying extra tags is still ours", func(t *testing.T) {
		ours := &types.Role{
			RoleName: aws.String("secrets-manager-role-my-cluster"),
			Tags: append(
				tpaws.IamTags("secrets-manager-role-my-cluster", ThreeportProviderTags()),
				types.Tag{Key: aws.String("CostCenter"), Value: aws.String("platform")},
			),
		}
		assert.NoError(t, refuseUnownedRole(ours, wantTags, remediateRuntimeNameCollision))
	})

	t.Run("somebody else's role of the same name is refused", func(t *testing.T) {
		theirs := &types.Role{
			RoleName: aws.String("secrets-manager-role-my-cluster"),
			Tags: []types.Tag{
				{Key: aws.String("Owner"), Value: aws.String("platform-team")},
			},
		}
		err := refuseUnownedRole(theirs, wantTags, remediateRuntimeNameCollision)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not created by threeport")
	})

	t.Run("an untagged role of the same name is refused", func(t *testing.T) {
		require.Error(t, refuseUnownedRole(&types.Role{
			RoleName: aws.String("secrets-manager-role-my-cluster"),
		}, wantTags, remediateRuntimeNameCollision))
	})

	t.Run("no role at all is an error, not an approval", func(t *testing.T) {
		require.Error(t, refuseUnownedRole(nil, wantTags, remediateRuntimeNameCollision))
	})

	t.Run("the refusal is distinguishable from any other failure", func(t *testing.T) {
		theirs := &types.Role{
			RoleName: aws.String("secrets-manager-role-my-cluster"),
			Tags:     []types.Tag{{Key: aws.String("Owner"), Value: aws.String("platform-team")}},
		}

		err := refuseUnownedRole(theirs, wantTags, remediateRuntimeNameCollision)
		assert.ErrorIs(
			t, err, ErrUnownedIamResource,
			"callers roll back a failed create by deleting the role by name, so a refusal "+
				"they cannot recognise gets the stranger's role deleted",
		)
		assert.NotErrorIs(
			t, errors.New("some other failure"), ErrUnownedIamResource,
			"and anything else has to stay outside it, or a real failure would skip its rollback",
		)
	})

	t.Run("the remediation is the caller's, not this function's", func(t *testing.T) {
		theirs := &types.Role{
			RoleName: aws.String("resource-manager-role-my-control-plane"),
			Tags:     []types.Tag{{Key: aws.String("Owner"), Value: aws.String("platform-team")}},
		}

		err := refuseUnownedRole(theirs, wantTags, remediateControlPlaneNameCollision)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rename the control plane")
		assert.NotContains(
			t, err.Error(), "kubernetes runtime instance",
			"this role is not named after one, so telling somebody to rename theirs is a dead end",
		)
	})
}

// TestRefuseUnownedIamResource covers the check that guards every IAM resource
// this file reuses or deletes, not only roles.
//
// The names, paths and tags are deterministic per cluster, so a policy or an
// OIDC provider left by an unrelated process sits exactly where a new one would
// go. Reusing it attaches a stranger's permissions to this cluster's
// identities, and teardown then deletes it out from under its real owner.
func TestRefuseUnownedIamResource(t *testing.T) {
	want := tpaws.IamTags("some-policy", ThreeportProviderTags())

	assert.NoError(
		t, refuseUnownedIamResource("policy", "some-policy", want, want, remediateRuntimeNameCollision),
		"a resource carrying threeport's tags is ours to use",
	)

	// the same key with a different value is the dangerous case: another
	// threeport install, or another cluster of a different name
	foreign := []types.Tag{{Key: aws.String("Name"), Value: aws.String("someone-elses")}}
	err := refuseUnownedIamResource("policy", "some-policy", foreign, want, remediateRuntimeNameCollision)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "policy")
	assert.Contains(t, err.Error(), "not created by threeport")

	assert.Error(
		t, refuseUnownedIamResource("OIDC provider", "arn:aws:iam::1:oidc-provider/x", nil, want, remediateRuntimeNameCollision),
		"an untagged resource is not ours either",
	)
}

// TestIamEntryPointsGuardTheAwsConfig covers the four entry points that build
// an IAM or EKS client from *i.AwsConfig.
//
// Nothing stops a wiring bug from leaving that nil - commit 175b6fc in this
// same branch fixed four of exactly that class - and dereferencing it gives a
// bare panic from inside the SDK constructor rather than something a reader can
// act on.
func TestIamEntryPointsGuardTheAwsConfig(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*KubernetesRuntimeInfraEKS) error
	}{
		{"CreateClusterIam", func(i *KubernetesRuntimeInfraEKS) error { return i.CreateClusterIam() }},
		{"CreateIrsaIam", func(i *KubernetesRuntimeInfraEKS) error { return i.CreateIrsaIam() }},
		{"DeleteEksIam", func(i *KubernetesRuntimeInfraEKS) error { return i.DeleteEksIam() }},
		{"createEbsStorageAddon", func(i *KubernetesRuntimeInfraEKS) error {
			return i.createEbsStorageAddon(context.Background(), "arn:aws:iam::1:role/x")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			infra := &KubernetesRuntimeInfraEKS{RuntimeInstanceName: "my-cluster"}

			var err error
			require.NotPanics(t, func() { err = test.call(infra) }, "a nil config must not panic")
			require.Error(t, err)
			assert.Contains(
				t, err.Error(), "AWS config",
				"the error has to name what is missing",
			)
		})
	}
}

// TestVpcIdFromStackState covers where the VPC to clean up comes from.
//
// It is read out of the stack's own state rather than looked up by tag,
// because this deletes network interfaces: a tag is a label anyone can reuse,
// so searching the account by name would reach another workload's, while a VPC
// recorded in this stack's state is one this stack created.
func TestVpcIdFromStackState(t *testing.T) {
	infra := testEksIamInfra()
	dir := t.TempDir()
	infra.stateDirRoot = dir

	t.Run("no state file means nothing to clean", func(t *testing.T) {
		vpcId, err := infra.vpcIdFromStackState()
		require.NoError(t, err)
		assert.Empty(t, vpcId)
	})

	statePath, err := infra.GetStateFilePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))

	t.Run("the VPC the stack recorded", func(t *testing.T) {
		require.NoError(t, os.WriteFile(statePath, []byte(`{
			"version": 3,
			"checkpoint": {"latest": {"resources": [
				{"type": "aws:ec2/subnet:Subnet", "id": "subnet-notthisone"},
				{"type": "aws:ec2/vpc:Vpc", "id": "vpc-0123456789abcdef0"}
			]}}
		}`), 0o600))

		vpcId, err := infra.vpcIdFromStackState()
		require.NoError(t, err)
		assert.Equal(t, "vpc-0123456789abcdef0", vpcId)
	})

	t.Run("state with no VPC leaves nothing to scope to", func(t *testing.T) {
		require.NoError(t, os.WriteFile(statePath, []byte(`{
			"version": 3,
			"checkpoint": {"latest": {"resources": [
				{"type": "aws:ec2/subnet:Subnet", "id": "subnet-abc"}
			]}}
		}`), 0o600))

		vpcId, err := infra.vpcIdFromStackState()
		require.NoError(t, err)
		assert.Empty(t, vpcId, "an empty id must not widen the search to the whole account")
	})
}
