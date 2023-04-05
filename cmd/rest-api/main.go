package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nats-io/nats.go"

	//"github.com/threeport/threeport/internal/authority"
	iapi "github.com/threeport/threeport/internal/api"
	"github.com/threeport/threeport/internal/api/database"
	_ "github.com/threeport/threeport/internal/api/docs"
	"github.com/threeport/threeport/internal/api/handlers"
	"github.com/threeport/threeport/internal/api/routes"
	"github.com/threeport/threeport/internal/api/versions"
	"github.com/threeport/threeport/internal/version"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// @title Threeport RESTful API
// @version v0.0.6
// @description Threeport RESTful API.
// @termsOfService https://threeport.io/api-tos/
// @contact.name Threeport Admin
// @contact.url https://threeport.io/support
// @contact.email support@threeport.io
// @host rest-api.threeport.io
// @BasePath /
//
//go:generate ../../bin/threeport-codegen api-version v0
//go:generate swag init --dir ./,../../pkg/api,../../internal/api --parseDependency --generalInfo main.go --output ../../internal/api/docs
func main() {
	// flags
	var envFile string
	var autoMigrate bool
	flag.StringVar(&envFile, "env-file", "/etc/threeport/env", "File from which to load environment")
	flag.BoolVar(&autoMigrate, "auto-migrate", false, "If true API server will auto migrate DB schema")
	flag.Parse()

	e := echo.New()
	e.HideBanner = true

	var validate *validator.Validate
	validate = validator.New()
	validate.RegisterValidation("optional", iapi.IsOptional)
	validate.RegisterValidation("association", iapi.IsAssociation)
	validate.RegisterValidation("ISO8601date", iapi.IsISO8601Date)
	e.Validator = &iapi.CustomValidator{Validator: validate}

	// middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	//e.Use(iapi.AuthorizationTokenCheck)

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &iapi.CustomContext{Context: c}
			return next(cc)
		}
	})

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		// Take required information from error and context and send it to a service like New Relic etc.
		fmt.Println(c.Path(), c.QueryParams(), err.Error())

		// Call the default handler to return the HTTP response
		e.DefaultHTTPErrorHandler(err, c)
	}

	// env vars for database and nats connection
	if err := godotenv.Load(envFile); err != nil {
		e.Logger.Fatalf("failed to load environment variables: %v", err)
	}

	// database connection
	db, err := database.Init(autoMigrate)
	if err != nil {
		e.Logger.Fatalf("failed to initialize database: %v", err)
	}

	//// authority
	//authority.Auth = authority.New(autoMigrate, authority.Options{
	//	TablesPrefix: "rbac_",
	//	DB:           db,
	//})
	//if authority.Auth == nil {
	//	e.Logger.Fatalf("failed to initialize RBAC DB: %v", err)
	//}

	// enable temporarily only to populate DB with initial authorization mapping
	//err = initRbac(authority.Auth)
	//if err != nil {
	//	e.Logger.Fatalf("failed to initialize RBAC: %v", err)
	//}

	// nats connection
	natsConn := fmt.Sprintf(
		"nats://%s:%s@%s:%s",
		os.Getenv("NATS_USER"),
		os.Getenv("NATS_PASSWORD"),
		os.Getenv("NATS_HOST"),
		os.Getenv("NATS_PORT"),
	)
	nc, err := nats.Connect(natsConn)
	if err != nil {
		e.Logger.Fatalf("failed to establish nats connection: %v", err)
	}
	defer nc.Close()

	// jetstream context
	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		e.Logger.Fatalf("failed to create jetstream context: %v", err)
	}

	// add stream
	js.AddStream(&nats.StreamConfig{
		Name:     v0.WorkloadStreamName,
		Subjects: v0.GetWorkloadSubjects(),
	})

	// handlers
	h := handlers.New(db, nc, js)

	// routes
	routes.AddRoutes(e, &h)
	routes.SwaggerRoutes(e)
	routes.VersionRoutes(e, &h)

	// add version info for queries to /<object>/versions
	iapi.Versions[0] = iapi.V0
	versions.AddVersions()

	fmt.Printf("\nThreeport REST API: %s\n", version.GetVersion())
	e.Logger.Fatal(e.Start(":1323"))
}
