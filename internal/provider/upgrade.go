package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"gorm.io/datatypes"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
)

// upgradeRequeueSeconds is how long an update waits while an upgrade runs
// before it looks again.  It matches the create requeue.
const upgradeRequeueSeconds = 120

// upgradeRetryDelay is how long the controller waits after a failed upgrade
// before it tries again.
const upgradeRetryDelay = 15 * time.Minute

// UpgradeSnapshot is an instance's upgrade state.
type UpgradeSnapshot struct {
	// The version the instance records its cluster runs, "" when none is
	// recorded.
	RecordedVersion string

	// When the last upgrade started, refreshed while it runs.
	Acknowledged *time.Time

	// Whether an upgrade is running.
	Upgrading bool

	// Whether the last upgrade failed.
	Failed bool

	// Whether the controller refused to upgrade the cluster.
	Refused bool

	// What the controller last recorded about upgrading the cluster.
	Status string
}

// UpgradeOutcome is how an upgrade ended, for the instance to record.
type UpgradeOutcome struct {
	// The version the cluster runs, recorded when it is known.
	Version string

	// Whether the upgrade failed, and is tried again after a wait.
	Failed bool

	// Whether the upgrade was refused, and isn't tried again until the
	// definition's version changes.
	Refused bool

	// What happened.
	Status string
}

// UpgradeLifecycleProvider is an InfraLifecycleProvider whose clusters the
// controller upgrades to their definition's KubernetesVersion.
type UpgradeLifecycleProvider interface {
	InfraLifecycleProvider

	// GetUpgrade fetches the instance's upgrade state from the API.
	GetUpgrade() (*UpgradeSnapshot, error)

	// GetUpgradeTarget fetches the version the instance's definition asks
	// for: its KubernetesVersion, or the default.
	GetUpgradeTarget() (string, error)

	// RecordUpgradeStatus records the version the cluster runs, when it is
	// not "", and what the controller found, without starting an upgrade.
	RecordUpgradeStatus(version, status string) error

	// AckUpgrade records that an upgrade started: Upgrading, the
	// acknowledgement and status, and clears Failed and Refused.
	AckUpgrade(status string) error

	// RefreshUpgradeAck updates the acknowledgement while the upgrade runs.
	RefreshUpgradeAck() error

	// FinishUpgrade records how the upgrade ended, and clears Upgrading.
	FinishUpgrade(outcome UpgradeOutcome) error

	// OnUpgraded performs provider-specific work after a successful upgrade.
	OnUpgraded(infra InfraProvider) error
}

// UpgradeableInfra is an InfraProvider whose cluster can be upgraded.
type UpgradeableInfra interface {
	InfraProvider

	// ClusterKubernetesVersion reads from the provider the Kubernetes minor
	// version the cluster runs, such as "1.36".
	ClusterKubernetesVersion() (string, error)

	// UpgradeInfra upgrades the cluster to version, one minor version past the
	// one it runs.  It refreshes the stack against the provider first, and
	// returns an UpgradeRefusedError without changing anything when the
	// upgrade would replace or delete any of the stack's resources.
	UpgradeInfra(version string) error
}

// UpgradeRefusedError is returned by UpgradeInfra when the upgrade would
// replace or delete resources.
type UpgradeRefusedError struct {
	// The resources the upgrade would replace or delete, each as its
	// operation, type and name.
	Steps []string
}

// Error describes the resources the refused upgrade would have replaced or
// deleted.
func (e *UpgradeRefusedError) Error() string {
	return fmt.Sprintf(
		"the upgrade would replace or delete resources, so nothing was changed: %s",
		strings.Join(e.Steps, ", "),
	)
}

// handleUpgrade upgrades the cluster of an instance whose creation is
// confirmed to its definition's KubernetesVersion, one minor version per
// step.  Each step runs in the background; the instance update it records
// starts the next one.
func handleUpgrade(p UpgradeLifecycleProvider, snap *ReconciliationSnapshot, log *logr.Logger) (int64, error) {
	upgrade, err := p.GetUpgrade()
	if err != nil {
		return 0, fmt.Errorf("failed to get upgrade state: %w", err)
	}

	// a running upgrade is left to finish, unless it was interrupted
	if upgrade.Upgrading && upgrade.Acknowledged != nil && !checkStaleAck(*upgrade.Acknowledged) {
		return upgradeRequeueSeconds, nil
	}

	// a failed upgrade is tried again after a wait
	if upgrade.Failed && upgrade.Acknowledged != nil {
		if wait := upgradeRetryDelay - currentClock().Now().UTC().Sub(*upgrade.Acknowledged); wait > 0 {
			return int64(wait.Seconds()) + 1, nil
		}
	}

	target, err := p.GetUpgradeTarget()
	if err != nil {
		return 0, fmt.Errorf("failed to get the definition's Kubernetes version: %w", err)
	}

	infra, err := p.BuildInfra()
	if err != nil {
		return 0, fmt.Errorf("failed to build infra for upgrade: %w", err)
	}
	upgradeable, ok := infra.(UpgradeableInfra)
	if !ok {
		return 0, fmt.Errorf("%T can't upgrade its cluster", infra)
	}

	// the cluster, not the stack, says what it runs: the provider may have
	// upgraded it outside Threeport
	current, err := upgradeable.ClusterKubernetesVersion()
	if err != nil {
		return 0, fmt.Errorf("failed to read the cluster's Kubernetes version: %w", err)
	}
	comparison, err := api_v0.CompareKubernetesVersions(current, target)
	if err != nil {
		return 0, fmt.Errorf("failed to compare the cluster's Kubernetes version with its definition's: %w", err)
	}

	// a cluster on or past its definition's version is recorded as it is; one
	// past it isn't downgraded
	if comparison >= 0 {
		status := upgrade.Status
		if comparison > 0 {
			status = fmt.Sprintf(
				"the cluster runs Kubernetes %s, newer than its definition's %s, and isn't downgraded",
				current, target,
			)
		}

		// an upgrade that reached the version without recording it, because
		// it was interrupted or its last step failed, may have left the
		// step's last work undone, such as the add-ons EKS moves after its
		// node group, so the step runs again on the version the cluster runs
		if comparison == 0 && (upgrade.Upgrading || upgrade.Failed) {
			if err := p.AckUpgrade(fmt.Sprintf("finishing the upgrade to Kubernetes %s", current)); err != nil {
				return 0, fmt.Errorf("failed to acknowledge the upgrade: %w", err)
			}
			return launchInfraUpgrade(upgradeConfig{
				Provider:      p,
				Infra:         upgradeable,
				ExistingState: snap.ResourceInventory,
				From:          current,
				To:            current,
				Log:           log,
			})
		}

		// one taken past the version is finished here, as is a refused one
		// the provider has since done on its own.  A step refused on the
		// version the cluster still runs, as one run again to finish its
		// work can be, stays refused.
		providerUpgraded := upgrade.Refused && current != upgrade.RecordedVersion
		if upgrade.Upgrading || upgrade.Failed || providerUpgraded {
			if comparison == 0 {
				status = fmt.Sprintf("upgraded to Kubernetes %s", current)
			}
			if err := p.OnUpgraded(upgradeable); err != nil {
				return 0, fmt.Errorf("failed to finish the upgrade: %w", err)
			}
			if err := p.FinishUpgrade(UpgradeOutcome{Version: current, Status: status}); err != nil {
				return 0, fmt.Errorf("failed to record the finished upgrade: %w", err)
			}
			return 0, nil
		}

		if current != upgrade.RecordedVersion || status != upgrade.Status {
			if err := p.RecordUpgradeStatus(current, status); err != nil {
				return 0, fmt.Errorf("failed to record the cluster's Kubernetes version: %w", err)
			}
		}
		return 0, nil
	}

	// a refused upgrade waits for the definition to ask for another version
	if upgrade.Refused {
		return 0, nil
	}

	// a cluster Threeport didn't create has no stack to upgrade it with
	if !HasInventory(snap.ResourceInventory) {
		outcome := UpgradeOutcome{
			Version: current,
			Refused: true,
			Status: fmt.Sprintf(
				"refused to upgrade from Kubernetes %s to %s: Threeport didn't create this cluster, so it has no record of its resources to upgrade them with",
				current, target,
			),
		}
		if err := p.FinishUpgrade(outcome); err != nil {
			return 0, fmt.Errorf("failed to record the refused upgrade: %w", err)
		}
		warnUpgrade(p, event.ReasonUpgradeRefused, outcome.Status, log)
		return 0, nil
	}

	next, err := api_v0.NextKubernetesVersion(current)
	if err != nil {
		return 0, err
	}
	if err := p.AckUpgrade(fmt.Sprintf("upgrading from Kubernetes %s to %s", current, next)); err != nil {
		return 0, fmt.Errorf("failed to acknowledge the upgrade: %w", err)
	}

	return launchInfraUpgrade(upgradeConfig{
		Provider:      p,
		Infra:         upgradeable,
		ExistingState: snap.ResourceInventory,
		From:          current,
		To:            next,
		Log:           log,
	})
}

// upgradeConfig contains the parameters of one upgrade step run in the
// background.
type upgradeConfig struct {
	Provider      UpgradeLifecycleProvider
	Infra         UpgradeableInfra
	ExistingState *datatypes.JSON
	From          string
	To            string
	Log           *logr.Logger
}

// step describes the upgrade step, as "from Kubernetes 1.35 to 1.36", or as
// "to Kubernetes 1.36" for one run again on the version the cluster runs.
func (c upgradeConfig) step() string {
	if c.From == c.To {
		return fmt.Sprintf("to Kubernetes %s", c.To)
	}

	return fmt.Sprintf("from Kubernetes %s to %s", c.From, c.To)
}

// upgradedStatus describes a finished upgrade step.
func upgradedStatus(from, version string) string {
	if from == version {
		return fmt.Sprintf("upgraded to Kubernetes %s", version)
	}

	return fmt.Sprintf("upgraded from Kubernetes %s to %s", from, version)
}

// launchInfraUpgrade acquires the concurrency semaphore, then launches
// executeInfraUpgrade in a background goroutine.  It returns a requeue delay
// for the reconciler.
func launchInfraUpgrade(config upgradeConfig) (int64, error) {
	sem := currentSemaphore()
	select {
	case sem <- struct{}{}:
		// acquired slot
	default:
		// the acknowledgement goes stale and the next pass starts again
		config.Log.Info("infrastructure worker pool full, requeuing upgrade")
		return 30, nil
	}

	go func() {
		defer func() { <-sem }()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic: %v", r)
				config.Log.Error(err, "recovered panic in infrastructure upgrade goroutine")
				finishUpgrade(config, UpgradeOutcome{
					Failed: true,
					Status: fmt.Sprintf("failed to upgrade %s: %v", config.step(), err),
				})
			}
		}()
		executeInfraUpgrade(config)
	}()

	return upgradeRequeueSeconds, nil
}

// executeInfraUpgrade runs one upgrade step: it restores the recorded stack,
// upgrades the cluster one minor version, records the stack's state and the
// outcome on the instance, and checks the cluster now runs the version.
func executeInfraUpgrade(config upgradeConfig) {
	atomic.AddInt64(&inFlightOps, 1)
	defer atomic.AddInt64(&inFlightOps, -1)

	quitAck := make(chan bool, 1)
	go refreshAck(config.Provider.RefreshUpgradeAck, quitAck, config.Log)
	defer func() { quitAck <- true }()

	failed := func(err error) {
		config.Log.Error(err, "failed to upgrade the cluster", "from", config.From, "to", config.To)
		finishUpgrade(config, UpgradeOutcome{
			Failed: true,
			Status: fmt.Sprintf("failed to upgrade %s: %v", config.step(), err),
		})
	}

	if err := config.Infra.SetStackState(config.ExistingState); err != nil {
		failed(fmt.Errorf("failed to restore the stack's state: %w", err))
		return
	}

	var quitStream chan bool
	if streamable, ok := config.Infra.(StreamableProvider); ok {
		quitStream = make(chan bool, 1)
		go streamState(streamable, config.Provider.SaveState, quitStream, config.Log)
	}
	upgradeErr := config.Infra.UpgradeInfra(config.To)
	if quitStream != nil {
		quitStream <- true
	}

	// the stack was refreshed and may have changed even when the upgrade
	// didn't run, so its state is recorded either way
	state, err := config.Infra.GetStackState()
	if err != nil {
		config.Log.Error(err, "failed to get stack state after upgrade")
	} else if state != nil {
		if err := persistWithRetry("save upgrade state", func() error {
			return config.Provider.SaveState(state)
		}, config.Log); err != nil {
			config.Log.Error(err, "failed to save stack state after upgrade")
		}
	}

	var refused *UpgradeRefusedError
	if errors.As(upgradeErr, &refused) {
		status := fmt.Sprintf("refused to upgrade %s: %v", config.step(), refused)
		finishUpgrade(config, UpgradeOutcome{Version: config.From, Refused: true, Status: status})
		warnUpgrade(config.Provider, event.ReasonUpgradeRefused, status, config.Log)
		return
	}
	if upgradeErr != nil {
		failed(upgradeErr)
		warnUpgrade(config.Provider, event.ReasonUpgradeFailed, fmt.Sprintf(
			"failed to upgrade %s: %v", config.step(), upgradeErr,
		), config.Log)
		return
	}

	// the step is done only when the cluster says so
	version, err := config.Infra.ClusterKubernetesVersion()
	if err != nil {
		failed(fmt.Errorf("failed to read the cluster's Kubernetes version after the upgrade: %w", err))
		return
	}
	comparison, err := api_v0.CompareKubernetesVersions(version, config.To)
	if err != nil {
		failed(err)
		return
	}
	if comparison < 0 {
		failed(fmt.Errorf("the cluster still runs Kubernetes %s after the upgrade", version))
		return
	}

	if err := config.Provider.OnUpgraded(config.Infra); err != nil {
		failed(fmt.Errorf("failed to finish the upgrade: %w", err))
		return
	}
	finishUpgrade(config, UpgradeOutcome{
		Version: version,
		Status:  upgradedStatus(config.From, version),
	})
}

// finishUpgrade records the outcome of an upgrade, retrying because the
// controller waits on Upgrading until it is recorded or goes stale.
func finishUpgrade(config upgradeConfig, outcome UpgradeOutcome) {
	if err := persistWithRetry("record upgrade outcome", func() error {
		return config.Provider.FinishUpgrade(outcome)
	}, config.Log); err != nil {
		config.Log.Error(err, "failed to record the upgrade's outcome, stale ack detection will recover")
	}
}

// warnUpgrade records a Warning event about an upgrade against the instance.
func warnUpgrade(p InfraLifecycleProvider, reason, note string, log *logr.Logger) {
	if err := p.RecordWarning(reason, note); err != nil {
		log.Error(err, "failed to record upgrade warning event", "reason", reason)
	}
}

// destructiveOps are the operations an upgrade refuses: each replaces or
// deletes a resource.
var destructiveOps = map[apitype.OpType]bool{
	apitype.OpReplace:           true,
	apitype.OpCreateReplacement: true,
	apitype.OpDeleteReplaced:    true,
	apitype.OpDelete:            true,
}

// destructiveSteps returns the resources a stream of preview events would
// replace or delete, each once, as "replace" or "delete" and the resource's
// type and name, sorted.
func destructiveSteps(engineEvents []events.EngineEvent) []string {
	operations := map[string]string{}
	for _, engineEvent := range engineEvents {
		if engineEvent.ResourcePreEvent == nil {
			continue
		}
		metadata := engineEvent.ResourcePreEvent.Metadata
		if !destructiveOps[metadata.Op] {
			continue
		}
		operation := "replace"
		if metadata.Op == apitype.OpDelete {
			operation = "delete"
		}
		name := metadata.URN[strings.LastIndex(metadata.URN, "::")+len("::"):]
		operations[fmt.Sprintf("%s %s", metadata.Type, name)] = operation
	}

	steps := make([]string, 0, len(operations))
	for resource, operation := range operations {
		steps = append(steps, operation+" "+resource)
	}
	sort.Strings(steps)

	return steps
}

// previewEventsTimeout bounds the wait for the last preview events once the
// preview has returned.
const previewEventsTimeout = 30 * time.Second

// previewDestructiveSteps previews an up of stack and returns the resources it
// would replace or delete.
func (w *PulumiWorkspace) previewDestructiveSteps(ctx context.Context, stack auto.Stack) ([]string, error) {
	eventsChan := make(chan events.EngineEvent)
	var mu sync.Mutex
	var received []events.EngineEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		for engineEvent := range eventsChan {
			mu.Lock()
			received = append(received, engineEvent)
			mu.Unlock()
		}
	}()

	if _, err := stack.Preview(ctx, optpreview.EventStreams(eventsChan)); err != nil {
		return nil, fmt.Errorf("failed to preview the upgrade: %w", err)
	}
	select {
	case <-done:
	case <-time.After(previewEventsTimeout):
		return nil, errors.New("timed out waiting for the upgrade preview's events")
	}

	mu.Lock()
	defer mu.Unlock()
	return destructiveSteps(received), nil
}

// upgradeStack upgrades a stack set up with a program that asks for the
// upgraded cluster.  Setting the stack up refreshed it against the provider;
// this previews the up, and runs it unless the preview replaces or deletes
// anything.
func (w *PulumiWorkspace) upgradeStack(stack auto.Stack) error {
	ctx := context.Background()
	steps, err := w.previewDestructiveSteps(ctx, stack)
	if err != nil {
		return err
	}
	if len(steps) > 0 {
		return &UpgradeRefusedError{Steps: steps}
	}

	if _, err := w.RunUp(ctx, stack); err != nil {
		return fmt.Errorf("failed to deploy stack: %w", err)
	}

	return nil
}
