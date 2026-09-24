package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// serializationTestPrefix names the objects this test creates so a failed run
// leaves something identifiable behind.
const serializationTestPrefix = "serialization-retry-test"

// TestConcurrentUpdatesAbsorbSerializationFailures covers what #471 asks for:
// not that SQLSTATE 40001 stops happening, but that the handlers absorb it.
//
// Under SERIALIZABLE isolation CockroachDB answers a write conflict by telling
// the client to re-run the transaction. Before the handlers wrapped their
// writes in crdbgorm.ExecuteTx, that came back to the caller as a 500 carrying
// "database serialization conflict". Hammering one row from several goroutines
// is the cheapest way to provoke the conflict; every request should still
// succeed, because the handler retries internally.
//
// A unit test cannot reach this. The retry path only runs when CockroachDB
// actually reports a conflict, which needs a real database under real
// contention, and the savepoint the retry rides on is CockroachDB-only syntax.
func TestConcurrentUpdatesAbsorbSerializationFailures(t *testing.T) {
	cli.InitConfig(nil, "")

	threeportConfig, _, err := cli.GetThreeportConfig("")
	require.Nil(t, err, "should have no error getting threeport config")
	apiClient, err := threeportConfig.GetHTTPClient(threeportConfig.CurrentControlPlane)
	require.Nil(t, err, "should have no error creating http client")
	controlPlaneConfig, err := threeportConfig.GetControlPlaneConfig(threeportConfig.CurrentControlPlane)
	require.Nil(t, err, "should not get an error looking up Threeport API endpoint")
	apiEndpoint := controlPlaneConfig.APIServer

	// arrange: one row for every writer to fight over. Spreading the writes
	// across several rows would let them commit without conflicting, and the
	// test would pass whether or not the retry works.
	profile, err := client.CreateProfile(apiClient, apiEndpoint, &v0.Profile{
		Name: util.Ptr(fmt.Sprintf("%s-profile", serializationTestPrefix)),
	})
	require.Nil(t, err, "should have no error creating profile")
	defer retryDelete(t, fmt.Sprintf("profile %d", *profile.ID), func() error {
		_, err := client.DeleteProfile(apiClient, apiEndpoint, *profile.ID)
		return err
	})

	// a single round of concurrent writes is not enough: CockroachDB resolves
	// contention on one row by making writers wait on the intent rather than
	// aborting, so the requests serialize without ever reporting 40001. Each
	// writer loops instead, so attempts keep overlapping long enough for a
	// read refresh to fail and the retry path to run.
	// tuned to contend, not to exhaust. This shape drives around ninety
	// transaction restarts through one row per run, measured on
	// crdb_internal.node_metrics 'txn.restarts.writetooold', and the handler
	// absorbs all of them: nothing reaches the client. Twenty writers of
	// twenty-five rounds instead pushes past the library's fifty-attempt cap
	// and the handler gives up, which is the pathological case #471 leaves to
	// the reconciler's requeue backstop rather than the retry.
	const writers = 6
	const roundsPerWriter = 6

	errs := make([]error, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			for round := 0; round < roundsPerWriter; round++ {
				_, updateErr := client.UpdateProfile(apiClient, apiEndpoint, &v0.Profile{
					Common: v0.Common{ID: profile.ID},
					Name:   util.Ptr(fmt.Sprintf("%s-profile-%d-%d", serializationTestPrefix, n, round)),
				})
				if updateErr != nil {
					errs[n] = updateErr
					return
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()

	// assert: no writer saw a serialization failure, and none failed at all.
	// The message is the one sanitizeInternalError produces for a 40001, so a
	// regression here names itself.
	for n, updateErr := range errs {
		if updateErr != nil && strings.Contains(updateErr.Error(), "database serialization conflict") {
			t.Fatalf("writer %d received a serialization conflict the handler should have retried: %v", n, updateErr)
		}
		assert.NoError(t, updateErr, "writer %d should have completed", n)
	}

	// the row survives as one object carrying one of the writers' names
	final, err := client.GetProfileByID(apiClient, apiEndpoint, *profile.ID)
	require.Nil(t, err, "should have no error reading the profile back")
	require.NotNil(t, final.Name)
	assert.True(
		t,
		strings.HasPrefix(*final.Name, serializationTestPrefix),
		"the surviving name should come from one of the writers, got %q", *final.Name,
	)
}
