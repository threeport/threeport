package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/version"
)

// startupHelperEnv, when set, makes TestStartupHelperProcess run tptctl's real
// Execute with the arguments in startupHelperArgs instead of being a test.
const (
	startupHelperEnv  = "TPTCTL_STARTUP_HELPER"
	startupHelperArgs = "TPTCTL_STARTUP_ARGS"
)

// TestStartupHelperProcess is the process runTptctl starts.  It is not a test
// on its own: it runs Execute, which reads os.Args and which exits the process
// on its error paths, so it has to run in a process of its own.
func TestStartupHelperProcess(t *testing.T) {
	if os.Getenv(startupHelperEnv) != "1" {
		return
	}

	os.Args = append([]string{"tptctl"}, strings.Fields(os.Getenv(startupHelperArgs))...)
	Execute()

	// exit here so that the testing package does not print PASS to stdout
	os.Exit(0)
}

// startupRun is what a run of tptctl wrote and exited with.
type startupRun struct {
	stdout   string
	stderr   string
	exitCode int
}

// runTptctl runs tptctl's startup and the given command in a separate process,
// with a home directory of its own and pluginDir as its plugin directory, and
// returns what it wrote to stdout and stderr.
func runTptctl(t *testing.T, pluginDir string, args string) startupRun {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(executable, "-test.run=^TestStartupHelperProcess$")
	cmd.Env = append(
		os.Environ(),
		startupHelperEnv+"=1",
		startupHelperArgs+"="+args,
		"HOME="+t.TempDir(),
		"THREEPORT_PLUGIN_DIR="+pluginDir,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	run := startupRun{}
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok, "tptctl could not be run: %v", err)
		run.exitCode = exitErr.ExitCode()
	}
	run.stdout = stdout.String()
	run.stderr = stderr.String()

	return run
}

// TestStartupKeepsStdoutForTheCommand covers what is printed while tptctl
// starts up, before the command runs.  Stdout belongs to the command, and
// -o json and -o yaml promise nothing else is on it, so every startup warning
// has to go to stderr.
func TestStartupKeepsStdoutForTheCommand(t *testing.T) {
	clientVersion := version.GetVersion()

	// a plugin dir with a plugin named like a core command
	conflictingDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(conflictingDir, "version"), []byte("#!/bin/sh\n"), 0o755))

	scenarios := []struct {
		name       string
		pluginDir  string
		wantStderr string
	}{
		{
			name:       "no plugin directory",
			pluginDir:  filepath.Join(t.TempDir(), "plugins-that-do-not-exist"),
			wantStderr: "Warning: no plugins installed",
		},
		{
			name:       "a plugin named like a core command",
			pluginDir:  conflictingDir,
			wantStderr: "conflicts with core 'tptctl version' command - plugin ignored",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name+", json", func(t *testing.T) {
			run := runTptctl(t, scenario.pluginDir, "version --client -o json")

			assert.Equal(t, 0, run.exitCode, "stderr: %s", run.stderr)
			var report versionReport
			require.NoError(t, json.Unmarshal([]byte(run.stdout), &report),
				"stdout must be only the JSON document, got: %q", run.stdout)
			assert.Equal(t, clientVersion, report.ClientVersion)
			assert.Contains(t, run.stderr, scenario.wantStderr)
		})

		t.Run(scenario.name+", text", func(t *testing.T) {
			run := runTptctl(t, scenario.pluginDir, "version --client")

			assert.Equal(t, 0, run.exitCode, "stderr: %s", run.stderr)
			assert.Equal(t, "Client Version: "+clientVersion+"\n", run.stdout)
			assert.Contains(t, run.stderr, scenario.wantStderr)
		})
	}
}
