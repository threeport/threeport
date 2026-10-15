package migrations

import (
	"context"
	"database/sql"
	"fmt"

	goose "github.com/pressly/goose/v3"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// init registers the migration with goose at startup.
func init() {
	goose.AddMigrationNoTxContext(Up000002, Down000002)
}

// Up000002 adds Tier to ControlPlaneDefinition.
//
// The column is added on its own rather than through AutoMigrate. AutoMigrate
// reconciles the whole table, and on a fresh install - where 000001 already
// created it from the current struct, Tier included - it goes looking for a
// unique constraint named uni_v0_control_plane_definitions_name that
// CockroachDB never created, because a unique field there becomes a unique
// index instead. The migration then fails and the control plane does not
// start, on exactly the installs that had nothing to migrate.
//
// Editing 000001 instead would work on a fresh install and do nothing on an
// existing one, because goose records it as applied and never runs it again.
//
// Rows that predate the column take the struct's default of development. That
// is the safe direction: tier gates production-only behavior, so an unknown
// tier should withhold it rather than switch it on for a control plane whose
// operator never asked for it.
func Up000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if gormDb.Migrator().HasColumn(&v0.ControlPlaneDefinition{}, "Tier") {
		return nil
	}
	if err := gormDb.Migrator().AddColumn(&v0.ControlPlaneDefinition{}, "Tier"); err != nil {
		return fmt.Errorf("could not add the tier column to control plane definitions: %w", err)
	}

	return nil
}

// Down000002 removes the column.
func Down000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if !gormDb.Migrator().HasColumn(&v0.ControlPlaneDefinition{}, "Tier") {
		return nil
	}
	if err := gormDb.Migrator().DropColumn(&v0.ControlPlaneDefinition{}, "Tier"); err != nil {
		return fmt.Errorf("could not drop column tier: %w", err)
	}

	return nil
}
