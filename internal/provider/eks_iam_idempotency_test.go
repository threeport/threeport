package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tpaws "github.com/threeport/threeport/pkg/aws/v0"
)

// alreadyExists is what IAM answers when the name is taken.
func alreadyExists() error {
	return &smithy.GenericAPIError{Code: errCodeEntityAlreadyExists, Message: "already exists"}
}

// iamEncoded is how IAM hands a policy document back: percent-encoded per RFC
// 3986, which is not the same as form encoding - a space is %20, never '+'.
func iamEncoded(document string) string {
	return strings.ReplaceAll(url.QueryEscape(document), "+", "%20")
}

// ownedTags are the tags an install gives what it creates, built the way the
// call sites build them rather than by hand, so that a change to IamTags or to
// ThreeportProviderTags reaches this test instead of passing against a
// fixture that no longer resembles production.
func ownedTags(name string) []types.Tag {
	return tpaws.IamTags(name, ThreeportProviderTags())
}

// fakeIam answers the calls this path makes. Anything else is not
// implemented, so a change that starts calling something else shows up.
//
// It records the input of every call. Returning a canned answer regardless of
// what was asked would let a refactor that dropped the tags, the trust policy
// or the policy document pass the whole suite.
type fakeIam struct {
	roleExists   bool
	policyExists bool

	existingRoleTags   []types.Tag
	existingPolicyTags []types.Tag

	// storedDocument is what IAM holds for an existing policy, in the encoded
	// form it answers with.
	storedDocument string
	storedVersions []types.PolicyVersion
	attachedArns   []string

	createRoleErr   error
	getRoleErr      error
	getPolicyErr    error
	createPolicyErr error

	creates, gets, policies, attaches, lists int

	// calls is the order things happened in, which is the whole point on the
	// teardown path: a policy still holding versions is refused.
	calls []string

	createRoleInput    *iam.CreateRoleInput
	createPolicyInput  *iam.CreatePolicyInput
	newVersions        []string
	deletedVersionIds  []string
	setDefaultVersions []bool
	attachedArn        string
	detachedArn        string
	deletedPolicyArn   string
	roleDeleted        bool
}

func (f *fakeIam) CreateRole(_ context.Context, in *iam.CreateRoleInput, _ ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	f.creates++
	f.createRoleInput = in
	if f.createRoleErr != nil {
		return nil, f.createRoleErr
	}
	if f.roleExists {
		return nil, alreadyExists()
	}

	return &iam.CreateRoleOutput{Role: &types.Role{
		RoleName: in.RoleName,
		Tags:     in.Tags,
	}}, nil
}

func (f *fakeIam) GetRole(_ context.Context, in *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.gets++
	if f.getRoleErr != nil {
		return nil, f.getRoleErr
	}

	return &iam.GetRoleOutput{Role: &types.Role{RoleName: in.RoleName, Tags: f.existingRoleTags}}, nil
}

func (f *fakeIam) CreatePolicy(_ context.Context, in *iam.CreatePolicyInput, _ ...func(*iam.Options)) (*iam.CreatePolicyOutput, error) {
	f.policies++
	f.createPolicyInput = in
	if f.createPolicyErr != nil {
		return nil, f.createPolicyErr
	}
	if f.policyExists {
		return nil, alreadyExists()
	}

	return &iam.CreatePolicyOutput{Policy: &types.Policy{
		Arn:  aws.String("arn:aws:iam::123456789012:policy/" + aws.ToString(in.PolicyName)),
		Tags: in.Tags,
	}}, nil
}

func (f *fakeIam) GetPolicy(_ context.Context, in *iam.GetPolicyInput, _ ...func(*iam.Options)) (*iam.GetPolicyOutput, error) {
	if f.getPolicyErr != nil {
		return nil, f.getPolicyErr
	}

	return &iam.GetPolicyOutput{Policy: &types.Policy{
		Arn:              in.PolicyArn,
		Tags:             f.existingPolicyTags,
		DefaultVersionId: aws.String("v1"),
	}}, nil
}

func (f *fakeIam) GetPolicyVersion(_ context.Context, in *iam.GetPolicyVersionInput, _ ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error) {
	return &iam.GetPolicyVersionOutput{PolicyVersion: &types.PolicyVersion{
		VersionId: in.VersionId,
		Document:  aws.String(f.storedDocument),
	}}, nil
}

func (f *fakeIam) ListPolicyVersions(context.Context, *iam.ListPolicyVersionsInput, ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error) {
	return &iam.ListPolicyVersionsOutput{Versions: f.storedVersions}, nil
}

func (f *fakeIam) CreatePolicyVersion(_ context.Context, in *iam.CreatePolicyVersionInput, _ ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error) {
	f.newVersions = append(f.newVersions, aws.ToString(in.PolicyDocument))
	f.setDefaultVersions = append(f.setDefaultVersions, in.SetAsDefault)

	return &iam.CreatePolicyVersionOutput{}, nil
}

func (f *fakeIam) DeletePolicyVersion(_ context.Context, in *iam.DeletePolicyVersionInput, _ ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error) {
	f.deletedVersionIds = append(f.deletedVersionIds, aws.ToString(in.VersionId))
	f.calls = append(f.calls, "DeletePolicyVersion:"+aws.ToString(in.VersionId))

	return &iam.DeletePolicyVersionOutput{}, nil
}

func (f *fakeIam) DetachRolePolicy(_ context.Context, in *iam.DetachRolePolicyInput, _ ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error) {
	f.calls = append(f.calls, "DetachRolePolicy")
	f.detachedArn = aws.ToString(in.PolicyArn)

	return &iam.DetachRolePolicyOutput{}, nil
}

func (f *fakeIam) DeletePolicy(_ context.Context, in *iam.DeletePolicyInput, _ ...func(*iam.Options)) (*iam.DeletePolicyOutput, error) {
	f.calls = append(f.calls, "DeletePolicy")
	f.deletedPolicyArn = aws.ToString(in.PolicyArn)

	return &iam.DeletePolicyOutput{}, nil
}

func (f *fakeIam) DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error) {
	f.calls = append(f.calls, "DeleteRole")
	f.roleDeleted = true

	return &iam.DeleteRoleOutput{}, nil
}

func (f *fakeIam) ListAttachedRolePolicies(context.Context, *iam.ListAttachedRolePoliciesInput, ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error) {
	f.lists++
	attached := make([]types.AttachedPolicy, 0, len(f.attachedArns))
	for _, arn := range f.attachedArns {
		attached = append(attached, types.AttachedPolicy{PolicyArn: aws.String(arn)})
	}

	return &iam.ListAttachedRolePoliciesOutput{AttachedPolicies: attached}, nil
}

func (f *fakeIam) AttachRolePolicy(_ context.Context, in *iam.AttachRolePolicyInput, _ ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	f.attaches++
	f.attachedArn = aws.ToString(in.PolicyArn)

	return &iam.AttachRolePolicyOutput{}, nil
}

// roleInput is the input the production path builds, so the assertions below
// are about what reaches IAM rather than about a literal written twice.
func roleInput() *iam.CreateRoleInput {
	return &iam.CreateRoleInput{
		RoleName:                 aws.String("the-role"),
		AssumeRolePolicyDocument: aws.String(`{"Version":"2012-10-17"}`),
		Tags:                     ownedTags("the-role"),
	}
}

// TestARetryAdoptsTheRoleTheFirstAttemptCreated covers the retry nobody can
// run by hand.
//
// The role name is derived from the control plane's, so it is the same on
// every attempt. An install that failed after creating it could not be run
// again: the second attempt asked for a role that existed, AWS refused, and
// it kept refusing until somebody deleted the role in a console.
func TestARetryAdoptsTheRoleTheFirstAttemptCreated(t *testing.T) {
	ours := ownedTags("the-role")

	t.Run("a first attempt creates it", func(t *testing.T) {
		client := &fakeIam{}

		role, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.NoError(t, err)
		assert.Equal(t, "the-role", aws.ToString(role.RoleName))
		assert.Zero(t, client.gets, "nothing to look up when the create worked")
	})

	t.Run("what the caller asked for is what reaches IAM", func(t *testing.T) {
		client := &fakeIam{}

		_, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.NoError(t, err)
		require.NotNil(t, client.createRoleInput)
		assert.Equal(
			t, ours, client.createRoleInput.Tags,
			"the tags are what later proves the role is ours; a create that drops them makes every retry refuse it",
		)
		assert.Equal(
			t, `{"Version":"2012-10-17"}`, aws.ToString(client.createRoleInput.AssumeRolePolicyDocument),
			"the trust policy is who may assume the role, so it cannot be silently replaced",
		)
	})

	t.Run("a retry adopts what the first attempt left", func(t *testing.T) {
		client := &fakeIam{roleExists: true, existingRoleTags: ours}

		role, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.NoError(t, err, "this is the retry that used to need a console to unblock")
		assert.Equal(t, "the-role", aws.ToString(role.RoleName))
		assert.Equal(t, 1, client.gets)
	})

	t.Run("a role carrying only a name tag is refused", func(t *testing.T) {
		client := &fakeIam{
			roleExists:       true,
			existingRoleTags: tpaws.IamTags("the-role", map[string]string{}),
		}

		_, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.Error(
			t, err,
			"a Name tag is a convention half the tooling in an account follows; ownership is the ProvisionedBy marker",
		)
		assert.Contains(t, err.Error(), "not created by threeport")
	})

	t.Run("a role somebody else owns is refused", func(t *testing.T) {
		client := &fakeIam{
			roleExists:       true,
			existingRoleTags: []types.Tag{{Key: aws.String("owner"), Value: aws.String("someone-else")}},
		}

		_, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.Error(t, err, "a name is not proof of ownership")
		assert.Contains(t, err.Error(), "not created by threeport")
	})

	t.Run("a create that failed for another reason is not an adoption", func(t *testing.T) {
		client := &fakeIam{createRoleErr: errors.New("access denied")}

		_, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
		assert.Zero(
			t, client.gets,
			"only 'already exists' means there is something to look up; anything else has to surface as it is",
		)
	})

	t.Run("a role that exists but cannot be read is an error", func(t *testing.T) {
		client := &fakeIam{roleExists: true, getRoleErr: errors.New("could not read it")}

		_, err := createRoleIfAbsent(context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision)
		require.Error(t, err, "not being able to check ownership is not permission to proceed")
		assert.Contains(t, err.Error(), "could not read it")
	})
}

const (
	policyArn      = "arn:aws:iam::123456789012:policy/the-role-resource-manager"
	policyDocument = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"eks:*","Resource":"*"}]}`
)

// attachPolicy runs the production entry point with this file's fixtures.
func attachPolicy(client *fakeIam, document string) error {
	return AttachPolicy(context.Background(), document, "the-role", "resource-manager", "123456789012", client)
}

// TestARetryAdoptsThePolicyToo covers the other half of the same attempt.
//
// The policy is created between the role and the attach, so a failure can
// leave it behind just as easily - and its name is derived the same way.
func TestARetryAdoptsThePolicyToo(t *testing.T) {
	ours := ownedTags("the-role-resource-manager")

	t.Run("a first attempt creates and attaches it", func(t *testing.T) {
		client := &fakeIam{}

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Equal(t, 1, client.attaches)
		assert.Contains(t, client.attachedArn, "the-role-resource-manager")
	})

	t.Run("what the caller asked for is what reaches IAM", func(t *testing.T) {
		client := &fakeIam{}

		require.NoError(t, attachPolicy(client, policyDocument))
		require.NotNil(t, client.createPolicyInput)
		assert.Equal(
			t, policyDocument, aws.ToString(client.createPolicyInput.PolicyDocument),
			"the document is the permissions themselves",
		)
		assert.Equal(
			t, ours, client.createPolicyInput.Tags,
			"an untagged policy is one no later run can prove it owns, so every retry would refuse it",
		)
	})

	t.Run("a retry attaches the policy already there", func(t *testing.T) {
		client := &fakeIam{
			policyExists:       true,
			existingPolicyTags: ours,
			storedDocument:     iamEncoded(policyDocument),
		}

		require.NoError(
			t, attachPolicy(client, policyDocument),
			"the policy existing is what a retry finds, not a reason to stop",
		)
		assert.Equal(t, 1, client.attaches, "it still has to be attached to the role")
		assert.Equal(
			t, policyArn, client.attachedArn,
			"the arn is derived from the account and the name, which a retry has",
		)
	})

	t.Run("a policy somebody else owns is refused", func(t *testing.T) {
		client := &fakeIam{
			policyExists:       true,
			existingPolicyTags: []types.Tag{{Key: aws.String("owner"), Value: aws.String("someone-else")}},
			storedDocument:     iamEncoded(policyDocument),
		}

		err := attachPolicy(client, policyDocument)
		require.Error(
			t, err,
			"the role being ours says nothing about a separate account-level resource that happens to sit at this name",
		)
		assert.Contains(t, err.Error(), "not created by threeport")
		assert.Zero(t, client.attaches, "a stranger's permissions must not reach the role")
	})

	t.Run("a policy that exists but cannot be read is an error", func(t *testing.T) {
		client := &fakeIam{policyExists: true, getPolicyErr: errors.New("could not read it")}

		err := attachPolicy(client, policyDocument)
		require.Error(t, err, "not being able to check ownership is not permission to proceed")
		assert.Contains(t, err.Error(), "could not read it")
		assert.Zero(t, client.attaches)
	})

	t.Run("a policy already attached is left alone", func(t *testing.T) {
		client := &fakeIam{
			policyExists:       true,
			existingPolicyTags: ours,
			storedDocument:     iamEncoded(policyDocument),
			attachedArns:       []string{policyArn},
		}

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Equal(t, 1, client.lists, "what is attached has to be read before deciding")
		assert.Zero(
			t, client.attaches,
			"AttachRolePolicy is not documented as idempotent, so an attach that is not needed is not made",
		)
	})
}

// TestAnAdoptedPolicyIsBroughtUpToDate covers the gap that outlives the retry.
//
// A policy at a derived name is created once and adopted by every run after
// it. Without this, a release that adds a permission would never reach an
// account installed before it: the install would report success and the role
// would be missing the permission, surfacing later as an AccessDenied with
// nothing pointing back at the install.
func TestAnAdoptedPolicyIsBroughtUpToDate(t *testing.T) {
	ours := ownedTags("the-role-resource-manager")

	adopted := func(stored string) *fakeIam {
		return &fakeIam{
			policyExists:       true,
			existingPolicyTags: ours,
			storedDocument:     stored,
		}
	}

	t.Run("a document that changed is written as the new default", func(t *testing.T) {
		client := adopted(iamEncoded(`{"Version":"2012-10-17","Statement":[]}`))

		require.NoError(t, attachPolicy(client, policyDocument))
		require.Len(t, client.newVersions, 1, "the release asked for permissions the stored policy does not grant")
		assert.Equal(t, policyDocument, client.newVersions[0])
		assert.Equal(
			t, []bool{true}, client.setDefaultVersions,
			"a version that is not the default grants nothing",
		)
	})

	t.Run("a document IAM only reformatted is left alone", func(t *testing.T) {
		reformatted := `{
  "Statement": [ { "Action": "eks:*", "Effect": "Allow", "Resource": "*" } ],
  "Version": "2012-10-17"
}`
		client := adopted(iamEncoded(reformatted))

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Empty(
			t, client.newVersions,
			"IAM reformats what it stores; comparing text would write a version every run until the limit stopped it",
		)
	})

	t.Run("a literal plus survives the round trip", func(t *testing.T) {
		withPlus := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"arn:aws:s3:::a+b"}]}`
		// '+' is a sub-delimiter RFC 3986 allows unencoded, so a document can
		// come back from IAM carrying one as itself rather than as %2B.
		stored := strings.ReplaceAll(iamEncoded(withPlus), "%2B", "+")
		require.Contains(t, stored, "a+b", "this case is only worth anything if the '+' is left raw")
		client := adopted(stored)

		require.NoError(t, attachPolicy(client, withPlus))
		assert.Empty(
			t, client.newVersions,
			"form decoding would read the '+' in this ARN as a space, call an unchanged policy changed, "+
				"and write a new version on every run until the limit stopped it",
		)
	})

	t.Run("the oldest version makes room when the policy is at the limit", func(t *testing.T) {
		client := adopted(iamEncoded(`{"Version":"2012-10-17","Statement":[]}`))
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		client.storedVersions = []types.PolicyVersion{
			{VersionId: aws.String("v5"), CreateDate: aws.Time(base.Add(4 * time.Hour))},
			{VersionId: aws.String("v2"), CreateDate: aws.Time(base.Add(1 * time.Hour))},
			{VersionId: aws.String("v4"), CreateDate: aws.Time(base.Add(3 * time.Hour))},
			{VersionId: aws.String("v1"), CreateDate: aws.Time(base), IsDefaultVersion: true},
			{VersionId: aws.String("v3"), CreateDate: aws.Time(base.Add(2 * time.Hour))},
		}

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Equal(
			t, []string{"v2"}, client.deletedVersionIds,
			"the oldest that is not in force goes; IAM refuses a sixth version",
		)
		require.Len(t, client.newVersions, 1)
	})

	t.Run("the version in force is never the one removed", func(t *testing.T) {
		client := adopted(iamEncoded(`{"Version":"2012-10-17","Statement":[]}`))
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		// v1 is both the oldest and the one in force
		client.storedVersions = []types.PolicyVersion{
			{VersionId: aws.String("v1"), CreateDate: aws.Time(base), IsDefaultVersion: true},
			{VersionId: aws.String("v2"), CreateDate: aws.Time(base.Add(1 * time.Hour))},
			{VersionId: aws.String("v3"), CreateDate: aws.Time(base.Add(2 * time.Hour))},
			{VersionId: aws.String("v4"), CreateDate: aws.Time(base.Add(3 * time.Hour))},
			{VersionId: aws.String("v5"), CreateDate: aws.Time(base.Add(4 * time.Hour))},
		}

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Equal(
			t, []string{"v2"}, client.deletedVersionIds,
			"removing the version in force would strip the role of its permissions mid-install",
		)
	})

	t.Run("a policy with fewer versions than the limit loses none", func(t *testing.T) {
		client := adopted(iamEncoded(`{"Version":"2012-10-17","Statement":[]}`))
		client.storedVersions = []types.PolicyVersion{
			{VersionId: aws.String("v1"), IsDefaultVersion: true},
		}

		require.NoError(t, attachPolicy(client, policyDocument))
		assert.Empty(t, client.deletedVersionIds, "there is room, so nothing has to go")
	})
}

// TestTearingDownARoleClearsThePolicyVersionsFirst covers what updating an
// adopted policy costs on the way out.
//
// IAM refuses to delete a policy that still holds versions other than the one
// in force. These policies used to have exactly one - they were created once
// and never touched - so the teardown could delete them directly. Now that an
// adopted policy's document is brought up to date, a control plane that has
// been upgraded holds policies with several, and the teardown detaches them
// before it tries to delete them: a failure there leaves a detached policy
// behind, and the next teardown does not look at it because it is no longer
// attached to anything.
func TestTearingDownARoleClearsThePolicyVersionsFirst(t *testing.T) {
	t.Run("a policy whose document was updated is still deleted", func(t *testing.T) {
		client := &fakeIam{
			attachedArns: []string{policyArn},
			storedVersions: []types.PolicyVersion{
				{VersionId: aws.String("v1")},
				{VersionId: aws.String("v2")},
				{VersionId: aws.String("v3"), IsDefaultVersion: true},
			},
		}

		require.NoError(t, deleteRoleAndItsPolicies(context.Background(), client, "the-role"))
		assert.Equal(t, []string{"v1", "v2"}, client.deletedVersionIds)
		assert.Equal(t, policyArn, client.deletedPolicyArn, "the policy itself still has to go")
		assert.True(t, client.roleDeleted)
	})

	t.Run("the version in force goes with the policy, not before it", func(t *testing.T) {
		client := &fakeIam{
			attachedArns: []string{policyArn},
			storedVersions: []types.PolicyVersion{
				{VersionId: aws.String("v1"), IsDefaultVersion: true},
				{VersionId: aws.String("v2")},
			},
		}

		require.NoError(t, deleteRoleAndItsPolicies(context.Background(), client, "the-role"))
		assert.NotContains(
			t, client.deletedVersionIds, "v1",
			"IAM refuses to delete the version in force on its own",
		)
	})

	t.Run("the versions go before the policy does", func(t *testing.T) {
		client := &fakeIam{
			attachedArns: []string{policyArn},
			storedVersions: []types.PolicyVersion{
				{VersionId: aws.String("v1")},
				{VersionId: aws.String("v2"), IsDefaultVersion: true},
			},
		}

		require.NoError(t, deleteRoleAndItsPolicies(context.Background(), client, "the-role"))
		assert.Equal(
			t,
			[]string{"DetachRolePolicy", "DeletePolicyVersion:v1", "DeletePolicy", "DeleteRole"},
			client.calls,
			"deleting the policy before its versions is the call IAM refuses",
		)
	})

	t.Run("a policy that was never updated loses no versions", func(t *testing.T) {
		client := &fakeIam{
			attachedArns:   []string{policyArn},
			storedVersions: []types.PolicyVersion{{VersionId: aws.String("v1"), IsDefaultVersion: true}},
		}

		require.NoError(t, deleteRoleAndItsPolicies(context.Background(), client, "the-role"))
		assert.Empty(t, client.deletedVersionIds, "there is nothing in the way")
		assert.Equal(t, policyArn, client.deletedPolicyArn)
	})
}

// TestTheRefusalSurvivesEveryWrapping covers what the caller's rollback
// depends on.
//
// pkg/cli/v0/provider_eks.go rolls a failed create back by deleting the role
// by name, and skips that only when the error is ErrUnownedIamResource. The
// refusal travels up through several fmt.Errorf calls to get there, so one of
// them using %v instead of %w would not fail to compile, would not fail any
// other test, and would quietly go back to deleting a stranger's role.
func TestTheRefusalSurvivesEveryWrapping(t *testing.T) {
	ours := ownedTags("the-role")
	theirs := []types.Tag{{Key: aws.String("Owner"), Value: aws.String("someone-else")}}

	t.Run("refusing a role", func(t *testing.T) {
		client := &fakeIam{roleExists: true, existingRoleTags: theirs}

		_, err := createRoleIfAbsent(
			context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnownedIamResource)
	})

	t.Run("refusing a policy, through AttachPolicy", func(t *testing.T) {
		client := &fakeIam{
			policyExists:       true,
			existingPolicyTags: theirs,
			storedDocument:     iamEncoded(policyDocument),
		}

		err := attachPolicy(client, policyDocument)
		require.Error(t, err)
		assert.ErrorIs(
			t, err, ErrUnownedIamResource,
			"AttachPolicy returns this through createPolicyIfAbsent, and CreateResourceManagerRole "+
				"wraps it again before the caller sees it",
		)
	})

	t.Run("an ordinary failure is not mistaken for a refusal", func(t *testing.T) {
		client := &fakeIam{createRoleErr: errors.New("access denied")}

		_, err := createRoleIfAbsent(
			context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision,
		)
		require.Error(t, err)
		assert.NotErrorIs(
			t, err, ErrUnownedIamResource,
			"a real failure that skipped its rollback would leave the role it just created behind",
		)
	})

	t.Run("the hint names what this role is actually derived from", func(t *testing.T) {
		client := &fakeIam{roleExists: true, existingRoleTags: theirs}

		_, err := createRoleIfAbsent(
			context.Background(), client, roleInput(), ours, remediateControlPlaneNameCollision,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rename the control plane")
		assert.NotContains(
			t, err.Error(), "kubernetes runtime instance",
			"the resource manager role is named after a control plane, or given with --role-name",
		)
	})
}

// TestTheRollbackLeavesAStrangersRoleAlone covers the decision pkg/cli/v0
// used to make for itself and get wrong.
//
// That caller deletes the resource manager role by name whenever the create
// fails, and a refusal to adopt is one of those failures - so the check that
// refuses to touch a role threeport did not create was followed, three lines
// later, by deleting it. Putting the decision here means a caller cannot
// forget it, and it can be exercised without an AWS account because the
// refusal path returns before any client is built.
func TestTheRollbackLeavesAStrangersRoleAlone(t *testing.T) {
	t.Run("a refusal is not rolled back", func(t *testing.T) {
		refusal := fmt.Errorf(
			"failed to create runtime manager role: %w",
			fmt.Errorf("IAM role the-role already exists and was %w; %s",
				ErrUnownedIamResource, remediateControlPlaneNameCollision),
		)

		require.NoError(
			t, RollBackResourceManagerRole("the-control-plane", aws.Config{}, refusal),
			"deleting the role the check just refused to adopt is worse than the collision",
		)
	})

	t.Run("the refusal is recognised however deep it is wrapped", func(t *testing.T) {
		client := &fakeIam{
			policyExists:       true,
			existingPolicyTags: []types.Tag{{Key: aws.String("Owner"), Value: aws.String("someone-else")}},
			storedDocument:     iamEncoded(policyDocument),
		}

		// the path CreateResourceManagerRole takes: AttachPolicy, through
		// createPolicyIfAbsent, wrapped again on the way out
		err := attachResourceManagerPolicies(
			context.Background(), client, "the-role", "123456789012", false, true,
		)
		require.Error(t, err)
		assert.ErrorIs(
			t, err, ErrUnownedIamResource,
			"a %v anywhere on this path compiles and reads the same, and silently turns the rollback back on",
		)
		require.NoError(t, RollBackResourceManagerRole("the-control-plane", aws.Config{}, err))
	})

	t.Run("an ordinary failure is still rolled back", func(t *testing.T) {
		// no AWS config to talk to, so reaching the delete is the observable
		// outcome: the point is that it does not short-circuit
		err := RollBackResourceManagerRole(
			"the-control-plane", aws.Config{}, errors.New("access denied"),
		)
		require.Error(
			t, err,
			"a real failure that skipped its rollback would leave behind the role it just created",
		)
		assert.NotContains(t, err.Error(), "access denied", "the rollback reports its own failure")
	})
}
