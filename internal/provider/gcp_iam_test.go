package provider

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// withNoIAMRetryDelay stops the retry tests sleeping between attempts.
func withNoIAMRetryDelay(t *testing.T) {
	t.Helper()
	previous := iamRetryDelay
	iamRetryDelay = time.Nanosecond
	t.Cleanup(func() { iamRetryDelay = previous })
}

// fakePolicies is a project policy store with an etag, as the real one has: a
// write carrying an etag other than the current one is a 409.
type fakePolicies struct {
	policy   *cloudresourcemanager.Policy
	etag     int
	getErrs  []error
	setErrs  []error
	gets     int
	sets     int
	conflict int // conflicting writes by another party still to come
}

func (f *fakePolicies) get(string) (*cloudresourcemanager.Policy, error) {
	f.gets++
	if len(f.getErrs) > 0 {
		err := f.getErrs[0]
		f.getErrs = f.getErrs[1:]
		return nil, err
	}
	// a copy, so a mutation that is never written does not leak into the store
	copied := &cloudresourcemanager.Policy{Etag: fmt.Sprint(f.etag)}
	for _, b := range f.policy.Bindings {
		copied.Bindings = append(copied.Bindings, &cloudresourcemanager.Binding{
			Role:    b.Role,
			Members: append([]string(nil), b.Members...),
		})
	}
	return copied, nil
}

func (f *fakePolicies) set(_ string, policy *cloudresourcemanager.Policy) (*cloudresourcemanager.Policy, error) {
	f.sets++
	if len(f.setErrs) > 0 {
		err := f.setErrs[0]
		f.setErrs = f.setErrs[1:]
		return nil, err
	}
	if f.conflict > 0 {
		// someone else writes first, so this etag is now stale
		f.conflict--
		f.etag++
	}
	if policy.Etag != fmt.Sprint(f.etag) {
		return nil, &googleapi.Error{Code: 409, Message: "etag mismatch"}
	}
	f.etag++
	f.policy = policy
	return policy, nil
}

func policyWith(bindings ...*cloudresourcemanager.Binding) *cloudresourcemanager.Policy {
	return &cloudresourcemanager.Policy{Bindings: bindings}
}

func binding(role string, members ...string) *cloudresourcemanager.Binding {
	return &cloudresourcemanager.Binding{Role: role, Members: members}
}

const testMember = "serviceAccount:threeport-svc-x@proj.iam.gserviceaccount.com"

func TestRemoveMemberFromPolicy(t *testing.T) {
	policy := policyWith(
		binding("roles/a", testMember, "user:a@example.com"),
		binding("roles/b", testMember),
		binding("roles/c", "user:b@example.com"),
	)

	changed := removeMemberFromPolicy(policy, testMember)

	assert.True(t, changed)
	require.Len(t, policy.Bindings, 2, "a binding left with no members is dropped")
	assert.Equal(t, []string{"user:a@example.com"}, policy.Bindings[0].Members)
	assert.Equal(t, "roles/c", policy.Bindings[1].Role)
	assert.False(t, policyNamesMember(policy, testMember))
}

func TestRemoveMemberFromPolicy_NotPresentIsNoChange(t *testing.T) {
	policy := policyWith(binding("roles/a", "user:a@example.com"))

	assert.False(t, removeMemberFromPolicy(policy, testMember))
	assert.Len(t, policy.Bindings, 1)
}

func TestAddMemberToPolicy(t *testing.T) {
	policy := policyWith(
		binding("roles/a", "user:a@example.com"),
		binding("roles/b", testMember),
		&cloudresourcemanager.Binding{
			Role:      "roles/a",
			Members:   []string{"user:c@example.com"},
			Condition: &cloudresourcemanager.Expr{Expression: "true"},
		},
	)

	assert.True(t, addMemberToPolicy(policy, testMember, []string{"roles/a", "roles/b", "roles/c"}))

	assert.Contains(t, policy.Bindings[0].Members, testMember, "added to the existing plain binding")
	assert.Len(t, policy.Bindings[1].Members, 1, "already present is not added twice")
	assert.NotContains(t, policy.Bindings[2].Members, testMember, "a conditional binding is a different grant")
	assert.Equal(t, "roles/c", policy.Bindings[3].Role)

	assert.False(t, addMemberToPolicy(policy, testMember, []string{"roles/a", "roles/b", "roles/c"}), "a repeat changes nothing")
}

func TestUpdateProjectPolicy_RereadsOnConflict(t *testing.T) {
	withNoIAMRetryDelay(t)
	store := &fakePolicies{
		policy:   policyWith(binding("roles/a", testMember)),
		conflict: 2,
	}

	got, err := updateProjectPolicy(store, "proj", isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return removeMemberFromPolicy(p, testMember)
	})

	require.NoError(t, err)
	assert.False(t, policyNamesMember(got, testMember))
	assert.Equal(t, 3, store.gets, "the policy is read again on each attempt, not retried stale")
	assert.Equal(t, 3, store.sets)
}

func TestUpdateProjectPolicy_NoChangeWritesNothing(t *testing.T) {
	store := &fakePolicies{policy: policyWith(binding("roles/a", "user:a@example.com"))}

	_, err := updateProjectPolicy(store, "proj", isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return removeMemberFromPolicy(p, testMember)
	})

	require.NoError(t, err)
	assert.Equal(t, 0, store.sets)
}

func TestUpdateProjectPolicy_PermanentFailureIsNotRetried(t *testing.T) {
	withNoIAMRetryDelay(t)
	denied := &googleapi.Error{Code: 403, Message: "denied"}
	store := &fakePolicies{
		policy:  policyWith(binding("roles/a", testMember)),
		setErrs: []error{denied},
	}

	_, err := updateProjectPolicy(store, "proj", isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return removeMemberFromPolicy(p, testMember)
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, denied)
	assert.Equal(t, 1, store.sets)
}

func TestUpdateProjectPolicy_GetFailureIsReturned(t *testing.T) {
	withNoIAMRetryDelay(t)
	store := &fakePolicies{
		policy:  policyWith(),
		getErrs: []error{&googleapi.Error{Code: 403}},
	}

	_, err := updateProjectPolicy(store, "proj", isTransientIAMError, func(*cloudresourcemanager.Policy) bool {
		t.Fatal("mutate must not run when the policy could not be read")
		return false
	})

	require.Error(t, err)
}

func TestUpdateProjectPolicy_GivesUpAfterTheAttemptLimit(t *testing.T) {
	withNoIAMRetryDelay(t)
	conflict := &googleapi.Error{Code: 409}
	store := &fakePolicies{policy: policyWith(binding("roles/a", testMember))}
	for i := 0; i < iamPolicyAttempts+5; i++ {
		store.setErrs = append(store.setErrs, conflict)
	}

	_, err := updateProjectPolicy(store, "proj", isTransientIAMError, func(p *cloudresourcemanager.Policy) bool {
		return removeMemberFromPolicy(p, testMember)
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, conflict)
	assert.Equal(t, iamPolicyAttempts, store.sets)
}

func TestIsNotFoundError(t *testing.T) {
	assert.True(t, isNotFoundError(&googleapi.Error{Code: 404}))
	assert.True(t, isNotFoundError(fmt.Errorf("wrapped: %w", &googleapi.Error{Code: 404})))
	assert.True(t, isNotFoundError(status.Error(codes.NotFound, "gone")))

	assert.False(t, isNotFoundError(nil))
	assert.False(t, isNotFoundError(&googleapi.Error{Code: 403}))
	assert.False(t, isNotFoundError(&googleapi.Error{Code: 500, Message: "backend 404 while proxying"}))
	assert.False(t, isNotFoundError(status.Error(codes.PermissionDenied, "denied")))

	// text alone is not a not-found: these used to read as absence
	assert.False(t, isNotFoundError(errors.New("googleapi: Error 403: project 4040404 Not Found in policy")))
	assert.False(t, isNotFoundError(errors.New("notFound")))
}

func TestIsTransientIAMError(t *testing.T) {
	assert.True(t, isTransientIAMError(&googleapi.Error{Code: 409}))
	assert.True(t, isTransientIAMError(&googleapi.Error{Code: 429}))
	assert.True(t, isTransientIAMError(&googleapi.Error{Code: 503}))
	assert.True(t, isTransientIAMError(status.Error(codes.Aborted, "etag")))

	assert.False(t, isTransientIAMError(&googleapi.Error{Code: 403}))
	assert.False(t, isTransientIAMError(&googleapi.Error{Code: 400}))
	assert.False(t, isTransientIAMError(errors.New("plain")))
}

func TestConfirmServiceAccountGone(t *testing.T) {
	withNoIAMRetryDelay(t)

	t.Run("a not found confirms it", func(t *testing.T) {
		require.NoError(t, confirmServiceAccountGone(func() error { return &googleapi.Error{Code: 404} }))
	})

	t.Run("a read that lags behind the delete is waited out", func(t *testing.T) {
		reads := 0
		err := confirmServiceAccountGone(func() error {
			reads++
			if reads < 3 {
				return nil
			}
			return &googleapi.Error{Code: 404}
		})
		require.NoError(t, err)
		assert.Equal(t, 3, reads)
	})

	t.Run("an account that stays is an error", func(t *testing.T) {
		require.Error(t, confirmServiceAccountGone(func() error { return nil }))
	})

	t.Run("a failed read is not taken as gone", func(t *testing.T) {
		require.Error(t, confirmServiceAccountGone(func() error { return &googleapi.Error{Code: 403} }))
	})
}

func TestGcpServiceAccountDeletable_UnownedIsNotOwnedError(t *testing.T) {
	account := &iam.ServiceAccount{Email: "other@proj.iam.gserviceaccount.com", Description: "someone else"}
	_, err := gcpServiceAccountDeletable(account, true, "mine", account.Email)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGcpServiceAccountNotOwned)
}

func TestGcpCleanupError_UnwrapsToCause(t *testing.T) {
	cause := &googleapi.Error{Code: 403}
	var err error = &GcpCleanupError{ServiceAccountEmail: "a@p", ProjectID: "p", Err: cause}

	var cleanupErr *GcpCleanupError
	require.ErrorAs(t, fmt.Errorf("outer: %w", err), &cleanupErr)
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "infrastructure destroyed")
}
