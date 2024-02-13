package migrations

import (
	"context"
	"database/sql"
	"fmt"

	tp041_db "github.com/threeport/threeport/041/pkg/api-server/v0/database"

	"github.com/pressly/goose/v3"
	"github.com/threeport/threeport/pkg/api-server/v0/database"
)

func init() {
	goose.AddMigrationNoTxContext(Up00001, Down00001)
}

func Up00001(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(tp041_db.GetDbInterfaces()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down00001(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	tablesToDrop := database.GetDbInterfaces()
	for _, table := range tablesToDrop {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}
