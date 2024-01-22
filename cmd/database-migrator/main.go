// This is custom goose binary with sqlite3 support only.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	_ "github.com/threeport/threeport/cmd/database-migrator/migrations"
	"github.com/threeport/threeport/pkg/api-server/v0/database"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	log "github.com/threeport/threeport/pkg/log/v0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB_HOST     = "localhost"
	DB_USER     = "tp_rest_api"
	DB_PASSWORD = "tp-rest-api-pwd"
	DB_NAME     = "threeport_api"
	DB_PORT     = "26257"
	DB_SSL_MODE = "disable"

	AllowedCommands = []string{"up", "up-to", "up-by-one", "down", "down-to", "redo", "status"}
	envFile         = ""
)

func main() {
	flag.StringVar(&envFile, "env-file", "", "File from which to load environment")
	flag.Parse()

	args := flag.Args()

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

	// env vars for database and nats connection
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			cli.Error("failed to load environment variables.", err)
			os.Exit(1)
		}

		if db_host, ok := os.LookupEnv("DB_HOST"); ok {
			DB_HOST = db_host
		}

		if db_user, ok := os.LookupEnv("DB_USER"); ok {
			DB_USER = db_user
		}

		if db_name, ok := os.LookupEnv("DB_NAME"); ok {
			DB_NAME = db_name
		}

		if db_port, ok := os.LookupEnv("DB_PORT"); ok {
			DB_PORT = db_port
		}

		if db_ssl_mode, ok := os.LookupEnv("DB_SSL_MODE"); ok {
			DB_SSL_MODE = db_ssl_mode
		}
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE)

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
