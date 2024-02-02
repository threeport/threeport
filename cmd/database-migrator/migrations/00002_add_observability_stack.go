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

func getObservabilityStackInterfaces() []interface{} {
	return []interface{}{
		&v0.ObservabilityStackDefinition{},
		&v0.ObservabilityStackInstance{},
		&v0.ObservabilityDashboardDefinition{},
		&v0.ObservabilityDashboardInstance{},
		&v0.MetricsDefinition{},
		&v0.MetricsInstance{},
		&v0.LoggingDefinition{},
		&v0.LoggingInstance{},
	}
}

func Up00002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(getObservabilityStackInterfaces()); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down00002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	tablesToDrop := getObservabilityStackInterfaces()
	for _, table := range tablesToDrop {
		if err := gormDb.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("could not drop table with gorm db: %w", err)
		}
	}

	return nil
}
