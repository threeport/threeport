package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func init() {
	goose.AddMigrationNoTxContext(Up00004, Down00004)
}

func getForwardProxyInterfaces() []interface{} {
	return []interface{}{
		&v0.ForwardProxyDefinition{},
		&v0.ForwardProxyInstance{},
	}
}

func Up00004(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	tablesToDrop := getForwardProxyInterfaces()
	for _, table := range tablesToDrop {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}

func Down00004(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(getForwardProxyInterfaces()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}
