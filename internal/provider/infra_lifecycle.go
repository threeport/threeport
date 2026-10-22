package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"gorm.io/datatypes"
)

// LifecycleConfig is the timing and capacity for infrastructure create
// and delete.
type LifecycleConfig struct {
	// The unrefreshed age after which the operation may be re-launched
	StaleAckThreshold time.Duration

	// The interval a running operation refreshes its acknowledgement
	RefreshInterval time.Duration

	// The maximum concurrent operations in this process; extras are requeued, not held
	SemaphoreCapacity int

	// The attempts to persist a creation failure
	PersistRetries int

	// The wait between creation-failure persist attempts
	PersistRetryDelay time.Duration
}

// defaultLifecycleConfig is the production timing and capacity.
var defaultLifecycleConfig = LifecycleConfig{
	StaleAckThreshold: 240 * time.Second,
	RefreshInterval:   60 * time.Second,
	SemaphoreCapacity: 5,
	PersistRetries:    30,
	PersistRetryDelay: 10 * time.Second,
}

// lifecycleMu guards lifecycleConfig, infraSemaphore, and lifecycleClock.
// A write-lock swap cannot race readers in running goroutines.
var lifecycleMu sync.RWMutex

// lifecycleConfig is the live timing and capacity.
var lifecycleConfig = defaultLifecycleConfig

// infraSemaphore limits concurrent infrastructure create and delete goroutines.
// Capacity matches defaultLifecycleConfig.SemaphoreCapacity.
var infraSemaphore = make(chan struct{}, defaultLifecycleConfig.SemaphoreCapacity)

// currentConfig returns the live lifecycle config.
func currentConfig() LifecycleConfig {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return lifecycleConfig
}

// currentSemaphore returns the live infrastructure operation semaphore.
// Capture the channel once so a swap cannot move the release.
func currentSemaphore() chan struct{} {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return infraSemaphore
}

// withDefaults returns a copy with each zero or negative field replaced
// by the production default. A zero refresh interval would spin the
// ack-refresh loop, a zero capacity would requeue every instance, and
// a zero retry count would skip persisting a failure.
func (c LifecycleConfig) withDefaults() LifecycleConfig {
	if c.StaleAckThreshold <= 0 {
		c.StaleAckThreshold = defaultLifecycleConfig.StaleAckThreshold
	}
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = defaultLifecycleConfig.RefreshInterval
	}
	if c.SemaphoreCapacity <= 0 {
		c.SemaphoreCapacity = defaultLifecycleConfig.SemaphoreCapacity
	}
	if c.PersistRetries <= 0 {
		c.PersistRetries = defaultLifecycleConfig.PersistRetries
	}
	if c.PersistRetryDelay <= 0 {
		c.PersistRetryDelay = defaultLifecycleConfig.PersistRetryDelay
	}
	return c
}

// setLifecycleConfig applies defaults to c, installs it as the live config,
// rebuilds the semaphore at that capacity, and returns a restore function.
func setLifecycleConfig(c LifecycleConfig) (restore func()) {
	// ensure every field is set before installing
	c = c.withDefaults()

	// replace the live config and semaphore
	lifecycleMu.Lock()
	oldConfig := lifecycleConfig
	oldSemaphore := infraSemaphore
	lifecycleConfig = c
	infraSemaphore = make(chan struct{}, c.SemaphoreCapacity)
	lifecycleMu.Unlock()

	// put the previous config and semaphore back
	return func() {
		lifecycleMu.Lock()
		lifecycleConfig = oldConfig
		infraSemaphore = oldSemaphore
		lifecycleMu.Unlock()
	}
}

// Clock is a source of the current time for stale-ack checks.
type Clock interface{ Now() time.Time }

// realClock is a Clock that reads the wall clock.
type realClock struct{}

// Now returns the current wall-clock time.
func (realClock) Now() time.Time { return time.Now() }

// lifecycleClock is the live clock used for stale-ack checks.
var lifecycleClock Clock = realClock{}

// currentClock returns the live clock.
func currentClock() Clock {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return lifecycleClock
}

// setLifecycleClock installs c as the live clock and returns a restore
// function that puts the previous clock back.
func setLifecycleClock(c Clock) (restore func()) {
	// replace the live clock
	lifecycleMu.Lock()
	oldClock := lifecycleClock
	lifecycleClock = c
	lifecycleMu.Unlock()

	// put the previous clock back
	return func() {
		lifecycleMu.Lock()
		lifecycleClock = oldClock
		lifecycleMu.Unlock()
	}
}

// inFlightOps counts create and delete operations running in goroutines.
var inFlightOps int64

// inFlightCount returns the running create and delete operation count.
func inFlightCount() int64 { return atomic.LoadInt64(&inFlightOps) }

// ReconciliationSnapshot captures the reconciliation timestamps and resource
// inventory for a provider instance at a point in time. This decouples the
// lifecycle handler from any specific API object type.
type ReconciliationSnapshot struct {
	CreationAcknowledged *time.Time
	CreationConfirmed    *time.Time
	CreationFailed       bool
	DeletionScheduled    *time.Time
	DeletionAcknowledged *time.Time
	DeletionConfirmed    *time.Time
	ResourceInventory    *datatypes.JSON

	// AbandonInfrastructure is the operator's explicit permission to delete an
	// instance whose recorded inventory is empty even though its creation was
	// confirmed. The cloud resources, if any, are left in place and untracked.
	AbandonInfrastructure bool
}

// InfraLifecycleProvider defines the provider-specific operations needed by the
// HandleInfraCreate and HandleInfraDelete state machines. Each infrastructure
// provider implements this interface; the lifecycle handler does everything
// else (ack/confirm checks, stale detection, goroutine wiring).
type InfraLifecycleProvider interface {
	// GetReconciliation fetches the latest reconciliation state and resource
	// inventory from the API.
	GetReconciliation() (*ReconciliationSnapshot, error)

	// BuildInfra constructs the InfraProvider implementor for this provider.
	BuildInfra() (InfraProvider, error)

	// IsCreateComplete checks whether the async create operation has finished.
	IsCreateComplete() (bool, error)

	// OnCreateConfirmed performs provider-specific post-creation work.
	OnCreateConfirmed(infra InfraProvider) error

	// SaveCreateOutputs saves provider-specific outputs and final state.
	SaveCreateOutputs(infra InfraProvider, state *datatypes.JSON) error

	// OnDeleteConfirmed performs provider-specific post-deletion cleanup.
	OnDeleteConfirmed(infra InfraProvider) error

	// AckCreation sets CreationAcknowledged and clears CreationFailed in the API.
	AckCreation() error

	// RefreshCreationAck updates CreationAcknowledged to prevent stale detection.
	RefreshCreationAck() error

	// SetCreationFailed marks CreationFailed=true in the API.
	SetCreationFailed() error

	// ConfirmCreation sets CreationConfirmed and Reconciled=true in the API.
	ConfirmCreation() error

	// AckDeletion sets DeletionAcknowledged in the API.
	AckDeletion() error

	// RefreshDeletionAck updates DeletionAcknowledged to prevent stale detection.
	RefreshDeletionAck() error

	// ConfirmDeletion sets DeletionConfirmed in the API.
	ConfirmDeletion() error

	// SaveState persists intermediate state to the API for crash recovery.
	SaveState(state *datatypes.JSON) error

	// ClearInventory sets ResourceInventory to "{}" to signal destroy complete.
	ClearInventory() error

	// PublishCreateNotification publishes a NATS notification for creation.
	PublishCreateNotification() error

	// PublishDeleteNotification publishes a NATS notification for deletion.
	PublishDeleteNotification() error
}

// HandleInfraCreate implements the create state machine for any infrastructure
// provider. It checks reconciliation state, manages ack/confirm transitions,
// and launches the create goroutine when needed.
func HandleInfraCreate(p InfraLifecycleProvider, log *logr.Logger) (int64, error) {
	// fetch latest state from API
	snap, err := p.GetReconciliation()
	if err != nil {
		return 0, fmt.Errorf("failed to get reconciliation state: %w", err)
	}

	// check if already reconciled
	if snap.CreationConfirmed != nil {
		return 0, nil
	}

	// check if previously acknowledged and not failed
	if snap.CreationAcknowledged != nil && !snap.CreationFailed {
		// check if creation is complete
		complete, err := p.IsCreateComplete()
		if err != nil {
			return 0, fmt.Errorf("failed to check create completion: %w", err)
		}

		if complete {
			// build infra for post-creation work (e.g., GetConnection)
			infra, err := p.BuildInfra()
			if err != nil {
				return 0, fmt.Errorf("failed to build infra for create confirmation: %w", err)
			}

			// perform provider-specific post-creation work
			if err := p.OnCreateConfirmed(infra); err != nil {
				return 0, fmt.Errorf("failed to run post-creation work: %w", err)
			}

			// confirm creation
			if err := p.ConfirmCreation(); err != nil {
				return 0, fmt.Errorf("failed to confirm creation: %w", err)
			}

			log.Info("creation confirmed")
			return 0, nil
		}

		// not complete yet — check if acknowledgement is stale
		if !checkStaleAck(*snap.CreationAcknowledged) {
			return 120, nil
		}
	}

	// one of the following is true:
	// 1. creation has not been acknowledged — new create request
	// 2. creation has previously failed — time to retry
	// 3. the last acknowledgement is stale — creation was interrupted

	// acknowledge creation
	if err := p.AckCreation(); err != nil {
		return 0, fmt.Errorf("failed to acknowledge creation: %w", err)
	}

	// build infra
	infra, err := p.BuildInfra()
	if err != nil {
		return 0, fmt.Errorf("failed to build infra for create: %w", err)
	}

	// check if deletion was scheduled while we were preparing to create
	snap, err = p.GetReconciliation()
	if err != nil {
		return 0, fmt.Errorf("failed to check deletion status before create: %w", err)
	}
	if snap.DeletionScheduled != nil {
		log.Info("deletion scheduled, aborting create to let delete handler proceed")
		return 0, nil
	}

	// wire callbacks and launch goroutine
	callbacks := infraCallbacks{
		RefreshAck:     p.RefreshCreationAck,
		SaveState:      p.SaveState,
		PersistFailure: p.SetCreationFailed,
		OnSuccess: func(state *datatypes.JSON) error {
			// save provider-specific outputs, retrying because the final state
			// is the only record of the resources just created
			if err := persistWithRetry("save create outputs", func() error {
				return p.SaveCreateOutputs(infra, state)
			}, log); err != nil {
				return fmt.Errorf("failed to save create outputs: %w", err)
			}

			// check if deletion was scheduled during the create operation
			latestSnap, err := p.GetReconciliation()
			if err == nil && latestSnap.DeletionScheduled != nil {
				log.Info("deletion was scheduled during create, skipping create notification to let delete proceed")
				return nil
			}

			// publish notification
			if err := p.PublishCreateNotification(); err != nil {
				log.Error(err, "failed to publish create notification")
			}

			return nil
		},
	}

	return launchInfraCreate(infraConfig{
		Infra:         infra,
		ExistingState: snap.ResourceInventory,
		Callbacks:     callbacks,
		Log:           log,
	})
}

// HandleInfraDelete implements the delete state machine for any infrastructure
// provider. It checks reconciliation state, manages ack/confirm transitions,
// handles cross-replica safety, and launches the delete goroutine when needed.
func HandleInfraDelete(p InfraLifecycleProvider, log *logr.Logger) (int64, error) {
	// fetch latest state from API
	snap, err := p.GetReconciliation()
	if err != nil {
		return 0, fmt.Errorf("failed to get reconciliation state: %w", err)
	}

	// validate that deletion is scheduled
	if snap.DeletionScheduled == nil {
		return 0, errors.New("deletion notification received but not scheduled")
	}

	// check if already confirmed
	if snap.DeletionConfirmed != nil {
		return 0, nil
	}

	// cross-replica safety: if a create operation is still in progress on
	// another replica, requeue to let it finish
	if snap.CreationAcknowledged != nil &&
		!checkStaleAck(*snap.CreationAcknowledged) &&
		snap.CreationConfirmed == nil {
		log.Info("create operation still in progress, requeueing delete")
		return 60, nil
	}

	// check if previously acknowledged
	if snap.DeletionAcknowledged != nil {
		// re-fetch to check if inventory has been cleared
		latestSnap, err := p.GetReconciliation()
		if err != nil {
			return 0, fmt.Errorf("failed to check deletion status: %w", err)
		}

		if inventoryCleared(latestSnap.ResourceInventory) {
			// refresh ack to prevent stale detection during cleanup
			if err := p.RefreshDeletionAck(); err != nil {
				log.Error(err, "failed to refresh deletion ack during cleanup")
			}

			// build infra for post-deletion cleanup
			infra, err := p.BuildInfra()
			if err != nil {
				return 0, fmt.Errorf("failed to build infra for delete confirmation: %w", err)
			}

			// confirm against the cloud where the provider can: a destroy that
			// ran without the right state clears the inventory all the same
			if !latestSnap.AbandonInfrastructure {
				if err := verifyNoCloudResources(infra, log); err != nil {
					return 0, err
				}
			}

			// perform provider-specific post-deletion cleanup
			if err := p.OnDeleteConfirmed(infra); err != nil {
				log.Error(err, "failed to run post-deletion cleanup, will retry")
				return 60, nil
			}

			// confirm deletion
			if err := p.ConfirmDeletion(); err != nil {
				return 0, fmt.Errorf("failed to confirm deletion: %w", err)
			}

			log.Info("deletion confirmed")
			return 0, nil
		}

		// resources not yet destroyed — check if ack is stale
		if checkStaleAck(*snap.DeletionAcknowledged) {
			log.Info("deletion acknowledgement is stale, re-launching delete goroutine")
			// fall through to re-launch
		} else {
			return 60, nil
		}
	}

	// refuse to destroy against an inventory that should not be empty
	if err := checkDeleteInventory(snap, log); err != nil {
		return 0, err
	}

	// with nothing recorded and no confirmed create, the delete cannot tell
	// "never created" from "state lost", so ask the cloud before destroying
	var infra InfraProvider
	if snap.cloudVerificationNeeded() {
		infra, err = p.BuildInfra()
		if err != nil {
			return 0, fmt.Errorf("failed to build infra for delete: %w", err)
		}
		if err := verifyNoCloudResources(infra, log); err != nil {
			return 0, err
		}
	}

	// acknowledge deletion
	if err := p.AckDeletion(); err != nil {
		return 0, fmt.Errorf("failed to acknowledge deletion: %w", err)
	}

	// build infra
	if infra == nil {
		infra, err = p.BuildInfra()
		if err != nil {
			return 0, fmt.Errorf("failed to build infra for delete: %w", err)
		}
	}

	// re-fetch for latest resource inventory
	snap, err = p.GetReconciliation()
	if err != nil {
		return 0, fmt.Errorf("failed to get resource inventory for delete: %w", err)
	}

	// check again against the inventory fetched after the ack so a change
	// since the first fetch cannot slip past
	if err := checkDeleteInventory(snap, log); err != nil {
		return 0, err
	}

	// wire callbacks and launch goroutine
	callbacks := infraCallbacks{
		RefreshAck: p.RefreshDeletionAck,
		SaveState:  p.SaveState,
		OnSuccess: func(_ *datatypes.JSON) error {
			// clear inventory to signal destroy complete
			if err := persistWithRetry("clear resource inventory", p.ClearInventory, log); err != nil {
				log.Error(err, "failed to clear resource inventory after deletion, stale ack detection will retry")
			}

			// publish notification
			if err := p.PublishDeleteNotification(); err != nil {
				log.Error(err, "failed to publish delete notification")
			}

			return nil
		},
	}

	return launchInfraDelete(infraConfig{
		Infra:         infra,
		ExistingState: snap.ResourceInventory,
		Callbacks:     callbacks,
		Log:           log,
	})
}

// infraCallbacks contains callback functions invoked at various points during
// create and delete goroutine lifecycles.
type infraCallbacks struct {
	// RefreshAck updates the acknowledged timestamp to prevent stale detection
	// while the operation is still running.
	RefreshAck func() error

	// SaveState persists intermediate state to the API for crash recovery.
	SaveState func(state *datatypes.JSON) error

	// PersistFailure marks the operation as failed in the API so the
	// reconciler knows to retry. Set for create operations, nil for delete.
	PersistFailure func() error

	// OnSuccess is called after successful infrastructure create or delete.
	// For create, state contains the final Pulumi state; for delete, state is nil.
	OnSuccess func(state *datatypes.JSON) error
}

// infraConfig contains all parameters needed to launch an infrastructure
// create or delete operation in a background goroutine.
type infraConfig struct {
	// Infra is the provider's infrastructure object that implements InfraProvider.
	Infra InfraProvider

	// ExistingState is the previously saved state from ResourceInventory.
	// When non-nil and non-empty, state is restored before operating.
	ExistingState *datatypes.JSON

	// Callbacks contains functions invoked during the operation.
	Callbacks infraCallbacks

	// Log is the structured logger for the operation.
	Log *logr.Logger
}

// checkStaleAck returns true if the given acknowledgement timestamp has gone
// stale, indicating the operation was interrupted (e.g. pod restart).
func checkStaleAck(ackTimestamp time.Time) bool {
	duration := currentClock().Now().UTC().Sub(ackTimestamp)
	return duration > currentConfig().StaleAckThreshold
}

// launchInfraCreate acquires the concurrency semaphore, then launches
// executeInfraCreate in a background goroutine. Returns a requeue delay
// for the reconciler.
func launchInfraCreate(config infraConfig) (int64, error) {
	// capture the semaphore channel
	sem := currentSemaphore()
	select {
	case sem <- struct{}{}:
		// acquired slot
	default:
		config.Log.Info("infrastructure worker pool full, requeuing")
		return 30, nil
	}

	// launch creation in background goroutine
	go func() {
		defer func() { <-sem }()
		defer func() {
			if r := recover(); r != nil {
				config.Log.Error(fmt.Errorf("panic: %v", r), "recovered panic in infrastructure create goroutine")
				persistFailure(config.Callbacks.PersistFailure, config.Log)
			}
		}()
		executeInfraCreate(config)
	}()

	return 120, nil
}

// launchInfraDelete acquires the concurrency semaphore, then launches
// executeInfraDelete in a background goroutine. Returns a requeue delay
// for the reconciler.
func launchInfraDelete(config infraConfig) (int64, error) {
	// capture the semaphore channel
	sem := currentSemaphore()
	select {
	case sem <- struct{}{}:
		// acquired slot
	default:
		config.Log.Info("infrastructure worker pool full, requeuing")
		return 30, nil
	}

	// launch deletion in background goroutine
	go func() {
		defer func() { <-sem }()
		// recover a panic without marking the operation failed
		defer func() {
			if r := recover(); r != nil {
				config.Log.Error(fmt.Errorf("panic: %v", r), "recovered panic in infrastructure delete goroutine")
			}
		}()
		executeInfraDelete(config)
	}()

	// requeue after 300 seconds while the delete runs
	return 300, nil
}

// executeInfraCreate runs the full infrastructure create lifecycle in a
// goroutine. It handles state restoration, optional streaming for providers
// that support it, and captures final state on success or failure.
func executeInfraCreate(config infraConfig) {
	// count this operation as in flight
	atomic.AddInt64(&inFlightOps, 1)
	defer atomic.AddInt64(&inFlightOps, -1)

	// refresh the creation acknowledgement until this function returns
	quitAck := make(chan bool, 1)
	go refreshAck(config.Callbacks.RefreshAck, quitAck, config.Log)
	defer func() { quitAck <- true }()

	// restore state from ResourceInventory if available (retry after failure
	// or pod restart so the provider knows about previously created resources)
	if hasExistingState(config.ExistingState) {
		if err := config.Infra.SetStackState(config.ExistingState); err != nil {
			config.Log.Error(err, "failed to restore stack state for retry")
			persistFailure(config.Callbacks.PersistFailure, config.Log)
			return
		}
		config.Log.Info("restored state from database for creation retry")

		// refresh state to sync with cloud reality if provider supports it
		if refreshable, ok := config.Infra.(RefreshableProvider); ok {
			if err := refreshable.RefreshStack(); err != nil {
				config.Log.Error(err, "failed to refresh stack state")
				// mark creation failed and do not deploy
				persistFailure(config.Callbacks.PersistFailure, config.Log)
				return
			}
			config.Log.Info("refreshed stack state against cloud reality")
		}
	}

	// start state streaming if provider supports it
	var quitStream chan bool
	streamStopped := false
	if streamable, ok := config.Infra.(StreamableProvider); ok {
		quitStream = make(chan bool, 1)
		go streamState(streamable, config.Callbacks.SaveState, quitStream, config.Log)
		defer func() {
			// skip quit when the stream was already stopped
			if !streamStopped {
				quitStream <- true
			}
		}()
	}

	// create infrastructure
	err := config.Infra.DeployInfra()

	// stop the stream watcher before capturing final state to prevent
	// a late fsnotify event from overwriting the authoritative state
	if quitStream != nil && !streamStopped {
		quitStream <- true
		streamStopped = true
	}

	if err != nil {
		config.Log.Error(err, "failed to create infrastructure")

		// capture state even on failure so retries can restore it and
		// avoid creating duplicate cloud resources
		stateJSON, stateErr := config.Infra.GetStackState()
		if stateErr != nil {
			config.Log.Error(stateErr, "failed to get stack state after failed creation")
		} else if stateJSON != nil {
			if saveErr := config.Callbacks.SaveState(stateJSON); saveErr != nil {
				config.Log.Error(saveErr, "failed to save partial state after failed creation")
			}
		}

		persistFailure(config.Callbacks.PersistFailure, config.Log)
		return
	}

	// capture final state
	stateJSON, err := config.Infra.GetStackState()
	if err != nil {
		config.Log.Error(err, "failed to get stack state after creation")
		persistFailure(config.Callbacks.PersistFailure, config.Log)
		return
	}

	// verify state integrity before declaring success
	if err := verifyState(stateJSON, config.Log); err != nil {
		config.Log.Error(err, "state verification failed after creation")
		persistFailure(config.Callbacks.PersistFailure, config.Log)
		return
	}

	// call provider-specific success handler
	if err := config.Callbacks.OnSuccess(stateJSON); err != nil {
		// the infrastructure exists but its state was not recorded, so mark
		// the create failed. Leaving it unmarked lets the stale ack expire and
		// a later reconcile deploy again from an empty inventory. A retry on
		// this process finds the state still on disk and picks up from it.
		config.Log.Error(err, "failed to execute success callback, marking creation failed")
		persistFailure(config.Callbacks.PersistFailure, config.Log)
	}
}

// executeInfraDelete runs the full infrastructure delete lifecycle in a
// goroutine. It handles state restoration, optional refresh for providers
// that support it, and captures updated state on failure.
func executeInfraDelete(config infraConfig) {
	// count this operation as in flight
	atomic.AddInt64(&inFlightOps, 1)
	defer atomic.AddInt64(&inFlightOps, -1)

	// refresh the deletion acknowledgement until this function returns
	quitAck := make(chan bool, 1)
	go refreshAck(config.Callbacks.RefreshAck, quitAck, config.Log)
	defer func() { quitAck <- true }()

	// restore state from ResourceInventory so the provider knows which cloud
	// resources to destroy. A destroy without the recorded state cannot find
	// them and would succeed having removed nothing, which the caller would
	// then confirm as a completed delete.
	if hasExistingState(config.ExistingState) {
		// validate state JSON before restoring — corrupt/truncated state from
		// a partial fsnotify write would cause SetStackState to fail
		if !json.Valid(*config.ExistingState) {
			config.Log.Error(
				fmt.Errorf("existing state is not valid JSON (%d bytes)", len(*config.ExistingState)),
				"refusing to destroy without restorable state, will retry on next reconciliation",
			)
			return
		}
		if err := config.Infra.SetStackState(config.ExistingState); err != nil {
			config.Log.Error(err, "failed to restore stack state for delete, refusing to destroy without it, will retry on next reconciliation")
			return
		}
		config.Log.Info("restored state from database for deletion")

		// refresh state to sync with cloud reality if provider supports it
		if refreshable, ok := config.Infra.(RefreshableProvider); ok {
			if err := refreshable.RefreshStack(); err != nil {
				config.Log.Error(err, "failed to refresh stack state before delete, proceeding with destroy")
			} else {
				config.Log.Info("refreshed stack state against cloud reality")
			}
		}
	} else {
		config.Log.Info("no resource inventory recorded, destroy will not remove any cloud resources")
	}

	// destroy infrastructure
	if err := config.Infra.DestroyInfra(); err != nil {
		config.Log.Error(err, "failed to delete infrastructure, will retry on next reconciliation")

		// capture updated state so retries know which resources remain
		stateJSON, stateErr := config.Infra.GetStackState()
		if stateErr != nil {
			config.Log.Error(stateErr, "failed to get stack state after failed deletion")
		} else if stateJSON != nil {
			if saveErr := config.Callbacks.SaveState(stateJSON); saveErr != nil {
				config.Log.Error(saveErr, "failed to save state after failed deletion")
			}
		}
		return
	}

	// call provider-specific success handler
	if err := config.Callbacks.OnSuccess(nil); err != nil {
		config.Log.Error(err, "failed to execute delete success callback")
	}
}

// streamState watches the state file via fsnotify and pushes changes to the
// API using the saveState callback on every Write/Create event. Only called
// for providers that implement StreamableProvider.
func streamState(
	provider StreamableProvider,
	saveState func(state *datatypes.JSON) error,
	quit chan bool,
	log *logr.Logger,
) {
	// get state file path and pre-create directory
	stateFilePath, err := provider.GetStateFilePath()
	if err != nil {
		log.Error(err, "failed to get state file path for streaming")
		return
	}
	stateDir := filepath.Dir(stateFilePath)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Error(err, "failed to create state directory for watcher")
		return
	}

	// create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error(err, "failed to create fsnotify watcher")
		return
	}
	defer watcher.Close()

	// watch the directory containing the state file
	if err := watcher.Add(stateDir); err != nil {
		log.Error(err, "failed to add directory to watcher")
		return
	}

	stateFileName := filepath.Base(stateFilePath)

	for {
		select {
		case <-quit:
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// only react to write/create events for the state file
			if filepath.Base(event.Name) != stateFileName {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// read and upload state immediately
			state, err := provider.ReadStateFile()
			if err != nil {
				log.Error(err, "failed to read state file during streaming")
				continue
			}
			if state == nil {
				continue
			}

			// validate JSON before uploading to prevent partial writes
			// from overwriting good state in the database
			if !json.Valid(*state) {
				log.V(1).Info("skipping partial state file write (invalid JSON)")
				continue
			}

			// push state via callback
			if err := saveState(state); err != nil {
				log.Error(err, "failed to update resource inventory during state streaming")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Error(err, "fsnotify watcher error")
		}
	}
}

// refreshAck calls refresh every RefreshInterval until quitChan is signaled.
func refreshAck(
	refresh func() error,
	quitChan chan bool,
	log *logr.Logger,
) {
	for {
		select {
		case <-quitChan:
			return
		case <-time.After(currentConfig().RefreshInterval):
			if err := refresh(); err != nil {
				log.Error(err, "failed to refresh acknowledged timestamp")
			}
		}
	}
}

// persistFailure calls persist to mark the operation failed, retrying on
// PersistRetryDelay up to PersistRetries times. Exhausted retries leave
// recovery to stale-ack detection.
func persistFailure(
	persist func() error,
	log *logr.Logger,
) {
	cfg := currentConfig()
	maxRetries := cfg.PersistRetries

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		lastErr = persist()
		if lastErr == nil {
			return
		}

		if attempt == maxRetries-1 {
			// skip sleep after the last attempt
			break
		}

		log.Error(lastErr, "failed to persist creation failure, retrying",
			"attempt", attempt+1, "maxRetries", maxRetries,
			"retryDelay", cfg.PersistRetryDelay)
		time.Sleep(cfg.PersistRetryDelay)
	}

	exhausted := fmt.Errorf("exhausted %d retries", maxRetries)
	if lastErr != nil {
		exhausted = fmt.Errorf("exhausted %d retries: %w", maxRetries, lastErr)
	}
	log.Error(
		exhausted,
		"failed to persist creation failure, stale ack detection will recover",
	)
}

// verifyState checks the integrity of a state JSON object to ensure it
// represents a valid deployment with resources.
func verifyState(state *datatypes.JSON, log *logr.Logger) error {
	// reject a missing state
	if state == nil {
		return fmt.Errorf("state is nil")
	}
	if len(*state) == 0 {
		return fmt.Errorf("state is empty")
	}

	// parse as generic JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(*state, &parsed); err != nil {
		return fmt.Errorf("state is not valid JSON: %w", err)
	}

	// check for resources in either format:
	// - checkpoint format: checkpoint.latest.resources
	// - deployment format: deployment.resources
	resourceCount := 0
	if checkpoint, ok := parsed["checkpoint"].(map[string]interface{}); ok {
		if latest, ok := checkpoint["latest"].(map[string]interface{}); ok {
			if resources, ok := latest["resources"].([]interface{}); ok {
				resourceCount = len(resources)
			}
		}
	}
	if resourceCount == 0 {
		if deployment, ok := parsed["deployment"].(map[string]interface{}); ok {
			if resources, ok := deployment["resources"].([]interface{}); ok {
				resourceCount = len(resources)
			}
		}
	}

	// reject state with no resources
	if resourceCount == 0 {
		return fmt.Errorf("state contains no resources")
	}

	// log the verified resource count
	log.Info("state verification passed", "resourceCount", resourceCount)
	return nil
}

// inventoryCleared returns true only for the "{}" sentinel that ClearInventory
// writes once a destroy has completed. A nil, empty or "null" inventory means
// the inventory is unknown, not that the resources are gone.
func inventoryCleared(inventory *datatypes.JSON) bool {
	return inventory != nil && string(*inventory) == "{}"
}

// HasInventory returns true if the inventory records state for cloud
// resources: it is not nil, empty, "{}" or "null".
func HasInventory(inventory *datatypes.JSON) bool {
	return hasExistingState(inventory)
}

// checkDeleteInventory refuses a delete that would run against an empty
// inventory for an instance whose creation was confirmed. Creation is only
// confirmed once resources were recorded, so an empty inventory here means the
// record was lost, not that there is nothing to destroy. Destroying and
// confirming anyway would drop the API record while the cloud resources keep
// running untracked. AbandonInfrastructure is the operator's explicit override.
func checkDeleteInventory(snap *ReconciliationSnapshot, log *logr.Logger) error {
	if HasInventory(snap.ResourceInventory) || snap.CreationConfirmed == nil {
		return nil
	}

	if snap.AbandonInfrastructure {
		log.Info(
			"WARNING: deleting an instance with no recorded resource inventory because infrastructure abandonment was requested, any cloud resources are left running and untracked",
		)
		return nil
	}

	return ErrEmptyInventory
}

// cloudVerificationNeeded returns true when a delete has no recorded inventory
// and no confirmed create to explain it, and the operator has not abandoned the
// infrastructure. Such a delete is only safe if the cloud holds nothing.
func (s *ReconciliationSnapshot) cloudVerificationNeeded() bool {
	return !HasInventory(s.ResourceInventory) &&
		s.CreationConfirmed == nil &&
		!s.AbandonInfrastructure
}

// verifyNoCloudResources asks the cloud, where the provider supports it,
// whether the infrastructure the delete is about to treat as gone still exists.
// A provider that cannot be asked is passed.
func verifyNoCloudResources(infra InfraProvider, log *logr.Logger) error {
	verifier, ok := infra.(CloudVerifier)
	if !ok {
		return nil
	}

	exists, err := verifier.ResourcesExist()
	if err != nil {
		return fmt.Errorf("failed to verify against the cloud before deleting: %w", err)
	}
	if exists {
		log.Info("WARNING: infrastructure for this instance still exists in the cloud")
		return ErrCloudResourcesExist
	}

	return nil
}

// ErrCloudResourcesExist is returned when a delete is refused because the cloud
// still holds infrastructure the controller was about to treat as gone.
var ErrCloudResourcesExist = errors.New(
	"refusing to delete: infrastructure for this instance still exists in the cloud " +
		"but the controller has no record of it to destroy. " +
		"Remove it by hand, or delete again with --abandon-infrastructure " +
		"(tptctl delete <instance type> --name <name>) to leave it running untracked",
)

// ErrEmptyInventory is returned when a delete is refused because a created
// instance has no recorded resource inventory.
var ErrEmptyInventory = errors.New(
	"refusing to delete: creation was confirmed but no resource inventory is recorded, " +
		"so cloud resources may still exist and cannot be tracked for destruction. " +
		"Verify in the cloud account and remove any remaining resources by hand, " +
		"then delete again with --abandon-infrastructure (tptctl delete <instance type> --name <name>) to proceed",
)

// persistWithRetry calls persist up to PersistRetries times, sleeping
// PersistRetryDelay between attempts, and returns the last error.
func persistWithRetry(
	operation string,
	persist func() error,
	log *logr.Logger,
) error {
	cfg := currentConfig()

	var lastErr error
	for attempt := 0; attempt < cfg.PersistRetries; attempt++ {
		if lastErr = persist(); lastErr == nil {
			return nil
		}

		if attempt == cfg.PersistRetries-1 {
			// skip sleep after the last attempt
			break
		}

		log.Error(lastErr, "failed to "+operation+", retrying",
			"attempt", attempt+1, "maxRetries", cfg.PersistRetries,
			"retryDelay", cfg.PersistRetryDelay)
		time.Sleep(cfg.PersistRetryDelay)
	}

	return fmt.Errorf("exhausted %d retries: %w", cfg.PersistRetries, lastErr)
}

// hasExistingState returns true if the state is non-nil, non-empty, and not
// a placeholder value ("{}" or "null").
func hasExistingState(state *datatypes.JSON) bool {
	return state != nil &&
		len(*state) > 0 &&
		string(*state) != "{}" &&
		string(*state) != "null"
}
