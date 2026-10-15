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
// AutoMigrate against the same type the init migration used: gorm compares the
// struct to the table and adds what is missing, so an installation that has
// already run 000001 gains the column and a fresh one is unaffected, having
// been created from the current struct in the first place.
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

	if err := gormDb.AutoMigrate(&v0.ControlPlaneDefinition{}); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate for control plane definition: %w", err)
	}

	return nil
}

// Down000002 removes the column.
func Down000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.Migrator().DropColumn(&v0.ControlPlaneDefinition{}, "tier"); err != nil {
		return fmt.Errorf("could not drop column tier: %w", err)
	}

	return nil
}
