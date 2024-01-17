package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationNoTxContext(Up00002, Down00002)
}

func Up00002(ctx context.Context, db *sql.DB) error {
	// contextGorm := ctx.Value("gormdb")
	// if contextGorm == nil {
	// 	return fmt.Errorf("could not retrieve gormdb from ctx")
	// }

	// var gormDb *gorm.DB
	// if g, ok := contextGorm.(*gorm.DB); ok {
	// 	gormDb = g
	// }

	return nil
}

func Down00002(ctx context.Context, db *sql.DB) error {
	return nil
}
