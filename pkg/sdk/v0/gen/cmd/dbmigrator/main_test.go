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

// TestEveryMigratorRollsBackButOnlyTheCoreBacksUp covers which migrators
// back the database up before migrating and roll a failed migration back.
// Every migrator, a module's included, rolls back, which needs nothing from
// the install.  Only the core control plane's backs up, since the backup
// location is part of its install.
func TestEveryMigratorRollsBackButOnlyTheCoreBacksUp(t *testing.T) {
	core := generateMigratorMain(t, &gen.Generator{}, &sdk.SdkConfig{})
	module := generateMigratorMain(
		t,
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"},
	)

	for name, source := range map[string]string{"core": core, "module": module} {
		if !strings.Contains(source, "func upWithRollback(") || !strings.Contains(source, "migrator.Up(ctx, steps, record, opts)") {
			t.Errorf("the %s migrator does not roll a failed migration back", name)
		}
		if !strings.Contains(source, "migrator.NewTableLock(") || !strings.Contains(source, "Lock: lock") {
			t.Errorf("the %s migrator does not take the migration lock, so two of its pods can migrate together", name)
		}
	}
	if !strings.Contains(module, `"threeport_widget_goose_db_version_lock"`) {
		t.Error("a module's migration lock is not kept next to its own goose version table")
	}
	if !strings.Contains(module, `"threeport_widget_goose_db_version_failure"`) {
		t.Error("a module's failure record is not kept next to its own goose version table")
	}
	if !strings.Contains(core, "func databaseBackup(") || !strings.Contains(core, "opts.Backup = backup") {
		t.Error("the core migrator does not back the database up before migrating")
	}
	if strings.Contains(module, "databaseBackup") || strings.Contains(module, "DbBackupURIFilename") {
		t.Error("a module's migrator backs up, naming installer constants its own installer does not declare")
	}
}

// TestEveryMigratorLocksEveryCommandThatChangesTheSchema covers the goose
// commands other than a plain 'up', such as the 'down' the README documents for
// rolling back by hand.  Each that changes the schema holds the migration lock,
// so it cannot run beside an API server pod's migrator applying migrations;
// 'status' only reads, and is not held up behind a running migration.
func TestEveryMigratorLocksEveryCommandThatChangesTheSchema(t *testing.T) {
	core := generateMigratorMain(t, &gen.Generator{}, &sdk.SdkConfig{})
	module := generateMigratorMain(
		t,
		&gen.Generator{Module: true, ModulePath: "example.com/widget-module"},
		&sdk.SdkConfig{ModuleName: "Widget", ApiNamespace: "widget.example.com"},
	)

	for name, source := range map[string]string{"core": core, "module": module} {
		if strings.Contains(source, "} else if err := goose.RunContext(") {
			t.Errorf("the %s migrator runs goose commands other than 'up' without the lock", name)
		}
		if !strings.Contains(source, "runGooseCommand(ctx, db, logger, command, arguments)") ||
			!strings.Contains(source, "migrator.WithLock(ctx, lock,") {
			t.Errorf("the %s migrator does not run its other goose commands under the migration lock", name)
		}
		if !strings.Contains(source, `if command == "status" {`) {
			t.Errorf("the %s migrator holds 'status' behind the migration lock", name)
		}
		if strings.Count(source, "NewTableLock(") != 1 {
			t.Errorf("the %s migrator builds its lock in more than one place, which can drift apart", name)
		}
	}
}
