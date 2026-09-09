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

// LifecycleConfig holds the tunable parameters of the infra
// lifecycle state machine. Production uses the defaults; tests
// override via setLifecycleConfig.
type LifecycleConfig struct {
	// StaleAckThreshold is how long an acknowledgement may go unrefreshed
	// before another control plane replica may take the operation over.
	StaleAckThreshold time.Duration

	// RefreshInterval is how often a running operation refreshes its
	// acknowledgement so other replicas can see it is still alive.
	RefreshInterval time.Duration

	// SemaphoreCapacity is how many infrastructure operations this process
	// runs at once. Requests past the cap are requeued rather than held in
	// memory.
	SemaphoreCapacity int

	// PersistRetries is how many times a failure is written back to the API
	// before the operation gives up and leaves recovery to stale ack
	// detection.
	PersistRetries int

	// PersistRetryDelay is how long to wait between attempts to write a
	// failure back to the API.
	PersistRetryDelay time.Duration
}

// defaultLifecycleConfig is the production configuration.
var defaultLifecycleConfig = LifecycleConfig{
	StaleAckThreshold: 240 * time.Second,
	RefreshInterval:   60 * time.Second,
	SemaphoreCapacity: 5,
	PersistRetries:    30,
	PersistRetryDelay: 10 * time.Second,
}

// lifecycleMu guards lifecycleConfig, infraSemaphore, and lifecycleClock.
// Production sets them once at init and only reads, so the read lock is
// uncontended; tests swap them via setLifecycleConfig and setLifecycleClock,
// and the lock keeps that swap from racing the background goroutines that
// read the values.
var lifecycleMu sync.RWMutex

// lifecycleConfig is the active configuration read by the lifecycle
// state machine.
var lifecycleConfig = defaultLifecycleConfig

// infraSemaphore limits concurrent infrastructure operations to prevent OOM
// from too many simultaneous deployments. Its capacity comes from the
// default config so the tunable and the running pool cannot drift apart.
var infraSemaphore = make(chan struct{}, defaultLifecycleConfig.SemaphoreCapacity)

// currentConfig returns the active lifecycle configuration.
func currentConfig() LifecycleConfig {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return lifecycleConfig
}

// currentSemaphore returns the active semaphore channel. Callers that acquire
// and release a slot must capture this once so a config swap cannot move the release.
func currentSemaphore() chan struct{} {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return infraSemaphore
}

// withDefaults returns the config with any unset field filled from the
// production defaults. A zero value is never a usable setting: a zero
// refresh interval spins the ack-refresh loop against the API as fast as
// the CPU allows, a zero capacity builds an unbuffered channel that
// requeues every instance forever, and a zero retry count skips the
// persist call the caller asked for.
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

// setLifecycleConfig swaps the lifecycle tunables and rebuilds the semaphore
// at the new capacity. Tests call it and restore through t.Cleanup.
func setLifecycleConfig(c LifecycleConfig) (restore func()) {
	c = c.withDefaults()

	lifecycleMu.Lock()
	oldConfig := lifecycleConfig
	oldSemaphore := infraSemaphore
	lifecycleConfig = c
	infraSemaphore = make(chan struct{}, c.SemaphoreCapacity)
	lifecycleMu.Unlock()
	return func() {
		lifecycleMu.Lock()
		lifecycleConfig = oldConfig
		infraSemaphore = oldSemaphore
		lifecycleMu.Unlock()
	}
}

// Clock abstracts wall-clock reads for stale-ack logic so tests can
// inject a fixed or advanceable time.
type Clock interface{ Now() time.Time }

// realClock implements Clock using the system wall clock.
type realClock struct{}

// Now returns the current system wall-clock time.
func (realClock) Now() time.Time { return time.Now() }

// lifecycleClock is the clock used for stale-ack checks.
var lifecycleClock Clock = realClock{}

// currentClock returns the active lifecycle clock.
func currentClock() Clock {
	lifecycleMu.RLock()
	defer lifecycleMu.RUnlock()
	return lifecycleClock
}

// setLifecycleClock swaps the lifecycle clock and returns a restore func
// for t.Cleanup. Only for tests.
func setLifecycleClock(c Clock) (restore func()) {
	lifecycleMu.Lock()
	oldClock := lifecycleClock
	lifecycleClock = c
	lifecycleMu.Unlock()
	return func() {
		lifecycleMu.Lock()
		lifecycleClock = oldClock
		lifecycleMu.Unlock()
	}
}

// inFlightOps counts infrastructure create and delete operations
// currently executing in background goroutines.
var inFlightOps int64

// inFlightCount returns the number of in-flight infrastructure
// operations. Only for tests.
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
			// save provider-specific outputs
			if err := p.SaveCreateOutputs(infra, state); err != nil {
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

	// acknowledge deletion
	if err := p.AckDeletion(); err != nil {
		return 0, fmt.Errorf("failed to acknowledge deletion: %w", err)
	}

	// build infra
	infra, err := p.BuildInfra()
	if err != nil {
		return 0, fmt.Errorf("failed to build infra for delete: %w", err)
	}

	// re-fetch for latest resource inventory
	snap, err = p.GetReconciliation()
	if err != nil {
		return 0, fmt.Errorf("failed to get resource inventory for delete: %w", err)
	}

	// wire callbacks and launch goroutine
	callbacks := infraCallbacks{
		RefreshAck: p.RefreshDeletionAck,
		SaveState:  p.SaveState,
		OnSuccess: func(_ *datatypes.JSON) error {
			// clear inventory to signal destroy complete
			if err := p.ClearInventory(); err != nil {
				log.Error(err, "failed to clear resource inventory after deletion")
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
	// acquire infrastructure concurrency semaphore; capture the channel
	// once so the release lands on the same channel
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
	// acquire infrastructure concurrency semaphore; capture the channel
	// once so the release lands on the same channel
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
		defer func() {
			if r := recover(); r != nil {
				config.Log.Error(fmt.Errorf("panic: %v", r), "recovered panic in infrastructure delete goroutine")
			}
		}()
		executeInfraDelete(config)
	}()

	return 300, nil
}

// executeInfraCreate runs the full infrastructure create lifecycle in a
// goroutine. It handles state restoration, optional streaming for providers
// that support it, and captures final state on success or failure.
func executeInfraCreate(config infraConfig) {
	// track in-flight operations for observability in tests
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
		config.Log.Error(err, "failed to execute success callback")
	}
}

// executeInfraDelete runs the full infrastructure delete lifecycle in a
// goroutine. It handles state restoration, optional refresh for providers
// that support it, and captures updated state on failure.
func executeInfraDelete(config infraConfig) {
	// track in-flight operations for observability in tests
	atomic.AddInt64(&inFlightOps, 1)
	defer atomic.AddInt64(&inFlightOps, -1)

	// refresh the deletion acknowledgement until this function returns
	quitAck := make(chan bool, 1)
	go refreshAck(config.Callbacks.RefreshAck, quitAck, config.Log)
	defer func() { quitAck <- true }()

	// restore state from ResourceInventory if available so the provider
	// knows which cloud resources to destroy
	if hasExistingState(config.ExistingState) {
		// validate state JSON before restoring — corrupt/truncated state from
		// a partial fsnotify write would cause SetStackState to fail
		if !json.Valid(*config.ExistingState) {
			config.Log.Error(
				fmt.Errorf("existing state is not valid JSON (%d bytes)", len(*config.ExistingState)),
				"skipping state restoration for delete, will attempt destroy without state",
			)
		} else {
			if err := config.Infra.SetStackState(config.ExistingState); err != nil {
				config.Log.Error(err, "failed to restore stack state for delete, proceeding without state")
			} else {
				config.Log.Info("restored state from database for deletion")

				// refresh state to sync with cloud reality if provider supports it
				if refreshable, ok := config.Infra.(RefreshableProvider); ok {
					if err := refreshable.RefreshStack(); err != nil {
						config.Log.Error(err, "failed to refresh stack state before delete, proceeding with destroy")
					} else {
						config.Log.Info("refreshed stack state against cloud reality")
					}
				}
			}
		}
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

// refreshAck calls the provided refresh function on the configured refresh
// interval until told to quit, preventing stale acknowledgement detection
// from re-launching the operation while it is still running.
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

// persistFailure calls the provided persist function to mark the operation as
// failed. If the call fails, it is retried on the configured delay up to the
// configured retry count. After exhausting retries, the goroutine returns and
// stale ack detection will recover the operation.
//
// The caller runs inside a launch goroutine that holds a semaphore slot for
// this whole window, so every sleep here is capacity the pool cannot hand to
// another instance.
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

		// sleeping after the final attempt buys nothing and holds the slot
		// for one more delay
		if attempt == maxRetries-1 {
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

	if resourceCount == 0 {
		return fmt.Errorf("state contains no resources")
	}

	log.Info("state verification passed", "resourceCount", resourceCount)
	return nil
}

// inventoryCleared returns true if the ResourceInventory is nil, empty,
// or contains only "{}" or "null".
func inventoryCleared(inventory *datatypes.JSON) bool {
	return inventory == nil ||
		len(*inventory) == 0 ||
		string(*inventory) == "{}" ||
		string(*inventory) == "null"
}

// hasExistingState returns true if the state is non-nil, non-empty, and not
// a placeholder value ("{}" or "null").
func hasExistingState(state *datatypes.JSON) bool {
	return state != nil &&
		len(*state) > 0 &&
		string(*state) != "{}" &&
		string(*state) != "null"
}
