package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

func init() {
	goose.AddMigrationNoTxContext(Up00001, Down00001)
}

func Up00001(ctx context.Context, db *sql.DB) error {
	contextGorm := ctx.Value("gormdb")
	if contextGorm == nil {
		return fmt.Errorf("could not retrieve gormdb from ctx")
	}

	var gormDb *gorm.DB
	if g, ok := contextGorm.(*gorm.DB); ok {
		gormDb = g
	}

	fmt.Println(gormDb.Config.DisableAutomaticPing)

	return nil
}

func Down00001(ctx context.Context, db *sql.DB) error {
	return nil
}
