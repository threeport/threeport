package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func init() {
	goose.AddMigrationNoTxContext(Up00012, Down00012)
}

func getAzureInterfaces() []interface{} {
	return []interface{}{
		&v0.AzureAccount{},
		&v0.AzureAksKubernetesRuntimeDefinition{},
		&v0.AzureAksKubernetesRuntimeInstance{},
	}
}

func Up00012(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(getAzureInterfaces()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down00012(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	tablesToDrop := getAzureInterfaces()
	for _, table := range tablesToDrop {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}
