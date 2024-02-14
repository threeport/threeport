package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func init() {
	goose.AddMigrationNoTxContext(Up00003, Down00003)
}

func Up00003(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.Migrator().AddColumn(v0.ControlPlaneInstance{}, "version"); err != nil {
		return fmt.Errorf("could not add version column to control plane instance: %w", err)
	}

	return nil
}

func Down00003(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.Migrator().DropColumn(v0.ControlPlaneInstance{}, "version"); err != nil {
		return fmt.Errorf("could not drop version column from control plane instance: %w", err)
	}

	return nil
}
