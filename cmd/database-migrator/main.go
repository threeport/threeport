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

	db_host := database.DB_HOST
	db_user := database.DB_USER
	db_name := database.DB_NAME
	db_port := database.DB_PORT
	db_ssl_mode := database.DB_SSL_MODE
	db_password := database.DB_PASSWORD

	// env vars for database and nats connection
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			cli.Error("failed to load environment variables.", err)
			os.Exit(1)
		}

		if dbhost, ok := os.LookupEnv("DB_HOST"); ok {
			db_host = dbhost
		}

		if dbuser, ok := os.LookupEnv("DB_USER"); ok {
			db_user = dbuser
		}

		if dbpw, ok := os.LookupEnv("DB_PASSWORD"); ok {
			db_password = dbpw
		}

		if dbname, ok := os.LookupEnv("DB_NAME"); ok {
			db_name = dbname
		}

		if dbport, ok := os.LookupEnv("DB_PORT"); ok {
			db_port = dbport
		}

		if sslmode, ok := os.LookupEnv("DB_SSL_MODE"); ok {
			db_ssl_mode = sslmode
		}
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", db_host, db_port, db_user, db_password, db_name, db_ssl_mode)

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
