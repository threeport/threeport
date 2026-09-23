package v0

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is tracked at test/module/testdata and copied into the generated
// config package before the module is built. It covers the generated
// database-migrator, which is a main package: nothing can import it, so
// building it and running it is the only way to reach what it does.

// TestDatabaseMigratorWithoutACommand covers invoking the generated
// database-migrator with no command.
//
// The generated code read args[0] before checking that an argument was there,
// so this panicked rather than printing usage. Every module generates this
// binary, so the panic reached all of them.
func TestDatabaseMigratorWithoutACommand(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "database-migrator")
	build := exec.Command("go", "build", "-o", binary, "../../../cmd/database-migrator")
	buildOutput, err := build.CombinedOutput()
	require.NoErrorf(t, err, "failed to build the generated database-migrator: %s", buildOutput)

	// an empty env file lets the -env-file case get past loading it and reach
	// the same argument handling as the bare invocation
	envFile := filepath.Join(t.TempDir(), "env")
	require.NoError(t, os.WriteFile(envFile, []byte{}, 0644))

	for name, args := range map[string][]string{
		"no arguments at all": {},
		"only an env file":    {"-env-file=" + envFile},
	} {
		t.Run(name, func(t *testing.T) {
			output, err := exec.Command(binary, args...).CombinedOutput()

			// it has to fail, but by its own error path rather than a panic:
			// a panic exits 2 and prints a stack trace instead of usage
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "expected a non-zero exit, output: %s", output)
			require.Equal(t, 1, exitErr.ExitCode(), "output: %s", output)

			require.NotContains(t, string(output), "panic:", "it panicked instead of printing usage")
			require.Contains(t, string(output), "no command provided")
			require.Contains(t, string(output), "usage: database-migrator")
		})
	}
}
