package provider

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	event "github.com/threeport/threeport/pkg/event/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// compile-time interface checks for the upgrade fakes and the providers
var (
	_ UpgradeLifecycleProvider = (*fakeUpgradeLifecycle)(nil)
	_ UpgradeableInfra         = (*fakeUpgradeInfra)(nil)
	_ UpgradeableInfra         = (*KubernetesRuntimeInfraEKS)(nil)
	_ UpgradeableInfra         = (*KubernetesRuntimeInfraGKE)(nil)
)

// fakeUpgradeLifecycle is a fakeLifecycle whose instance's cluster can be
// upgraded.
type fakeUpgradeLifecycle struct {
	*fakeLifecycle

	upgradeMu sync.Mutex
	upgrade   UpgradeSnapshot
	target    string

	// The statuses passed to RecordUpgradeStatus and AckUpgrade
	recorded []string
	acked    []string
	// The outcomes passed to FinishUpgrade
	finished []UpgradeOutcome
}

// newFakeUpgradeLifecycle returns a fake whose creation is confirmed and whose
// stack is recorded, with a definition asking for target.
func newFakeUpgradeLifecycle(infra *fakeUpgradeInfra, target string) *fakeUpgradeLifecycle {
	fl := newFakeLifecycle(&ReconciliationSnapshot{
		CreationConfirmed: util.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		ResourceInventory: validStackState(),
	})
	fl.infra = infra

	return &fakeUpgradeLifecycle{fakeLifecycle: fl, target: target}
}

// GetUpgrade returns the fake's upgrade state.
func (f *fakeUpgradeLifecycle) GetUpgrade() (*UpgradeSnapshot, error) {
	if err := f.recordSimple("GetUpgrade"); err != nil {
		return nil, err
	}
	f.upgradeMu.Lock()
	defer f.upgradeMu.Unlock()
	snapshot := f.upgrade
	return &snapshot, nil
}

// GetUpgradeTarget returns the version the fake's definition asks for.
func (f *fakeUpgradeLifecycle) GetUpgradeTarget() (string, error) {
	return f.target, f.recordSimple("GetUpgradeTarget")
}

// RecordUpgradeStatus records the version and status.
func (f *fakeUpgradeLifecycle) RecordUpgradeStatus(version, status string) error {
	f.upgradeMu.Lock()
	f.recorded = append(f.recorded, status)
	f.upgrade.RecordedVersion = version
	f.upgrade.Status = status
	f.upgradeMu.Unlock()
	return f.recordSimple("RecordUpgradeStatus")
}

// AckUpgrade records that an upgrade started.
func (f *fakeUpgradeLifecycle) AckUpgrade(status string) error {
	f.upgradeMu.Lock()
	f.acked = append(f.acked, status)
	f.upgrade.Upgrading, f.upgrade.Failed, f.upgrade.Refused = true, false, false
	f.upgrade.Acknowledged = util.Ptr(currentClock().Now().UTC())
	f.upgrade.Status = status
	f.upgradeMu.Unlock()
	return f.recordSimple("AckUpgrade")
}

// RefreshUpgradeAck records the call.
func (f *fakeUpgradeLifecycle) RefreshUpgradeAck() error {
	return f.recordSimple("RefreshUpgradeAck")
}

// FinishUpgrade records how the upgrade ended.
func (f *fakeUpgradeLifecycle) FinishUpgrade(outcome UpgradeOutcome) error {
	f.upgradeMu.Lock()
	f.finished = append(f.finished, outcome)
	f.upgrade.Upgrading, f.upgrade.Failed, f.upgrade.Refused = false, outcome.Failed, outcome.Refused
	if outcome.Version != "" {
		f.upgrade.RecordedVersion = outcome.Version
	}
	f.upgrade.Status = outcome.Status
	f.upgradeMu.Unlock()
	return f.recordSimple("FinishUpgrade")
}

// OnUpgraded records the call.
func (f *fakeUpgradeLifecycle) OnUpgraded(_ InfraProvider) error {
	return f.recordSimple("OnUpgraded")
}

// setUpgrade sets the fake's upgrade state.
func (f *fakeUpgradeLifecycle) setUpgrade(snapshot UpgradeSnapshot) {
	f.upgradeMu.Lock()
	defer f.upgradeMu.Unlock()
	f.upgrade = snapshot
}

// finishedOutcomes returns the outcomes recorded so far.
func (f *fakeUpgradeLifecycle) finishedOutcomes() []UpgradeOutcome {
	f.upgradeMu.Lock()
	defer f.upgradeMu.Unlock()
	return append([]UpgradeOutcome(nil), f.finished...)
}

// waitFinished waits for the upgrade running in the background to record its
// outcome, and returns it.
func (f *fakeUpgradeLifecycle) waitFinished(t *testing.T) UpgradeOutcome {
	t.Helper()
	require.Eventually(t, func() bool { return len(f.finishedOutcomes()) > 0 }, 5*time.Second, 5*time.Millisecond)
	return f.finishedOutcomes()[0]
}

// fakeUpgradeInfra is a fakeInfra whose cluster runs a version and can be
// upgraded.
type fakeUpgradeInfra struct {
	*fakeInfra

	versionMu sync.Mutex
	version   string
	// The error UpgradeInfra returns
	upgradeErr error
	// Whether UpgradeInfra leaves the cluster on its version
	upgradeIgnored bool
	// The versions passed to UpgradeInfra
	upgradedTo []string
}

// newFakeUpgradeInfra returns a fake whose cluster runs version.
func newFakeUpgradeInfra(version string) *fakeUpgradeInfra {
	return &fakeUpgradeInfra{fakeInfra: newFakeInfra(), version: version}
}

// ClusterKubernetesVersion returns the version the fake cluster runs.
func (f *fakeUpgradeInfra) ClusterKubernetesVersion() (string, error) {
	f.versionMu.Lock()
	defer f.versionMu.Unlock()
	return f.version, nil
}

// UpgradeInfra records the version and moves the fake cluster to it, unless
// set to fail or to be ignored.
func (f *fakeUpgradeInfra) UpgradeInfra(version string) error {
	f.versionMu.Lock()
	defer f.versionMu.Unlock()
	f.upgradedTo = append(f.upgradedTo, version)
	if f.upgradeErr != nil {
		return f.upgradeErr
	}
	if !f.upgradeIgnored {
		f.version = version
	}
	return nil
}

// upgrades returns the versions passed to UpgradeInfra.
func (f *fakeUpgradeInfra) upgrades() []string {
	f.versionMu.Lock()
	defer f.versionMu.Unlock()
	return append([]string(nil), f.upgradedTo...)
}

// TestAClusterOnItsDefinitionsVersionIsLeftAsItIs covers an update of a
// cluster that runs the version its definition asks for: the version is
// recorded once, and nothing is upgraded.
func TestAClusterOnItsDefinitionsVersionIsLeftAsItIs(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeUpgradeLifecycle(infra, "1.36")

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Equal(t, 1, fl.callCount("RecordUpgradeStatus"), "the version it runs is recorded")
	assert.Equal(t, 0, fl.callCount("AckUpgrade"))

	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, fl.callCount("RecordUpgradeStatus"), "a write that changes nothing would notify again")
	assert.Empty(t, infra.upgrades())
}

// TestAClusterTheProviderUpgradedIsntDowngraded covers a cluster the
// provider upgraded past its definition's version, as EKS does when standard
// support ends: it is recorded as it is, with why, and left alone.
func TestAClusterTheProviderUpgradedIsntDowngraded(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeUpgradeLifecycle(infra, "1.35")

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	snapshot, err := fl.GetUpgrade()
	require.NoError(t, err)
	assert.Equal(t, "1.36", snapshot.RecordedVersion)
	assert.Equal(t, "the cluster runs Kubernetes 1.36, newer than its definition's 1.35, and isn't downgraded", snapshot.Status)
	assert.Empty(t, infra.upgrades())
	assert.Equal(t, 0, fl.callCount("AckUpgrade"))
}

// TestAnOlderClusterIsUpgradedOneMinorVersion covers a cluster older than its
// definition's version: one step moves it one minor version, on its recorded
// stack, and records the version it then runs.
func TestAnOlderClusterIsUpgradedOneMinorVersion(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	fl := newFakeUpgradeLifecycle(infra, "1.37")

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(upgradeRequeueSeconds), requeue)
	assert.Equal(t, []string{"upgrading from Kubernetes 1.35 to 1.36"}, fl.acked)

	outcome := fl.waitFinished(t)
	assert.Equal(t, UpgradeOutcome{Version: "1.36", Status: "upgraded from Kubernetes 1.35 to 1.36"}, outcome)
	assert.Equal(t, []string{"1.36"}, infra.upgrades(), "one minor version per step")
	assert.Equal(t, 1, infra.setStackStateCallCount(), "the recorded stack is restored first")
	assert.Equal(t, 1, fl.callCount("SaveState"), "and its state recorded after")
	assert.Equal(t, 1, fl.callCount("OnUpgraded"))
	assert.Empty(t, fl.recordedWarnings())

	// the instance update the step recorded starts the next one
	requeue, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(upgradeRequeueSeconds), requeue)
	require.Eventually(t, func() bool { return len(fl.finishedOutcomes()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"1.36", "1.37"}, infra.upgrades())

	requeue, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Len(t, infra.upgrades(), 2, "done once it runs the definition's version")
}

// TestAnUpgradeThatWouldReplaceResourcesIsRefused covers an upgrade whose
// preview replaces or deletes resources: nothing is changed, the refusal is
// recorded with what it would have replaced, and it isn't tried again.
func TestAnUpgradeThatWouldReplaceResourcesIsRefused(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	infra.upgradeErr = &UpgradeRefusedError{Steps: []string{"replace aws:eks/nodeGroup:NodeGroup workers"}}
	fl := newFakeUpgradeLifecycle(infra, "1.36")

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Refused)
	assert.False(t, outcome.Failed)
	assert.Equal(t, "1.35", outcome.Version)
	assert.Contains(t, outcome.Status, "refused to upgrade from Kubernetes 1.35 to 1.36")
	assert.Contains(t, outcome.Status, "replace aws:eks/nodeGroup:NodeGroup workers")
	assert.Equal(t, 0, fl.callCount("OnUpgraded"))
	require.Len(t, fl.recordedWarnings(), 1)
	assert.Equal(t, event.ReasonUpgradeRefused, fl.recordedWarnings()[0].reason)

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	assert.Len(t, infra.upgrades(), 1, "a refused upgrade waits for another version to be asked for")
}

// TestAFailedUpgradeIsTriedAgainAfterAWait covers an upgrade that fails: the
// failure is recorded, and the next attempt waits for the retry delay.
func TestAFailedUpgradeIsTriedAgainAfterAWait(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	restore := setLifecycleClock(clock)
	defer restore()

	infra := newFakeUpgradeInfra("1.35")
	infra.upgradeErr = errors.New("the control plane update failed")
	fl := newFakeUpgradeLifecycle(infra, "1.36")

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Failed)
	assert.Equal(t, "failed to upgrade from Kubernetes 1.35 to 1.36: the control plane update failed", outcome.Status)
	require.Len(t, fl.recordedWarnings(), 1)
	assert.Equal(t, event.ReasonUpgradeFailed, fl.recordedWarnings()[0].reason)

	clock.Advance(5 * time.Minute)
	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(10*60+1), requeue, "the rest of the wait")
	assert.Len(t, infra.upgrades(), 1)

	clock.Advance(10 * time.Minute)
	infra.versionMu.Lock()
	infra.upgradeErr = nil
	infra.versionMu.Unlock()
	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(fl.finishedOutcomes()) == 2 }, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, "upgraded from Kubernetes 1.35 to 1.36", fl.finishedOutcomes()[1].Status)
}

// TestAnUpgradeTheClusterDidntTakeFails covers an up that succeeds while the
// cluster still runs its version: the step isn't counted done.
func TestAnUpgradeTheClusterDidntTakeFails(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	infra.upgradeIgnored = true
	fl := newFakeUpgradeLifecycle(infra, "1.36")

	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Failed)
	assert.Contains(t, outcome.Status, "the cluster still runs Kubernetes 1.35 after the upgrade")
	assert.Equal(t, 0, fl.callCount("OnUpgraded"))
}

// TestAClusterThreeportDidntCreateIsntUpgraded covers a cluster with no
// recorded stack, as one registered with --control-plane-only has: it is
// refused, since there is nothing to upgrade it with.
func TestAClusterThreeportDidntCreateIsntUpgraded(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	fl := newFakeUpgradeLifecycle(infra, "1.36")
	fl.snaps = []*ReconciliationSnapshot{{CreationConfirmed: util.Ptr(time.Now())}}

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	require.Len(t, fl.finishedOutcomes(), 1)
	assert.True(t, fl.finishedOutcomes()[0].Refused)
	assert.Contains(t, fl.finishedOutcomes()[0].Status, "Threeport didn't create this cluster")
	assert.Empty(t, infra.upgrades())
	assert.Equal(t, 0, fl.callCount("AckUpgrade"))
}

// TestARunningUpgradeIsLeftToFinish covers updates that arrive while an
// upgrade runs, and after it was interrupted.
func TestARunningUpgradeIsLeftToFinish(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	restore := setLifecycleClock(clock)
	defer restore()

	infra := newFakeUpgradeInfra("1.35")
	fl := newFakeUpgradeLifecycle(infra, "1.36")
	fl.setUpgrade(UpgradeSnapshot{Upgrading: true, Acknowledged: util.Ptr(clock.Now())})

	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(upgradeRequeueSeconds), requeue)
	assert.Equal(t, 0, fl.callCount("BuildInfra"))

	// an acknowledgement nobody refreshes is an interrupted upgrade
	clock.Advance(currentConfig().StaleAckThreshold + time.Second)
	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	fl.waitFinished(t)
	assert.Equal(t, []string{"1.36"}, infra.upgrades())
}

// TestADeleteWaitsForARunningUpgrade covers a delete that arrives while an
// upgrade runs on the same stack.
func TestADeleteWaitsForARunningUpgrade(t *testing.T) {
	infra := newFakeUpgradeInfra("1.35")
	fl := newFakeUpgradeLifecycle(infra, "1.36")
	fl.snaps = []*ReconciliationSnapshot{{
		CreationConfirmed: util.Ptr(time.Now().UTC()),
		DeletionScheduled: util.Ptr(time.Now().UTC()),
		ResourceInventory: validStackState(),
	}}
	fl.setUpgrade(UpgradeSnapshot{Upgrading: true, Acknowledged: util.Ptr(time.Now().UTC())})

	requeue, err := HandleInfraDelete(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(60), requeue)
	assert.Equal(t, 0, fl.callCount("AckDeletion"))

	// once it is done, the delete goes ahead
	fl.setUpgrade(UpgradeSnapshot{Acknowledged: util.Ptr(time.Now().UTC())})
	_, err = HandleInfraDelete(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, fl.callCount("AckDeletion"))
	require.Eventually(t, func() bool { return infra.destroyCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return lifecycleGoroutinesRemaining(0) == 0 }, 5*time.Second, 5*time.Millisecond)
}

// TestAPreviewsReplacementsAndDeletionsAreNamed covers reading the steps an
// upgrade refuses from preview events: each resource replaced or deleted,
// once, and nothing created, updated or left the same.
func TestAPreviewsReplacementsAndDeletionsAreNamed(t *testing.T) {
	step := func(op apitype.OpType, resourceType, name string) events.EngineEvent {
		return events.EngineEvent{EngineEvent: apitype.EngineEvent{ResourcePreEvent: &apitype.ResourcePreEvent{
			Metadata: apitype.StepEventMetadata{
				Op:   op,
				Type: resourceType,
				URN:  "urn:pulumi:stack::project::" + resourceType + "::" + name,
			},
		}}}
	}

	steps := destructiveSteps([]events.EngineEvent{
		step(apitype.OpSame, "aws:ec2/vpc:Vpc", "vpc"),
		step(apitype.OpUpdate, "aws:eks/cluster:Cluster", "cluster"),
		step(apitype.OpCreate, "aws:ec2/subnet:Subnet", "new-subnet"),
		step(apitype.OpCreateReplacement, "aws:eks/nodeGroup:NodeGroup", "workers"),
		step(apitype.OpReplace, "aws:eks/nodeGroup:NodeGroup", "workers"),
		step(apitype.OpDeleteReplaced, "aws:eks/nodeGroup:NodeGroup", "workers"),
		step(apitype.OpDelete, "aws:ec2/natGateway:NatGateway", "nat"),
		{EngineEvent: apitype.EngineEvent{SummaryEvent: &apitype.SummaryEvent{}}},
	})
	assert.Equal(t, []string{
		"delete aws:ec2/natGateway:NatGateway nat",
		"replace aws:eks/nodeGroup:NodeGroup workers",
	}, steps)

	assert.Empty(t, destructiveSteps([]events.EngineEvent{step(apitype.OpUpdate, "aws:eks/cluster:Cluster", "cluster")}))
	assert.EqualError(t, &UpgradeRefusedError{Steps: steps},
		"the upgrade would replace or delete resources, so nothing was changed: delete aws:ec2/natGateway:NatGateway nat, replace aws:eks/nodeGroup:NodeGroup workers")
}

// TestTheLowerOfTwoVersionsIsTheClusters covers the version an EKS cluster
// reports: the lower of its control plane's and its node group's.
func TestTheLowerOfTwoVersionsIsTheClusters(t *testing.T) {
	for _, c := range [][3]string{{"1.36", "1.35", "1.35"}, {"1.35", "1.36", "1.35"}, {"1.36", "1.36", "1.36"}} {
		lower, err := lowerKubernetesVersion(c[0], c[1])
		require.NoError(t, err)
		assert.Equal(t, c[2], lower, "%s and %s", c[0], c[1])
	}
	_, err := lowerKubernetesVersion("1.36", "v1.35")
	assert.Error(t, err)

	assert.Equal(t, "1.36", minorVersion("1.36.4-gke.2046003"), "a GKE release")
}

// TestAnUpgradeThatReachedItsVersionFinishesItsStep covers an upgrade left
// unrecorded after the cluster reached its definition's version: one
// interrupted before it recorded its outcome, and one whose last step failed,
// as it does when an add-on EKS moves after the node group fails.  The step
// runs again on the version the cluster runs, so its last work is done before
// the upgrade is recorded as finished.
func TestAnUpgradeThatReachedItsVersionFinishesItsStep(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	restore := setLifecycleClock(clock)
	defer restore()
	stale := util.Ptr(clock.Now().Add(-time.Hour))

	for name, snapshot := range map[string]UpgradeSnapshot{
		"interrupted": {Upgrading: true, Acknowledged: stale, RecordedVersion: "1.35", Status: "upgrading from Kubernetes 1.35 to 1.36"},
		"failed":      {Failed: true, Acknowledged: stale, RecordedVersion: "1.35", Status: "failed to upgrade"},
	} {
		t.Run(name, func(t *testing.T) {
			infra := newFakeUpgradeInfra("1.36")
			fl := newFakeUpgradeLifecycle(infra, "1.36")
			fl.setUpgrade(snapshot)

			_, err := HandleInfraUpdate(fl, newTestLogger())
			require.NoError(t, err)
			assert.Equal(t, []string{"finishing the upgrade to Kubernetes 1.36"}, fl.acked)
			assert.Equal(t, UpgradeOutcome{Version: "1.36", Status: "upgraded to Kubernetes 1.36"}, fl.waitFinished(t))
			assert.Equal(t, []string{"1.36"}, infra.upgrades(), "the step again, on the version the cluster runs")
			assert.Equal(t, 1, fl.callCount("OnUpgraded"))

			upgrade, err := fl.GetUpgrade()
			require.NoError(t, err)
			assert.False(t, upgrade.Upgrading || upgrade.Failed || upgrade.Refused)

			// and then the cluster is left as it is
			_, err = HandleInfraUpdate(fl, newTestLogger())
			require.NoError(t, err)
			assert.Len(t, infra.upgrades(), 1)
		})
	}

	// work that fails again leaves the upgrade failed, to be tried again
	infra := newFakeUpgradeInfra("1.36")
	infra.upgradeErr = errors.New("the coredns add-on update failed")
	fl := newFakeUpgradeLifecycle(infra, "1.36")
	fl.setUpgrade(UpgradeSnapshot{Failed: true, Acknowledged: stale, RecordedVersion: "1.35"})
	_, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	outcome := fl.waitFinished(t)
	assert.True(t, outcome.Failed)
	assert.Equal(t, "failed to upgrade to Kubernetes 1.36: the coredns add-on update failed", outcome.Status)
	assert.Equal(t, 0, fl.callCount("OnUpgraded"))
}

// TestAnUpgradeTheProviderFinishedIsRecorded covers a refused upgrade the
// provider has since done on its own, and one the provider took past the
// definition's version: each is finished without running a step, with its
// flags cleared and the post-upgrade work run.  A step refused on the version
// the cluster still runs stays refused.
func TestAnUpgradeTheProviderFinishedIsRecorded(t *testing.T) {
	infra := newFakeUpgradeInfra("1.36")
	fl := newFakeUpgradeLifecycle(infra, "1.36")
	fl.setUpgrade(UpgradeSnapshot{Refused: true, RecordedVersion: "1.35", Status: "refused to upgrade"})
	requeue, err := HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(0), requeue)
	require.Len(t, fl.finishedOutcomes(), 1)
	assert.Equal(t, UpgradeOutcome{Version: "1.36", Status: "upgraded to Kubernetes 1.36"}, fl.finishedOutcomes()[0])
	assert.Equal(t, 1, fl.callCount("OnUpgraded"))
	assert.Empty(t, infra.upgrades())
	upgrade, err := fl.GetUpgrade()
	require.NoError(t, err)
	assert.False(t, upgrade.Upgrading || upgrade.Failed || upgrade.Refused)

	infra = newFakeUpgradeInfra("1.36")
	fl = newFakeUpgradeLifecycle(infra, "1.36")
	fl.setUpgrade(UpgradeSnapshot{Refused: true, RecordedVersion: "1.36", Status: "refused to upgrade to Kubernetes 1.36"})
	_, err = HandleInfraUpdate(fl, newTestLogger())
	require.NoError(t, err)
	assert.Empty(t, fl.finishedOutcomes())
	assert.Empty(t, infra.upgrades())
	upgrade, err = fl.GetUpgrade()
	require.NoError(t, err)
	assert.True(t, upgrade.Refused, "refused until the definition asks for another version")

	// one the provider took past the definition's version keeps saying so
	for name, snapshot := range map[string]UpgradeSnapshot{
		"refused": {Refused: true, RecordedVersion: "1.35"},
		"failed":  {Failed: true, RecordedVersion: "1.35"},
	} {
		t.Run(name, func(t *testing.T) {
			infra := newFakeUpgradeInfra("1.37")
			fl := newFakeUpgradeLifecycle(infra, "1.36")
			fl.setUpgrade(snapshot)
			_, err := HandleInfraUpdate(fl, newTestLogger())
			require.NoError(t, err)
			require.Len(t, fl.finishedOutcomes(), 1)
			assert.Equal(t, "1.37", fl.finishedOutcomes()[0].Version)
			assert.Contains(t, fl.finishedOutcomes()[0].Status, "newer than its definition's 1.36")
			assert.Empty(t, infra.upgrades())
		})
	}
}
