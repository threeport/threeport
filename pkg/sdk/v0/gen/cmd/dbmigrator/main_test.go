package dbmigrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// generateMigratorMain writes the database migrator's main package for the
// given generator into a scratch directory and returns it.
func generateMigratorMain(t *testing.T, generator *gen.Generator, sdkConfig *sdk.SdkConfig) string {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("failed to change to scratch directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})

	if err := GenDbMigratorMain(generator, sdkConfig); err != nil {
		t.Fatalf("GenDbMigratorMain returned an error: %v", err)
	}
	generated, err := os.ReadFile(filepath.Join("cmd", "database-migrator", "main_gen.go"))
	if err != nil {
		t.Fatalf("failed to read generated migrator: %v", err)
	}

	return string(generated)
}

// TestOnlyTheCoreMigratorCreatesTheBackupSchedule covers which migrators
// create the database backup schedule.  A module's migrator refers to its
// own installer package, which has no backup schedule file to mount, so the
// schedule code is left out of it rather than naming a constant that package
// does not declare.
func TestOnlyTheCoreMigratorCreatesTheBackupSchedule(t *testing.T) {
	core := generateMigratorMain(t, &gen.Generator{}, &sdk.SdkConfig{})
	for _, want := range []string{"func createBackupSchedule(", "createBackupSchedule(gormDb, logger)", "DbBackupScheduleFilename"} {
		if !strings.Contains(core, want) {
			t.Errorf("the core migrator does not contain %q", want)
		}
	}

	module := generateMigratorMain(
		t,
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"},
	)
	for _, unwanted := range []string{"createBackupSchedule", "DbBackupScheduleFilename"} {
		if strings.Contains(module, unwanted) {
			t.Errorf("a module's migrator contains %q", unwanted)
		}
	}
	if !strings.Contains(module, "DbInitFilename") {
		t.Error("a module's migrator no longer runs its init script")
	}
}
