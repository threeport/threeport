// This is custom goose binary with sqlite3 support only.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	_ "github.com/threeport/threeport/cmd/database-migrator/migrations"
	"github.com/threeport/threeport/pkg/api-server/v0/database"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	log "github.com/threeport/threeport/pkg/log/v0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	DB_HOST     = "localhost"
	DB_USER     = "tp_rest_api"
	DB_PASSWORD = "tp-rest-api-pwd"
	DB_NAME     = "threeport_api"
	DB_PORT     = 26257
)

var AllowedCommands []string = []string{"up", "up-to", "up-by-one", "down", "down-to", "redo", "status"}

func main() {
	if len(os.Args) < 2 {
		cli.Error(fmt.Sprintf("please provide one of available commands: %s", strings.Join(AllowedCommands[:], ",")), nil)
		os.Exit(1)
	}

	args := os.Args[1:]

	command := args[0]
	found := false

	for _, c := range AllowedCommands {
		if command == c {
			found = true
		}
	}

	if !found {
		cli.Error(fmt.Sprintf("provided command not in list of commands: %s", strings.Join(AllowedCommands[:], ",")), nil)
		os.Exit(1)
	}

	dir := "."

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)

	db, err := goose.OpenDBWithDriver("postgres", dsn)
	if err != nil {
		cli.Error("goose: failed to open DB:\n", err)
		os.Exit(1)
	}

	defer func() {
		if err := db.Close(); err != nil {
			cli.Error("goose: failed to close DB:\n", err)
			os.Exit(1)
		}
	}()

	arguments := []string{}
	if len(args) > 1 {
		arguments = args[1:]
	}

	ctx := context.TODO()

	logger, err := log.NewLogger(false)
	if err != nil {
		cli.Error("could not create logger:\n", err)
		os.Exit(1)
	}

	gormdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: &database.ZapLogger{Logger: &logger},
		NowFunc: func() time.Time {
			utc, _ := time.LoadLocation("UTC")
			return time.Now().In(utc).Truncate(time.Microsecond)
		},
	})
	if err != nil {
		cli.Error("could not create gorm db object:\n", err)
		os.Exit(1)
	}

	ctx = context.WithValue(ctx, "gormdb", gormdb)

	goose.SetTableName("threeport_goose_db_version")

	if err := goose.RunContext(ctx, command, db, dir, arguments...); err != nil {
		cli.Error(fmt.Sprintf("goose context run failed %s:", command), err)
		os.Exit(1)
	}
}

func getGormDbFromContext(ctx context.Context) *gorm.DB {
	contextGorm := ctx.Value("gormdb")
	if contextGorm == nil {
		return nil
	}

	var gormDb *gorm.DB
	if g, ok := contextGorm.(*gorm.DB); ok {
		gormDb = g
	}

	return gormDb
}
