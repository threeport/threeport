package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

func init() {
	goose.AddMigrationNoTxContext(Up00002, Down00002)
}

func getSecretInterfaces() []interface{} {
	return []interface{}{
		&v0.SecretDefinition{},
		&v0.SecretInstance{},
	}
}

func Up00005(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(getSecretInterfaces()...); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down00005(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	for _, table := range getSecretInterfaces() {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}
