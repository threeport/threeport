package provider

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ SettingsLifecycleProvider = (*fakeSettingsLifecycle)(nil)

// fakeSettingsLifecycle is a fakeUpgradeLifecycle whose definition also has
// settings its cluster may not run.  Applying them records them, so none is
// pending after.
type fakeSettingsLifecycle struct {
	*fakeUpgradeLifecycle

	settingsMu sync.Mutex
	pending    []string
	applied    int
}

// PendingSettings returns the settings the cluster doesn't run.
func (f *fakeSettingsLifecycle) PendingSettings() ([]string, error) {
	f.settingsMu.Lock()
	defer f.settingsMu.Unlock()
	return append([]string(nil), f.pending...), f.recordSimple("PendingSettings")
}

// OnSettingsApplied records the settings as run.
func (f *fakeSettingsLifecycle) OnSettingsApplied(_ InfraProvider) error {
	f.settingsMu.Lock()
	defer f.settingsMu.Unlock()
	f.pending = nil
	f.applied++
	return f.recordSimple("OnSettingsApplied")
}

// appliedCount returns how often a step recorded the settings it applied.
func (f *fakeSettingsLifecycle) appliedCount() int {
	f.settingsMu.Lock()
	defer f.settingsMu.Unlock()
	return f.applied
}

// newFakeSettingsLifecycle returns a fake whose cluster runs version, as its
// definition asks, and not the settings pending.
func newFakeSettingsLifecycle(infra *fakeUpgradeInfra, version string, pending ...string) *fakeSettingsLifecycle {
	fl := newFakeUpgradeLifecycle(infra, version)
	fl.setUpgrade(UpgradeSnapshot{RecordedVersion: version})

	return &fakeSettingsLifecycle{fakeUpgradeLifecycle: fl, pending: pending}
}

// TestSettingsAClusterDoesntRunAreApplied covers a cluster on its
// definition's version that doesn't run settings the definition asks for: a
// step on its version applies them, records them, and says so, and the next
// update finds nothing to do.
func TestSettingsAClusterDoesntRunAreApplied(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeSettingsLifecycle(infra, "1.36", "SecureBoot", "BootDiskKmsKey")

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(upgradeRequeueSeconds), requeue)
	assert.Equal(t, []string{"upgrading to the definition's SecureBoot, BootDiskKmsKey on Kubernetes 1.36"}, fl.acked)

	outcome := fl.waitFinished(t)
	assert.Equal(t, UpgradeOutcome{
		Version: "1.36",
		Status:  "upgraded to the definition's SecureBoot, BootDiskKmsKey on Kubernetes 1.36",
	}, outcome)
	assert.Equal(t, []string{"1.36"}, infra.upgrades(), "on the version it runs")
	assert.Equal(t, 1, fl.appliedCount())
	assert.Equal(t, 1, infra.setStackStateCallCount(), "the recorded stack is restored first")

	requeue, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Len(t, infra.upgrades(), 1, "nothing left to apply")
}

// TestAClusterThatRunsItsSettingsIsLeftAsItIs covers a cluster that runs its
// definition's version and settings: nothing is applied.
func TestAClusterThatRunsItsSettingsIsLeftAsItIs(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeSettingsLifecycle(infra, "1.36")

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("PendingSettings"))
	assert.Empty(t, infra.upgrades())
	assert.Empty(t, fl.acked)
}

// TestAnUpgradeStepRecordsTheSettingsItApplied covers a step that upgrades
// the version: it applies the definition's settings too, and records them.
func TestAnUpgradeStepRecordsTheSettingsItApplied(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	fl := newFakeSettingsLifecycle(infra, "1.36", "IntranodeVisibility")
	fl.setUpgrade(UpgradeSnapshot{RecordedVersion: "1.35"})

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, []string{"upgrading from Kubernetes 1.35 to 1.36"}, fl.acked)
	outcome := fl.waitFinished(t)
	assert.Equal(t, "upgraded from Kubernetes 1.35 to 1.36", outcome.Status)
	assert.Equal(t, 1, fl.appliedCount())
}

// TestRefusedSettingsWaitForTheDefinition covers a step applying settings
// whose preview replaces or deletes what it may not: the refusal is
// recorded, failed too so the work is still there to do, and isn't tried
// again until it is cleared, as the definition's next update does.
func TestRefusedSettingsWaitForTheDefinition(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	infra.upgradeErr = &UpgradeRefusedError{Steps: []string{"delete gcp:container/nodePool:NodePool workers"}}
	fl := newFakeSettingsLifecycle(infra, "1.36", "SecureBoot")

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Refused)
	assert.True(t, outcome.Failed)
	assert.Contains(t, outcome.Status, "refused to upgrade to the definition's SecureBoot on Kubernetes 1.36")
	assert.Zero(t, fl.appliedCount())

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Len(t, infra.upgrades(), 1, "a refusal waits")

	// the definition's next update clears it
	infra.upgradeErr = nil
	snapshot, err := fl.GetUpgrade()
	require.NoError(t, err)
	snapshot.Refused, snapshot.Failed, snapshot.Upgrading = false, false, false
	fl.setUpgrade(*snapshot)
	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(fl.finishedOutcomes()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, 1, fl.appliedCount())
}

// TestFailedSettingsAreTriedAgain covers a step applying settings that
// fails: it is recorded as failed, and run again on the cluster's version
// once the retry delay is over, applying them then.
func TestFailedSettingsAreTriedAgain(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	restore := setLifecycleClock(clock)
	defer restore()

	infra := newFakeUpgradeInfra("1.36")
	infra.upgradeErr = errors.New("the node pool update failed")
	fl := newFakeSettingsLifecycle(infra, "1.36", "BootDiskType")

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Failed)
	assert.Equal(t, "failed to upgrade to the definition's BootDiskType on Kubernetes 1.36: the node pool update failed", outcome.Status)

	infra.upgradeErr = nil
	clock.Advance(upgradeRetryDelay + time.Second)
	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(fl.finishedOutcomes()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.False(t, fl.finishedOutcomes()[1].Failed)
	assert.Equal(t, 1, fl.appliedCount())
}

// TestSettingsArentAppliedWithoutAStack covers a cluster Threeport didn't
// create: it has no stack to apply settings with, so none is applied.
func TestSettingsArentAppliedWithoutAStack(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeSettingsLifecycle(infra, "1.36", "SecureBoot")
	fl.snaps[0].ResourceInventory = nil

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Empty(t, infra.upgrades())
}
