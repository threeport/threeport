package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	util "github.com/threeport/threeport/cmd/rest-api/util"
	"github.com/threeport/threeport/internal/version"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	"github.com/threeport/threeport/pkg/api-server/v0/database"
	_ "github.com/threeport/threeport/pkg/api-server/v0/docs"
	"github.com/threeport/threeport/pkg/api-server/v0/handlers"
	"github.com/threeport/threeport/pkg/api-server/v0/routes"
	"github.com/threeport/threeport/pkg/api-server/v0/versions"
	handlers_v1 "github.com/threeport/threeport/pkg/api-server/v1/handlers"
	routes_v1 "github.com/threeport/threeport/pkg/api-server/v1/routes"
	versions_v1 "github.com/threeport/threeport/pkg/api-server/v1/versions"
	log "github.com/threeport/threeport/pkg/log/v0"
)

// @title Threeport RESTful API
// @version v0.5.3
// @description Threeport RESTful API.
// @termsOfService https://threeport.io/api-tos/
// @contact.name Threeport Admin
// @contact.url https://threeport.io/support
// @contact.email support@threeport.io
// @host rest-api.threeport.io
// @BasePath /
func main() {
	// flags
	var envFile string
	var autoMigrate bool
	var verbose bool
	var authEnabled bool
	flag.StringVar(&envFile, "env-file", "/etc/threeport/env", "File from which to load environment")
	flag.BoolVar(&autoMigrate, "auto-migrate", false, "If true API server will auto migrate DB schema")
	flag.BoolVar(&verbose, "verbose", false, "Write logs with v(1).InfoLevel and above")
	flag.BoolVar(&authEnabled, "auth-enabled", true, "Enable client certificate authentication")
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
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &iapi.CustomContext{Context: c}
			return next(cc)
		}
	})
	logger, err := log.NewLogger(verbose)
	if err != nil {
		panic(err)
	}
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:   true,
		LogURI:      true,
		LogStatus:   true,
		LogRemoteIP: true,
		LogHost:     true,
		LogLatency:  true,
		LogError:    true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.Info("request",
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", v.Status),
				zap.String("remoteIP", v.RemoteIP),
				zap.String("host", v.Host),
				zap.Duration("latency", v.Latency),
				zap.Error(v.Error),
			)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		// call the default handler to return the HTTP response
		e.DefaultHTTPErrorHandler(err, c)
	}

	// env vars for database and nats connection
	if err := godotenv.Load(envFile); err != nil {
		e.Logger.Fatalf("failed to load environment variables: %v", err)
	}

	// database connection
	db, err := database.Init(autoMigrate, &logger)
	if err != nil {
		e.Logger.Fatalf("failed to initialize database: %v", err)
	}

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
	js, err := util.InitJetStream(nc)
	if err != nil {
		e.Logger.Fatalf("failed to initialize nats jet stream: %v", err)
	}

	// v0
	// handlers
	h := handlers.New(db, nc, *js)

	// routes
	routes.AddRoutes(e, &h)
	routes.AddCustomRoutes(e, &h)
	routes.SwaggerRoutes(e)
	routes.VersionRoutes(e, &h)

	// v1
	// handlers
	h_v1 := handlers_v1.New(db, nc, *js)

	// routes
	routes_v1.AddRoutes(e, &h_v1)
	routes_v1.AddCustomRoutes(e, &h_v1)

	// add version info for queries to /<object>/versions
	iapi.Versions[0] = iapi.V0
	iapi.Versions[1] = "v1"

	versions.AddVersions()
	versions_v1.AddVersions()

	if authEnabled {
		configDir := "/etc/threeport"

		// load server certificate and private key
		cert, err := tls.LoadX509KeyPair(filepath.Join(configDir, "cert/tls.crt"), filepath.Join(configDir, "cert/tls.key"))
		if err != nil {
			e.Logger.Fatal(err)
		}

		// load server root certificate authority
		caCert, err := os.ReadFile(filepath.Join(configDir, "ca/tls.crt"))
		if err != nil {
			e.Logger.Fatal(err)
		}

		// create certificate pool and add server root certificate authority
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		// configure https server
		server := http.Server{
			Addr:    ":1323",
			Handler: e,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCertPool,
				ClientCAs:    caCertPool,
				ClientAuth:   tls.RequireAndVerifyClientCert,
			},
		}

		fmt.Printf("\nThreeport REST API: %s\n", version.GetVersion())
		configureHealthCheckEndpoint()
		if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	} else {

		// configure http server
		server := http.Server{
			Addr:    ":1323",
			Handler: e,
		}

		fmt.Printf("\nThreeport REST API: %s\n", version.GetVersion())
		configureHealthCheckEndpoint()
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}
}

// configureHealthCheckEndpoint sets up a health check endpoint for the API server
func configureHealthCheckEndpoint() {

	// set up health check endpoint
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	go http.ListenAndServe(":8081", nil)
}
