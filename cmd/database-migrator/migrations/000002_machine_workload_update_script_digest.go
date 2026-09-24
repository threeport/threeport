package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"
	"gorm.io/gorm"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// columnUpdateScriptDigest is the column this migration manages.
const columnUpdateScriptDigest = "update_script_digest"

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000002, Down000002)
}

// Up000002 adds the column recording the inputs a machine workload's update
// script last ran successfully with. Without it the reconciler has nothing to
// compare against and runs operator-authored code on every notification.
func Up000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	return addUpdateScriptDigest(gormDb)
}

// addUpdateScriptDigest adds the column when it is not already there.
func addUpdateScriptDigest(gormDb *gorm.DB) error {
	if gormDb.Migrator().HasColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest) {
		return nil
	}
	if err := gormDb.Migrator().AddColumn(
		&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest,
	); err != nil {
		return fmt.Errorf("failed to add %s to machine workload instances: %w", columnUpdateScriptDigest, err)
	}

	return nil
}

// Down000002 removes the column added by Up000002. Existing rows lose the record
// of what their update script last ran with, so the next reconciliation of each
// runs it again.
func Down000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	return dropUpdateScriptDigest(gormDb)
}

// dropUpdateScriptDigest removes the column when it is there.
func dropUpdateScriptDigest(gormDb *gorm.DB) error {
	if !gormDb.Migrator().HasColumn(&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest) {
		return nil
	}
	if err := gormDb.Migrator().DropColumn(
		&v0.MachineWorkloadInstance{}, columnUpdateScriptDigest,
	); err != nil {
		return fmt.Errorf("failed to drop %s from machine workload instances: %w", columnUpdateScriptDigest, err)
	}

	return nil
}
