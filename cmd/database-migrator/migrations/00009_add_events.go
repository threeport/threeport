package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	v1 "github.com/threeport/threeport/pkg/api/v1"
)

func init() {
	goose.AddMigrationNoTxContext(Up00009, Down00009)
}

func getEventInterface() []interface{} {
	return []interface{}{
		&v1.Event{},
	}
}

func Up00009(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(getEventInterface()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down00009(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	for _, table := range getEventInterface() {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}
