package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// init registers the module image migration.
func init() {
	// register the up and down steps
	goose.AddMigrationNoTxContext(Up000009, Down000009)
}

// moduleImageColumns are the repository and tag a module was installed from.
var moduleImageColumns = [][2]string{
	{"image_namespace", "STRING"},
	{"image_tag", "STRING"},
}

// Up000009 adds the image repository and tag a module was installed from.
// Cockroach rejects ADD COLUMN inside a transaction, so this migration runs outside one.
// 000001 builds its tables from the current models, so a database it created after
// the fields were added has them already, and each is added only where it is missing.
func Up000009(ctx context.Context, db *sql.DB) error {
	// add the image a module was installed from
	for _, column := range moduleImageColumns {
		statement := fmt.Sprintf(
			"ALTER TABLE v0_module_apis ADD COLUMN IF NOT EXISTS %s %s",
			column[0], column[1],
		)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to add module image columns: %w", err)
		}
	}
	return nil
}

// Down000009 drops the recorded module image.
func Down000009(ctx context.Context, db *sql.DB) error {
	// drop the recorded image
	for _, column := range moduleImageColumns {
		statement := fmt.Sprintf(
			"ALTER TABLE v0_module_apis DROP COLUMN IF EXISTS %s",
			column[0],
		)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to drop module image columns: %w", err)
		}
	}
	return nil
}
