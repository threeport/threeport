// generated by 'threeport-sdk gen' - do not edit

package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	validator "github.com/go-playground/validator/v10"
	godotenv "github.com/joho/godotenv"
	echo "github.com/labstack/echo/v4"
	middleware "github.com/labstack/echo/v4/middleware"
	natsgo "github.com/nats-io/nats.go"
	util "github.com/threeport/threeport/cmd/rest-api/util"
	version "github.com/threeport/threeport/internal/version"
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	database "github.com/threeport/threeport/pkg/api-server/v0/database"
	_ "github.com/threeport/threeport/pkg/api-server/v0/docs"
	handlers_v0 "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	routes_v0 "github.com/threeport/threeport/pkg/api-server/v0/routes"
	versions_v0 "github.com/threeport/threeport/pkg/api-server/v0/versions"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	log "github.com/threeport/threeport/pkg/log/v0"
	zap "go.uber.org/zap"
	"net/http"
	"os"
	"path/filepath"
)

// @title Threeport RESTful API
// @version v0.6.0
// @description Core API server for the Threeport application orchestration control plane.
// @contact.url https://threerport.io
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

	// set up echo
	e := echo.New()
	e.HideBanner = true

	var validate *validator.Validate
	validate = validator.New()
	validate.RegisterValidation("optional", apiserver_lib.IsOptional)
	validate.RegisterValidation("association", apiserver_lib.IsAssociation)
	validate.RegisterValidation("ISO8601date", apiserver_lib.IsISO8601Date)
	e.Validator = &apiserver_lib.CustomValidator{Validator: validate}

	// middleware
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &apiserver_lib.CustomContext{Context: c}
			return next(cc)
		}
	})
	logger, err := log.NewLogger(verbose)
	if err != nil {
		panic(err)
	}
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogError:    true,
		LogHost:     true,
		LogLatency:  true,
		LogMethod:   true,
		LogRemoteIP: true,
		LogStatus:   true,
		LogURI:      true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.Info(
				"request",
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

	// add extension router middleware
	if err := api_v0.InitExtensionRouter(db, e); err != nil {
		e.Logger.Fatalf("failed to initialize extension proxy router: %v", err)
	}

	// nats connection
	natsConn := fmt.Sprintf(
		"nats://%s:%s@%s:%s",
		os.Getenv("NATS_USER"),
		os.Getenv("NATS_PASSWORD"),
		os.Getenv("NATS_HOST"),
		os.Getenv("NATS_PORT"),
	)
	nc, err := natsgo.Connect(natsConn)
	if err != nil {
		e.Logger.Fatalf("failed to establish nats connection: %v", err)
	}
	defer nc.Close()

	// jetstream context
	js, err := util.InitJetStream(nc)
	if err != nil {
		e.Logger.Fatalf("failed to initialize nats jet stream: %v", err)
	}

	// handlers
	// v0
	h_v0 := handlers_v0.New(db, nc, *js)

	// routes
	routes_v0.SwaggerRoutes(e)
	util.VersionRoute(e)

	// v0
	routes_v0.AddRoutes(e, &h_v0)
	routes_v0.AddCustomRoutes(e, &h_v0)

	// add version info for queries to /<object>/versions
	apiserver_lib.Versions[0] = "v0"
	versions_v0.AddVersions()

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
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    caCertPool,
				RootCAs:      caCertPool,
			},
		}

		fmt.Printf("\nThreeport REST API: %s\n", version.GetVersion())
		configureHealthCheckEndpoint()
		if server.ListenAndServeTLS("", "") != http.ErrServerClosed {
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
		if server.ListenAndServe() != http.ErrServerClosed {
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
