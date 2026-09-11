package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requirePulumiCLI skips the test when the pulumi CLI is not on PATH,
// then points HOME at a temp dir and disables the CLI update check.
func requirePulumiCLI(t *testing.T) {
	// register as a test helper
	t.Helper()
	// skip when the pulumi CLI is not on PATH
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("pulumi CLI not found on PATH; skipping test that needs a real pulumi backend")
	}
	// point HOME at a temp dir so workspace setup stays off the real home
	t.Setenv("HOME", t.TempDir())
	// disable checking for a new pulumi version
	t.Setenv("PULUMI_SKIP_UPDATE_CHECK", "true")
}

// checkpointState returns version-3 checkpoint JSON for a stack named
// name in project. Marker names the stack resource to tell payloads apart.
func checkpointState(project, name, marker string) string {
	return fmt.Sprintf(
		`{"version":3,"checkpoint":{"stack":"organization/%s/%s","latest":{"manifest":{"time":"0001-01-01T00:00:00Z","magic":"","version":""},"resources":[{"urn":"urn:pulumi:%s::%s::pulumi:pulumi:Stack::%s","type":"pulumi:pulumi:Stack"}]}}}`,
		project, name, name, project, marker,
	)
}

// TestNewPulumiWorkspace_WithStateDirRoot covers a workspace whose
// state dir is rooted at an injected path.
func TestNewPulumiWorkspace_WithStateDirRoot(t *testing.T) {
	// construct a workspace under an injected temp root
	root := t.TempDir()
	w := NewPulumiWorkspace("instance-a", "oke", WithStateDirRoot(root))

	// assert name, project, and injected root
	assert.Equal(t, "instance-a", w.RuntimeInstanceName)
	assert.Equal(t, "oke", w.ProjectName)
	assert.Equal(t, root, w.stateDirRoot)

	// resolve the state file path
	path, err := w.GetStateFilePath()
	require.NoError(t, err)
	// assert the path is under the injected root
	assert.Equal(
		t,
		filepath.Join(root, "instance-a", ".pulumi", "stacks", "oke", "instance-a.json"),
		path,
	)

	// assert path resolution created the instance state dir
	assert.Equal(t, filepath.Join(root, "instance-a"), w.stateDir)
	info, err := os.Stat(w.stateDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestNewPulumiWorkspace_DefaultRoot covers a workspace that stores
// state under the process home directory.
func TestNewPulumiWorkspace_DefaultRoot(t *testing.T) {
	// redirect HOME to a temp dir
	home := t.TempDir()
	t.Setenv("HOME", home)

	// construct a workspace with the default root
	w := NewPulumiWorkspace("instance-b", "eks")

	// resolve the state file path
	path, err := w.GetStateFilePath()
	require.NoError(t, err)

	// assert the path is under HOME/.threeport/pulumi-state
	wantSuffix := filepath.Join(
		".threeport", "pulumi-state", "instance-b",
		".pulumi", "stacks", "eks", "instance-b.json",
	)
	assert.True(
		t, strings.HasSuffix(path, wantSuffix),
		"path %q should end with %q", path, wantSuffix,
	)
	assert.True(
		t, strings.HasPrefix(path, home),
		"path %q should be under the redirected home dir %q", path, home,
	)

	// assert path resolution created the instance state dir under HOME
	info, err := os.Stat(filepath.Join(home, ".threeport", "pulumi-state", "instance-b"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestGetStateFilePath_EmptyName rejects a workspace with an empty
// runtime instance name.
func TestGetStateFilePath_EmptyName(t *testing.T) {
	// construct a workspace with an empty name
	root := t.TempDir()
	w := NewPulumiWorkspace("", "oke", WithStateDirRoot(root))

	// resolve the state file path
	path, err := w.GetStateFilePath()
	// assert empty name is rejected with no path
	require.Error(t, err)
	assert.Empty(t, path)
	assert.Contains(t, err.Error(), "runtime instance name is empty")

	// assert the empty-name guard created no files under the root
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestSetStackState_CheckpointRoundTrip covers writing checkpoint
// state and reading the same bytes back from disk.
func TestSetStackState_CheckpointRoundTrip(t *testing.T) {
	// skip without the pulumi CLI
	requirePulumiCLI(t)

	// construct a workspace under a temp root
	root := t.TempDir()
	w := NewPulumiWorkspace("ckpt-instance", "ckptproj", WithStateDirRoot(root))

	// write checkpoint state
	state := checkpointState("ckptproj", "ckpt-instance", "round-trip")
	require.NoError(t, w.SetStackState(jsonPtr(state)))

	// assert the state file matches the written bytes
	path, err := w.GetStateFilePath()
	require.NoError(t, err)
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, state, string(onDisk), "checkpoint state must land on disk byte-identical")

	// assert ReadStateFile returns the same bytes
	readBack, err := w.ReadStateFile()
	require.NoError(t, err)
	require.NotNil(t, readBack)
	assert.Equal(t, state, string(*readBack))
}

// TestSetStackState_AtomicTempThenRename covers a successful write
// leaving no temp file, and a failed temp write leaving prior state.
func TestSetStackState_AtomicTempThenRename(t *testing.T) {
	// skip without the pulumi CLI
	requirePulumiCLI(t)

	// construct a workspace under a temp root
	root := t.TempDir()
	w := NewPulumiWorkspace("atomic-instance", "atomicproj", WithStateDirRoot(root))

	// write the first checkpoint
	first := checkpointState("atomicproj", "atomic-instance", "first")
	require.NoError(t, w.SetStackState(jsonPtr(first)))

	// assert the first checkpoint is on disk with no leftover temp file
	path, err := w.GetStateFilePath()
	require.NoError(t, err)
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, first, string(onDisk))
	_, statErr := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "no temp file may remain after a successful write")

	// occupy the temp path with a directory so the next write fails
	require.NoError(t, os.Mkdir(path+".tmp", 0755))
	// write a second checkpoint
	second := checkpointState("atomicproj", "atomic-instance", "second")
	err = w.SetStackState(jsonPtr(second))
	// assert the failed write names the temp file and leaves first intact
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write temporary state file")
	onDisk, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, first, string(onDisk), "failed temp write must leave the previous state intact")
	// remove the occupied temp path
	require.NoError(t, os.Remove(path+".tmp"))
}

// TestSetStackState_ExportFormatRequiresBackend covers export-format
// state stored as a checkpoint and re-exported as a deployment.
func TestSetStackState_ExportFormatRequiresBackend(t *testing.T) {
	// skip without the pulumi CLI
	requirePulumiCLI(t)

	// construct a workspace under a temp root
	root := t.TempDir()
	w := NewPulumiWorkspace("export-instance", "exportproj", WithStateDirRoot(root))

	// write export-format state
	exportState := `{"version":3,"deployment":{"manifest":{"time":"0001-01-01T00:00:00Z","magic":"","version":""}}}`
	require.NoError(t, w.SetStackState(jsonPtr(exportState)))

	// assert the on-disk file is checkpoint format, not export format
	onDisk, err := w.ReadStateFile()
	require.NoError(t, err)
	require.NotNil(t, onDisk)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(*onDisk, &parsed))
	assert.Contains(t, parsed, "checkpoint")
	assert.NotContains(t, parsed, "deployment")

	// assert GetStackState returns export-format version 3
	stateJSON, err := w.GetStackState()
	require.NoError(t, err)
	require.NotNil(t, stateJSON)
	var deployment apitype.UntypedDeployment
	require.NoError(t, json.Unmarshal(*stateJSON, &deployment))
	assert.Equal(t, 3, deployment.Version)
	assert.NotNil(t, deployment.Deployment)
}

// TestPulumiWorkspace_ZeroValueStillWorks covers a workspace built as
// a struct literal with no constructor options.
func TestPulumiWorkspace_ZeroValueStillWorks(t *testing.T) {
	// redirect HOME to a temp dir
	home := t.TempDir()
	t.Setenv("HOME", home)

	// construct a workspace from a struct literal
	w := &PulumiWorkspace{
		RuntimeInstanceName: "instance-z",
		ProjectName:         "gke",
	}

	// resolve the state file path
	path, err := w.GetStateFilePath()
	require.NoError(t, err)

	// assert the path is under HOME/.threeport/pulumi-state
	wantSuffix := filepath.Join(
		".threeport", "pulumi-state", "instance-z",
		".pulumi", "stacks", "gke", "instance-z.json",
	)
	assert.True(
		t, strings.HasSuffix(path, wantSuffix),
		"path %q should end with %q", path, wantSuffix,
	)
	assert.True(
		t, strings.HasPrefix(path, home),
		"path %q should be under the redirected home dir %q", path, home,
	)
}

// TestResolveStateDir_EmptyName rejects an empty runtime instance name
// from path, create, and delete methods, and reports no state dir.
func TestResolveStateDir_EmptyName(t *testing.T) {
	// construct a workspace with an empty name
	w := NewPulumiWorkspace("", "oke", WithStateDirRoot(t.TempDir()))

	// resolve the state directory
	dir, err := w.resolveStateDir()
	// assert empty name is rejected with no directory
	require.Error(t, err)
	assert.Empty(t, dir)
	assert.Contains(t, err.Error(), "runtime instance name is empty")

	// assert setStateDir rejects an empty name
	require.Error(t, w.setStateDir(), "setStateDir must refuse an empty name")

	// assert GetStateFilePath rejects an empty name
	path, err := w.GetStateFilePath()
	require.Error(t, err)
	assert.Empty(t, path)

	// assert HasStateDir is false for an empty name
	assert.False(t, w.HasStateDir(), "an unnamed workspace claims no state dir")
	// assert DeleteStackState rejects an empty name
	require.Error(t, w.DeleteStackState(), "DeleteStackState must refuse an empty name")
}

// TestStateDirRoot_HonoredByEveryMethod covers create, detect, and
// delete of a state dir under an injected root.
func TestStateDirRoot_HonoredByEveryMethod(t *testing.T) {
	// construct a workspace under an injected temp root
	root := t.TempDir()
	w := NewPulumiWorkspace("instance-a", "oke", WithStateDirRoot(root))

	// assert no state dir exists yet
	assert.False(t, w.HasStateDir(), "no state dir exists before one is created")

	// create the state dir
	require.NoError(t, w.setStateDir())
	stateDir := filepath.Join(root, "instance-a")
	// assert the dir landed under the injected root and is visible
	assert.DirExists(t, stateDir, "the state dir lands under the injected root")
	assert.True(t, w.HasStateDir(), "the state dir is visible once created")

	// write a marker file in the state dir
	marker := filepath.Join(stateDir, "marker")
	require.NoError(t, os.WriteFile(marker, []byte("state"), 0644))

	// delete the state dir
	require.NoError(t, w.DeleteStackState())
	// assert the dir is gone and HasStateDir is false
	assert.NoDirExists(t, stateDir, "deletion removes the dir under the injected root")
	assert.False(t, w.HasStateDir(), "the state dir is gone after deletion")
}

// TestDeleteStackState_MissingDirIsNotAnError accepts delete of a
// state dir that was never created.
func TestDeleteStackState_MissingDirIsNotAnError(t *testing.T) {
	// construct a workspace whose state dir does not exist
	w := NewPulumiWorkspace("never-created", "oke", WithStateDirRoot(t.TempDir()))

	// assert delete of a missing dir is not an error
	assert.NoError(t, w.DeleteStackState())
}
