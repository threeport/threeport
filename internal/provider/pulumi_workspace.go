package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/events"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v2"
	"gorm.io/datatypes"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// PulumiWorkspace encapsulates all Pulumi workspace, stack, and state management
// logic that is provider-agnostic. Embed this struct in provider-specific infra
// structs (e.g., KubernetesRuntimeInfraOKE) to get workspace/state/stack
// management for free.
type PulumiWorkspace struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// ProjectName is the Pulumi project name (e.g., "oke", "eks", "gke").
	ProjectName string

	// ProjectDescription is the description written to Pulumi.yaml.
	ProjectDescription string

	// StackConfigs are key-value pairs set on the Pulumi stack (e.g.,
	// {"oci:region": "us-ashburn-1"}).
	StackConfigs map[string]string

	// stateDir is the path to the Pulumi state directory on disk.
	stateDir string

	// Logger enables structured logging for Pulumi operations.
	// When nil, ProgressStreams(os.Stdout) is used (CLI path).
	// When set, EventStreams is used with structured logr output (controller path).
	Logger *logr.Logger
}

// logInfo logs an informational message using the structured logger if
// available, otherwise falls back to CLI info formatting for the CLI path.
func (w *PulumiWorkspace) logInfo(msg string, keysAndValues ...interface{}) {
	if w.Logger != nil {
		w.Logger.Info(msg, keysAndValues...)
	} else {
		util.CliOutputInfo(msg)
	}
}

// logError logs an error message using the structured logger if available,
// otherwise falls back to fmt.Fprintf(os.Stderr) for the CLI path.
func (w *PulumiWorkspace) logError(err error, msg string) {
	if w.Logger != nil {
		w.Logger.Error(err, msg)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	}
}

// RefreshStack refreshes the Pulumi stack state to match reality in the cloud.
// This clears stale pending operations and updates drifted resource properties.
func (w *PulumiWorkspace) RefreshStack() error {
	// set up Pulumi workspace and get stack
	stack, err := w.SetupStack(func(ctx *pulumi.Context) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace for refresh: %w", err)
	}

	ctx := context.Background()

	// refresh the stack
	if _, err := w.runRefresh(ctx, stack); err != nil {
		return fmt.Errorf("failed to refresh stack: %w", err)
	}

	return nil
}

// GetStackState returns the state of the stack as a JSON object.
func (w *PulumiWorkspace) GetStackState() (*datatypes.JSON, error) {
	workspace, err := w.initWorkspace()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Pulumi workspace: %w", err)
	}

	ctx := context.Background()

	// load stack from workspace
	stack, err := auto.SelectStack(ctx, w.getStackName(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	// get the stack's state
	state, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to export stack state: %w", err)
	}

	// convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	jsonState := datatypes.JSON(stateJSON)
	return &jsonState, nil
}

// SetStackState restores the Pulumi state from a JSON object stored in the
// database. State in export/deployment format (from GetStackState via
// stack.Export) is imported via stack.Import which converts to the backend's
// checkpoint format. State in raw checkpoint format (from ReadStateFile during
// streaming) is written directly to the state file.
func (w *PulumiWorkspace) SetStackState(state *datatypes.JSON) error {
	// initialize workspace and create/select the stack
	workspace, err := w.initWorkspace()
	if err != nil {
		return fmt.Errorf("failed to initialize Pulumi workspace: %w", err)
	}

	ctx := context.Background()

	stack, err := auto.UpsertStack(ctx, w.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to create/select stack for state import: %w", err)
	}

	// try to unmarshal as UntypedDeployment (export format from GetStackState)
	var deployment apitype.UntypedDeployment
	if err := json.Unmarshal(*state, &deployment); err == nil && deployment.Deployment != nil {
		// export format — use stack.Import for proper backend format conversion
		if err := stack.Import(ctx, deployment); err != nil {
			return fmt.Errorf("failed to import stack state: %w", err)
		}
		return nil
	}

	// checkpoint format (from ReadStateFile) — write directly to state file
	stateFilePath, err := w.GetStateFilePath()
	if err != nil {
		return fmt.Errorf("failed to get state file path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(stateFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create state file directory: %w", err)
	}
	// write state atomically via temp file + rename to prevent corruption
	// if the process crashes mid-write
	tmpFile := stateFilePath + ".tmp"
	if err := os.WriteFile(tmpFile, *state, 0644); err != nil {
		return fmt.Errorf("failed to write temporary state file: %w", err)
	}
	if err := os.Rename(tmpFile, stateFilePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// GetStateFilePath returns the path to the Pulumi state JSON file on disk.
func (w *PulumiWorkspace) GetStateFilePath() (string, error) {
	if err := w.setStateDir(); err != nil {
		return "", fmt.Errorf("failed to set state directory: %w", err)
	}

	return filepath.Join(
		w.stateDir,
		".pulumi", "stacks", w.ProjectName,
		w.RuntimeInstanceName+".json",
	), nil
}

// ReadStateFile reads the current Pulumi state directly from disk.
// Returns nil (not error) if the file doesn't exist yet.
func (w *PulumiWorkspace) ReadStateFile() (*datatypes.JSON, error) {
	stateFilePath, err := w.GetStateFilePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get state file path: %w", err)
	}

	// return nil if file doesn't exist yet (Pulumi hasn't written it)
	if _, err := os.Stat(stateFilePath); os.IsNotExist(err) {
		return nil, nil
	}

	stateBytes, err := os.ReadFile(stateFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	state := datatypes.JSON(stateBytes)
	return &state, nil
}

// SetupStack creates and configures a Pulumi stack for the given program,
// reusing the common workspace initialization. If existing state is detected
// from a previous run, it refreshes the stack to reconcile with cloud state
// and clear pending operations from interrupted runs.
func (w *PulumiWorkspace) SetupStack(program pulumi.RunFunc) (auto.Stack, error) {
	workspace, err := w.initWorkspace(auto.Program(program))
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to initialize Pulumi workspace: %w", err)
	}

	ctx := context.Background()

	// create or select a stack
	stack, err := auto.UpsertStack(ctx, w.getStackName(), workspace)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create/select stack: %w", err)
	}

	// set up stack configuration from provider-specific config map
	for key, value := range w.StackConfigs {
		if err := stack.SetConfig(ctx, key, auto.ConfigValue{Value: value}); err != nil {
			return auto.Stack{}, fmt.Errorf("failed to set stack config %q: %w", key, err)
		}
	}

	// if existing state with actual resources is detected, reconcile with
	// cloud state to clear pending operations from interrupted runs before
	// deploying. UpsertStack creates an empty state file, so check file
	// size to distinguish real state from a freshly-initialized stack.
	stateFilePath, _ := w.GetStateFilePath()
	if fi, err := os.Stat(stateFilePath); err == nil && fi.Size() > 0 {
		w.logInfo("existing Pulumi state detected, reconciling with cloud state")
		if _, refreshErr := w.runRefresh(ctx, stack); refreshErr != nil {
			// log but don't fail — the subsequent up may still succeed
			w.logInfo("warning: failed to reconcile stack state", "error", refreshErr)
		}
	}

	return stack, nil
}

// RunUp runs Pulumi stack.Up with either structured logging or progress streams.
func (w *PulumiWorkspace) RunUp(ctx context.Context, stack auto.Stack) (auto.UpResult, error) {
	if w.Logger == nil {
		return stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	}

	eventsChan := make(chan events.EngineEvent)
	go w.logEvents(eventsChan, "up")
	return stack.Up(ctx, optup.EventStreams(eventsChan))
}

// RunDestroy runs Pulumi stack.Destroy with either structured logging or progress streams.
func (w *PulumiWorkspace) RunDestroy(ctx context.Context, stack auto.Stack) (auto.DestroyResult, error) {
	if w.Logger == nil {
		return stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	}

	eventsChan := make(chan events.EngineEvent)
	go w.logEvents(eventsChan, "destroy")
	return stack.Destroy(ctx, optdestroy.EventStreams(eventsChan))
}

// DestroyStack tears down the Pulumi stack: sets up workspace, refreshes state,
// destroys resources, and removes the local state directory.
func (w *PulumiWorkspace) DestroyStack() error {
	// set up Pulumi workspace and get stack
	stack, err := w.SetupStack(func(ctx *pulumi.Context) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	ctx := context.Background()

	// refresh state before destroy to clear stale pending operations and
	// ensure Pulumi knows the current state of all cloud resources
	if _, err := w.runRefresh(ctx, stack); err != nil {
		// log but don't fail - destroy may still succeed without refresh
		w.logError(err, "failed to refresh stack before destroy, proceeding with destroy")
	}

	// destroy the stack
	_, err = w.RunDestroy(ctx, stack)
	if err != nil {
		return fmt.Errorf("failed to destroy stack: %w", err)
	}

	// remove the state directory after successful destruction
	if err := os.RemoveAll(w.stateDir); err != nil {
		return fmt.Errorf("failed to remove state directory: %w", err)
	}

	return nil
}

// DeleteStackState deletes the Pulumi stack state directory.
func (w *PulumiWorkspace) DeleteStackState() error {
	stateDir, err := GetPulumiRuntimeStateDir(w.RuntimeInstanceName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		return nil // directory doesn't exist, nothing to delete
	}
	return os.RemoveAll(stateDir)
}

// HasStateDir returns true if the Pulumi state directory exists on disk,
// indicating infrastructure may still exist or has state to clean up.
func (w *PulumiWorkspace) HasStateDir() bool {
	stateDir, err := GetPulumiRuntimeStateDir(w.RuntimeInstanceName)
	if err != nil {
		return false
	}
	_, err = os.Stat(stateDir)
	return err == nil
}

// initWorkspace initializes the Pulumi workspace directory, environment
// variables, project file, and creates the local workspace. Additional options
// (e.g. auto.Program) can be passed to customize the workspace.
func (w *PulumiWorkspace) initWorkspace(opts ...auto.LocalWorkspaceOption) (auto.Workspace, error) {
	// set up state directory
	if err := w.setStateDir(); err != nil {
		return nil, fmt.Errorf("failed to set state directory: %w", err)
	}

	// get Pulumi environment variables as a map (not process-global)
	envVars, err := w.getEnvVars()
	if err != nil {
		return nil, fmt.Errorf("failed to get Pulumi environment variables: %w", err)
	}

	// create Pulumi.yaml project file
	pulumiProject := map[string]string{
		"name":        w.ProjectName,
		"runtime":     "go",
		"description": w.ProjectDescription,
	}
	pulumiYamlBytes, err := yaml.Marshal(pulumiProject)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Pulumi.yaml: %w", err)
	}
	pulumiYamlPath := filepath.Join(w.stateDir, "Pulumi.yaml")
	if err := os.WriteFile(pulumiYamlPath, pulumiYamlBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to create Pulumi.yaml: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend and per-instance env vars
	allOpts := append(
		[]auto.LocalWorkspaceOption{
			auto.WorkDir(w.stateDir),
			auto.EnvVars(envVars),
		},
		opts...,
	)
	workspace, err := auto.NewLocalWorkspace(ctx, allOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	return workspace, nil
}

// runRefresh runs Pulumi stack.Refresh with either structured logging or progress streams.
func (w *PulumiWorkspace) runRefresh(ctx context.Context, stack auto.Stack) (auto.RefreshResult, error) {
	if w.Logger == nil {
		return stack.Refresh(ctx, optrefresh.ProgressStreams(os.Stdout))
	}

	eventsChan := make(chan events.EngineEvent)
	go w.logEvents(eventsChan, "refresh")
	return stack.Refresh(ctx, optrefresh.EventStreams(eventsChan))
}

// logEvents consumes Pulumi engine events and logs them via structured logging.
func (w *PulumiWorkspace) logEvents(eventsChan <-chan events.EngineEvent, operation string) {
	logger := w.Logger.WithValues(
		"component", "pulumi",
		"pulumiOperation", operation,
		"runtimeInstance", w.RuntimeInstanceName,
	)

	seq := 0
	for event := range eventsChan {
		seq++
		w.logEvent(logger, event, seq)
	}
}

// logEvent logs a single Pulumi engine event.
func (w *PulumiWorkspace) logEvent(logger logr.Logger, event events.EngineEvent, seq int) {
	switch {
	case event.DiagnosticEvent != nil:
		e := event.DiagnosticEvent
		if e.Severity == "error" {
			logger.Info("pulumi diagnostic",
				"sequence", seq,
				"severity", e.Severity,
				"message", e.Message,
				"urn", e.URN,
			)
		}
	case event.ResourcePreEvent != nil:
		e := event.ResourcePreEvent
		logger.Info("pulumi resource operation starting",
			"sequence", seq,
			"resourceType", e.Metadata.Type,
			"op", string(e.Metadata.Op),
			"urn", string(e.Metadata.URN),
		)
	case event.ResOutputsEvent != nil:
		e := event.ResOutputsEvent
		logger.Info("pulumi resource operation complete",
			"sequence", seq,
			"resourceType", e.Metadata.Type,
			"op", string(e.Metadata.Op),
			"urn", string(e.Metadata.URN),
		)
	case event.ResOpFailedEvent != nil:
		e := event.ResOpFailedEvent
		logger.Info("pulumi resource operation failed",
			"sequence", seq,
			"resourceType", e.Metadata.Type,
			"op", string(e.Metadata.Op),
			"urn", string(e.Metadata.URN),
		)
	case event.SummaryEvent != nil:
		e := event.SummaryEvent
		logger.Info("pulumi operation summary",
			"sequence", seq,
			"durationSeconds", e.DurationSeconds,
			"resourceChanges", e.ResourceChanges,
		)
	case event.PreludeEvent != nil:
		e := event.PreludeEvent
		logger.Info("pulumi operation starting",
			"sequence", seq,
			"config", e.Config,
		)
	case event.CancelEvent != nil:
		logger.Info("pulumi operation cancelled", "sequence", seq)
	case event.StdoutEvent != nil:
		e := event.StdoutEvent
		logger.V(1).Info("pulumi stdout",
			"sequence", seq,
			"message", e.Message,
		)
	}
}

// getEnvVars returns Pulumi environment variables as a map for use with
// auto.EnvVars(), avoiding process-global os.Setenv which is unsafe for
// concurrent goroutines operating on different stacks.
func (w *PulumiWorkspace) getEnvVars() (map[string]string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	pulumiHome := filepath.Join(userHome, ".threeport", "pulumi-home")
	if err := os.MkdirAll(pulumiHome, 0755); err != nil {
		return nil, fmt.Errorf("failed to create pulumi home directory: %w", err)
	}

	return map[string]string{
		"PULUMI_BACKEND_URL":            "file://" + w.stateDir,
		"PULUMI_HOME":                   pulumiHome,
		"PULUMI_PROJECT":                w.ProjectName,
		"PULUMI_CONFIG_PASSPHRASE":      "",
		"PULUMI_IGNORE_AMBIENT_PLUGINS": "true",
		"PULUMI_PLUGIN_PATH":            filepath.Join(pulumiHome, "plugins"),
	}, nil
}

// setStateDir sets the state directory for the Pulumi stack.
func (w *PulumiWorkspace) setStateDir() error {
	dir, err := GetPulumiRuntimeStateDir(w.RuntimeInstanceName)
	if err != nil {
		return err
	}
	w.stateDir = dir
	if err := os.MkdirAll(w.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	return nil
}

// getStackName returns the name of the Pulumi stack.
func (w *PulumiWorkspace) getStackName() string {
	return w.RuntimeInstanceName
}
