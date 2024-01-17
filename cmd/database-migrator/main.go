// This is custom goose binary with sqlite3 support only.

package main

import (
	"context"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

const (
	DB_HOST     = "localhost"
	DB_USER     = "tp_rest_api"
	DB_PASSWORD = "tp-rest-api-pwd"
	DB_NAME     = "threeport_api"
	DB_PORT     = 26257
)

func main() {
	args := os.Args[1:]

	if len(args) != 1 {
		fmt.Println("please specify 1 migration command for the migrator library")
		return
	}

	command := args[0]
	dir := "."

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)

	db, err := goose.OpenDBWithDriver("postgres", dsn)
	if err != nil {
		cli.Error("goose: failed to open DB:\n", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			cli.Error("goose: failed to close DB:\n", err)
		}
	}()

	arguments := []string{}

	ctx := context.TODO()

	// logger, err := log.NewLogger(false)
	// if err != nil {
	// 	cli.Error("could not create logger:\n", err)
	// }

	// gormdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
	// 	Logger: &database.ZapLogger{Logger: &logger},
	// 	NowFunc: func() time.Time {
	// 		utc, _ := time.LoadLocation("UTC")
	// 		return time.Now().In(utc).Truncate(time.Microsecond)
	// 	},
	// })
	// if err != nil {
	// 	cli.Error("could not create gorm db object:\n", err)
	// }

	// ctx = context.WithValue(ctx, "gormdb", gormdb)

	goose.SetTableName("threeport_goose_db_version")

	if err := goose.RunContext(ctx, command, db, dir, arguments...); err != nil {
		cli.Error(fmt.Sprintf("goose context run failed %s:", command), err)
	}
}

// func getGormDbFromContext(ctx context.Context) *gorm.DB {
// 	contextGorm := ctx.Value("gormdb")
// 	if contextGorm == nil {
// 		return nil
// 	}

// 	var gormDb *gorm.DB
// 	if g, ok := contextGorm.(*gorm.DB); ok {
// 		gormDb = g
// 	}

// 	return gormDb
// }
