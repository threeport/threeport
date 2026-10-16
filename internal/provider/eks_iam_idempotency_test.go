package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alreadyExists is what IAM answers when the name is taken.
func alreadyExists() error {
	return &smithy.GenericAPIError{Code: "EntityAlreadyExists", Message: "already exists"}
}

// fakeIam answers the four calls this path makes. Anything else is not
// implemented, so a change that starts calling something else shows up.
type fakeIam struct {
	roleExists   bool
	policyExists bool
	existingTags []types.Tag

	createRoleErr error
	getRoleErr    error

	creates, gets, policies, attaches int
	attachedArn                       string
}

func (f *fakeIam) CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	f.creates++
	if f.createRoleErr != nil {
		return nil, f.createRoleErr
	}
	if f.roleExists {
		return nil, alreadyExists()
	}

	return &iam.CreateRoleOutput{Role: &types.Role{RoleName: aws.String("the-role")}}, nil
}

func (f *fakeIam) GetRole(_ context.Context, in *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.gets++
	if f.getRoleErr != nil {
		return nil, f.getRoleErr
	}

	return &iam.GetRoleOutput{Role: &types.Role{RoleName: in.RoleName, Tags: f.existingTags}}, nil
}

func (f *fakeIam) CreatePolicy(_ context.Context, in *iam.CreatePolicyInput, _ ...func(*iam.Options)) (*iam.CreatePolicyOutput, error) {
	f.policies++
	if f.policyExists {
		return nil, alreadyExists()
	}

	return &iam.CreatePolicyOutput{Policy: &types.Policy{
		Arn: aws.String("arn:aws:iam::123456789012:policy/" + aws.ToString(in.PolicyName)),
	}}, nil
}

func (f *fakeIam) AttachRolePolicy(_ context.Context, in *iam.AttachRolePolicyInput, _ ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	f.attaches++
	f.attachedArn = aws.ToString(in.PolicyArn)

	return &iam.AttachRolePolicyOutput{}, nil
}

// TestARetryAdoptsTheRoleTheFirstAttemptCreated covers the retry nobody can
// run by hand.
//
// The role name is derived from the control plane's, so it is the same on
// every attempt. An install that failed after creating it could not be run
// again: the second attempt asked for a role that existed, AWS refused, and
// it kept refusing until somebody deleted the role in a console.
func TestARetryAdoptsTheRoleTheFirstAttemptCreated(t *testing.T) {
	ours := []types.Tag{{Key: aws.String("threeport"), Value: aws.String("the-role")}}

	t.Run("a first attempt creates it", func(t *testing.T) {
		client := &fakeIam{}

		role, err := createRoleIfAbsent(client, &iam.CreateRoleInput{RoleName: aws.String("the-role")}, ours)
		require.NoError(t, err)
		assert.Equal(t, "the-role", aws.ToString(role.RoleName))
		assert.Zero(t, client.gets, "nothing to look up when the create worked")
	})

	t.Run("a retry adopts what the first attempt left", func(t *testing.T) {
		client := &fakeIam{roleExists: true, existingTags: ours}

		role, err := createRoleIfAbsent(client, &iam.CreateRoleInput{RoleName: aws.String("the-role")}, ours)
		require.NoError(t, err, "this is the retry that used to need a console to unblock")
		assert.Equal(t, "the-role", aws.ToString(role.RoleName))
		assert.Equal(t, 1, client.gets)
	})

	t.Run("a role somebody else owns is refused", func(t *testing.T) {
		client := &fakeIam{
			roleExists:   true,
			existingTags: []types.Tag{{Key: aws.String("owner"), Value: aws.String("someone-else")}},
		}

		_, err := createRoleIfAbsent(client, &iam.CreateRoleInput{RoleName: aws.String("the-role")}, ours)
		require.Error(t, err, "a name is not proof of ownership")
		assert.Contains(t, err.Error(), "not created by threeport")
	})

	t.Run("a create that failed for another reason is not an adoption", func(t *testing.T) {
		client := &fakeIam{createRoleErr: errors.New("access denied")}

		_, err := createRoleIfAbsent(client, &iam.CreateRoleInput{RoleName: aws.String("the-role")}, ours)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
		assert.Zero(
			t, client.gets,
			"only 'already exists' means there is something to look up; anything else has to surface as it is",
		)
	})

	t.Run("a role that exists but cannot be read is an error", func(t *testing.T) {
		client := &fakeIam{roleExists: true, getRoleErr: errors.New("could not read it")}

		_, err := createRoleIfAbsent(client, &iam.CreateRoleInput{RoleName: aws.String("the-role")}, ours)
		require.Error(t, err, "not being able to check ownership is not permission to proceed")
		assert.Contains(t, err.Error(), "could not read it")
	})
}

// TestARetryAdoptsThePolicyToo covers the other half of the same attempt.
//
// The policy is created between the role and the attach, so a failure can
// leave it behind just as easily - and its name is derived the same way.
func TestARetryAdoptsThePolicyToo(t *testing.T) {
	t.Run("a first attempt creates and attaches it", func(t *testing.T) {
		client := &fakeIam{}

		require.NoError(t, AttachPolicy("{}", "the-role", "resource-manager", "123456789012", client))
		assert.Equal(t, 1, client.attaches)
		assert.Contains(t, client.attachedArn, "the-role-resource-manager")
	})

	t.Run("a retry attaches the policy already there", func(t *testing.T) {
		client := &fakeIam{policyExists: true}

		require.NoError(
			t, AttachPolicy("{}", "the-role", "resource-manager", "123456789012", client),
			"the policy existing is what a retry finds, not a reason to stop",
		)
		assert.Equal(t, 1, client.attaches, "it still has to be attached to the role")
		assert.Equal(
			t, "arn:aws:iam::123456789012:policy/the-role-resource-manager", client.attachedArn,
			"the arn is derived from the account and the name, which a retry has",
		)
	})
}
