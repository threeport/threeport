package restapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenRestApiMain generates source code for the REST API main package.
func GenRestApiMain(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// get project version
	projectVersionBytes, err := os.ReadFile("internal/version/version.txt")
	if err != nil {
		return fmt.Errorf("failed to read version from 'internal/version/version.txt': %w", err)
	}
	projectVersion := string(projectVersionBytes)

	// set startup message output
	var startupMessage string
	if gen.Extension {
		startupMessage = fmt.Sprintf(
			"\nThreeport extension REST API for API namespace %s: %%s\n",
			sdkConfig.ApiNamespace,
		)
	} else {
		startupMessage = "\nThreeport REST API: %s\n"
	}

	// build handler registration source code
	handlerRegistration := &Statement{}
	handlerRegistration.Comment("handlers")
	for _, versionConf := range gen.GlobalVersionConfig.Versions {
		handlerRegistration.Line().Comment(versionConf.VersionName)
		handlerRegistration.Line()
		handlerRegistration.Id(fmt.Sprintf("h_%s", versionConf.VersionName)).Op(":=").Qual(
			fmt.Sprintf("%s/pkg/api-server/%s/handlers", gen.ModulePath, versionConf.VersionName),
			"New",
		).Call(List(Id("db"), Id("nc"), Op("*").Id("js")))
	}
	handlerRegistration.Line()

	// build routes registration source code
	routeRegistration := &Statement{}
	routeRegistration.Comment("routes")
	routeRegistration.Line()
	routeRegistration.Qual(
		"github.com/threeport/threeport/pkg/api-server/v0/routes",
		"SwaggerRoutes",
	).Call(Id("e"))
	routeRegistration.Line()
	routeRegistration.Qual(
		fmt.Sprintf("%s/cmd/rest-api/util", gen.ModulePath),
		"VersionRoute",
	).Call(Id("e"))
	routeRegistration.Line()
	for _, versionConf := range gen.GlobalVersionConfig.Versions {
		routeRegistration.Line().Comment(versionConf.VersionName)
		routeRegistration.Line()
		routeRegistration.Qual(
			fmt.Sprintf("%s/pkg/api-server/%s/routes", gen.ModulePath, versionConf.VersionName),
			"AddRoutes",
		).Call(List(Id("e"), Op("&").Id(fmt.Sprintf("h_%s", versionConf.VersionName))))
		routeRegistration.Line()
		routeRegistration.Qual(
			fmt.Sprintf("%s/pkg/api-server/%s/routes", gen.ModulePath, versionConf.VersionName),
			"AddCustomRoutes",
		).Call(List(Id("e"), Op("&").Id(fmt.Sprintf("h_%s", versionConf.VersionName))))
		routeRegistration.Line()
	}
	routeRegistration.Line()

	// build version registration source code
	versionRegistration := &Statement{}
	versionRegistration.Comment("add version info for queries to /<object>/versions")
	versionRegistration.Line()
	for i, versionConf := range gen.GlobalVersionConfig.Versions {
		versionRegistration.Qual(
			"github.com/threeport/threeport/pkg/api-server/lib/v0",
			"Versions",
		).Index(Lit(i)).Op("=").Lit(versionConf.VersionName)
		versionRegistration.Line()
	}
	for _, versionConf := range gen.GlobalVersionConfig.Versions {
		versionRegistration.Qual(
			fmt.Sprintf("%s/pkg/api-server/%s/versions", gen.ModulePath, versionConf.VersionName),
			"AddVersions",
		).Call()
		versionRegistration.Line()
	}
	versionRegistration.Line()

	f := NewFile("main")
	f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")
	for _, versionConf := range gen.GlobalVersionConfig.Versions {
		f.ImportAlias(
			fmt.Sprintf("%s/pkg/api-server/%s/routes", gen.ModulePath, versionConf.VersionName),
			fmt.Sprintf("routes_%s", versionConf.VersionName),
		)
		f.ImportAlias(
			fmt.Sprintf("%s/pkg/api-server/%s/versions", gen.ModulePath, versionConf.VersionName),
			fmt.Sprintf("versions_%s", versionConf.VersionName),
		)
		f.ImportAlias(
			fmt.Sprintf("%s/pkg/api-server/%s/handlers", gen.ModulePath, versionConf.VersionName),
			fmt.Sprintf("handlers_%s", versionConf.VersionName),
		)

	}
	f.ImportAlias("github.com/labstack/echo/v4", "echo")
	f.ImportAlias("github.com/go-playground/validator/v10", "validator")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/lib/v0", "apiserver_lib")
	f.ImportAlias(util.SetImportAlias(
		"github.com/threeport/threeport/pkg/log/v0",
		"log",
		"tp_log",
		gen.Extension,
	))
	f.ImportAlias(util.SetImportAlias(
		"github.com/threeport/threeport/pkg/api-server/v0/routes",
		"routes_v0",
		"tp_routes",
		gen.Extension,
	))
	f.ImportAlias(util.SetImportAlias(
		"github.com/threeport/threeport/pkg/api-server/v0",
		"apiserver",
		"tp_apiserver",
		gen.Extension,
	))
	f.Anon("github.com/threeport/threeport/pkg/api-server/v0/docs")

	// swagger docs
	if sdkConfig.ApiDocs.Title != "" {
		f.Comment(fmt.Sprintf("@title %s", sdkConfig.ApiDocs.Title))
	} else {
		f.Comment("@title Threeport Extension API")
	}
	f.Comment(fmt.Sprintf("@version %s", strings.TrimSuffix(projectVersion, "\n")))
	if sdkConfig.ApiDocs.Description != "" {
		f.Comment(fmt.Sprintf("@description %s", sdkConfig.ApiDocs.Description))
	}
	if sdkConfig.ApiDocs.TosLink != "" {
		f.Comment(fmt.Sprintf("@termsOfService %s", sdkConfig.ApiDocs.TosLink))
	}
	if sdkConfig.ApiDocs.ContactName != "" {
		f.Comment(fmt.Sprintf("@contact.name %s", sdkConfig.ApiDocs.ContactName))
	}
	if sdkConfig.ApiDocs.ContactUrl != "" {
		f.Comment(fmt.Sprintf("@contact.url %s", sdkConfig.ApiDocs.ContactUrl))
	}
	if sdkConfig.ApiDocs.ContactEmail != "" {
		f.Comment(fmt.Sprintf("@contact.email %s", sdkConfig.ApiDocs.ContactEmail))
	}
	f.Comment(fmt.Sprintf("@BasePath /%s", sdkConfig.ApiNamespace))

	f.Func().Id("main").Params().Block(
		Comment("flags"),
		Var().Id("envFile").String(),
		Var().Id("autoMigrate").Bool(),
		Var().Id("verbose").Bool(),
		Var().Id("authEnabled").Bool(),
		Qual("flag", "StringVar").Call(
			Id("&envFile"),
			Lit("env-file"),
			Lit("/etc/threeport/env"),
			Lit("File from which to load environment"),
		),
		Qual("flag", "BoolVar").Call(
			Id("&autoMigrate"),
			Lit("auto-migrate"),
			False(),
			Lit("If true API server will auto migrate DB schema"),
		),
		Qual("flag", "BoolVar").Call(
			Id("&verbose"),
			Lit("verbose"),
			False(),
			Lit("Write logs with v(1).InfoLevel and above"),
		),
		Qual("flag", "BoolVar").Call(
			Id("&authEnabled"),
			Lit("auth-enabled"),
			True(),
			Lit("Enable client certificate authentication"),
		),
		Qual("flag", "Parse").Call(),
		Line(),

		Comment("set up echo"),
		Id("e").Op(":=").Qual("github.com/labstack/echo/v4", "New").Call(),
		Id("e").Dot("HideBanner").Op("=").True(),
		Line(),

		Var().Id("validate").Op("*").Qual("github.com/go-playground/validator/v10", "Validate"),
		Id("validate").Op("=").Qual("github.com/go-playground/validator/v10", "New").Call(),
		Id("validate").Dot("RegisterValidation").Call(
			Lit("optional"), Qual(
				"github.com/threeport/threeport/pkg/api-server/lib/v0",
				"IsOptional",
			),
		),
		Id("validate").Dot("RegisterValidation").Call(
			Lit("association"), Qual(
				"github.com/threeport/threeport/pkg/api-server/lib/v0",
				"IsAssociation",
			),
		),
		Id("validate").Dot("RegisterValidation").Call(
			Lit("ISO8601date"), Qual(
				"github.com/threeport/threeport/pkg/api-server/lib/v0",
				"IsISO8601Date",
			),
		),
		Id("e").Dot("Validator").Op("=").Op("&").Qual(
			"github.com/threeport/threeport/pkg/api-server/lib/v0",
			"CustomValidator",
		).Values(Dict{
			Id("Validator"): Id("validate"),
		}),
		Line(),

		Comment("middleware"),
		Id("e").Dot("Use").Call(
			Func().Params(Id("next").Qual(
				"github.com/labstack/echo/v4",
				"HandlerFunc",
			)).Qual(
				"github.com/labstack/echo/v4",
				"HandlerFunc",
			).Block(
				Return(Func().Params(Id("c").Qual(
					"github.com/labstack/echo/v4",
					"Context",
				)).Error().Block(
					Id("cc").Op(":=").Op("&").Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"CustomContext",
					).Values(Dict{
						Id("Context"): Id("c"),
					}),
					Return(Id("next").Call(Id("cc"))),
				)),
			)),
		Id("logger").Op(",").Id("err").Op(":=").Qual(
			"github.com/threeport/threeport/pkg/log/v0",
			"NewLogger",
		).Call(Id("verbose")),
		If(Id("err").Op("!=").Nil()).Block(
			Panic(Id("err")),
		),
		Id("e").Dot("Use").Call(
			Qual(
				"github.com/labstack/echo/v4/middleware",
				"RequestLoggerWithConfig",
			).Call(Qual(
				"github.com/labstack/echo/v4/middleware",
				"RequestLoggerConfig",
			).Values(Dict{
				Id("LogMethod"):   True(),
				Id("LogURI"):      True(),
				Id("LogStatus"):   True(),
				Id("LogRemoteIP"): True(),
				Id("LogHost"):     True(),
				Id("LogLatency"):  True(),
				Id("LogError"):    True(),
				Id("LogValuesFunc"): Func().Params(Id("c").Qual(
					"github.com/labstack/echo/v4",
					"Context",
				), Id("v").Qual(
					"github.com/labstack/echo/v4/middleware",
					"RequestLoggerValues",
				)).Error().Block(
					Id("logger").Dot("Info").Call(
						Line().Lit("request"),
						Line().Qual("go.uber.org/zap", "String").Call(Lit("method"), Id("v").Dot("Method")),
						Line().Qual("go.uber.org/zap", "String").Call(Lit("uri"), Id("v").Dot("URI")),
						Line().Qual("go.uber.org/zap", "Int").Call(Lit("status"), Id("v").Dot("Status")),
						Line().Qual("go.uber.org/zap", "String").Call(Lit("remoteIP"), Id("v").Dot("RemoteIP")),
						Line().Qual("go.uber.org/zap", "String").Call(Lit("host"), Id("v").Dot("Host")),
						Line().Qual("go.uber.org/zap", "Duration").Call(Lit("latency"), Id("v").Dot("Latency")),
						Line().Qual("go.uber.org/zap", "Error").Call(Id("v").Dot("Error")),
						Line(),
					),
					Return(Nil()),
				),
			}))),

		Id("e").Dot("Use").Call(Qual("github.com/labstack/echo/v4/middleware", "Recover").Call()),
		Line(),

		Id("e").Dot("HTTPErrorHandler").Op("=").Func().Params(
			Id("err").Error(), Id("c").Qual("github.com/labstack/echo/v4", "Context"),
		).Block(
			Comment("call the default handler to return the HTTP response"),
			Id("e").Dot("DefaultHTTPErrorHandler").Call(Id("err"), Id("c")),
		),
		Line(),

		Comment("env vars for database and nats connection"),
		If(Err().Op(":=").Qual(
			"github.com/joho/godotenv",
			"Load",
		).Call(Id("envFile")), Err().Op("!=").Nil()).Block(
			Id("e").Dot("Logger").Dot("Fatalf").Call(
				Lit("failed to load environment variables: %v"), Err(),
			),
		),
		Line(),

		Comment("database connection"),

		// Declare dbAttemptsMax, dbWaitDurationSeconds, dbAttempts, gormDb, dbConnectErr
		Id("dbAttemptsMax").Op(":=").Lit(25),
		Id("dbWaitDurationSeconds").Op(":=").Lit(20),
		Id("dbAttempts").Op(":=").Lit(0),
		Id("gormDb").Op(":=").Op("&").Qual("gorm.io/gorm", "DB").Values(),
		Var().Id("dbConnectErr").Error(),

		// For loop for DB connection attempts
		For(Id("dbAttempts").Op("<").Id("dbAttemptsMax")).Block(
			List(Id("db"), Id("err")).Op(":=").Qual("database", "Init").Call(Id("autoMigrate"), Op("&").Id("logger")),
			If(Id("err").Op("==").Nil()).Block(
				Id("gormDb").Op("=").Id("db"),
				Break(),
			).Else().Block(
				Id("dbConnectErr").Op("=").Id("err"),
			),
			Id("dbAttempts").Op("++"),
			Qual("time", "Sleep").Call(Qual("time", "Second").Op("*").Qual("time", "Duration").Call(Id("dbWaitDurationSeconds"))),
		),

		// Check if gormDb is nil
		If(Id("gormDb").Op("==").Nil()).Block(
			Id("e").Dot("Logger").Dot("Fatalf").Call(Lit("timed out trying to connect to database %v"), Id("dbConnectErr")),
		),

		Line(), // Newline for separation

		// NATS connection
		Comment("nats connection"),
		Id("natsConnString").Op(":=").Qual("fmt", "Sprintf").Call(
			Lit("nats://%s:%s@%s:%s"),
			Qual("os", "Getenv").Call(Lit("NATS_USER")),
			Qual("os", "Getenv").Call(Lit("NATS_PASSWORD")),
			Qual("os", "Getenv").Call(Lit("NATS_HOST")),
			Qual("os", "Getenv").Call(Lit("NATS_PORT")),
		),

		// Declare natsAttemptsMax, natsWaitDurationSeconds, natsAttempts, natsConn, natsConnErr
		Id("natsAttemptsMax").Op(":=").Lit(5),
		Id("natsWaitDurationSeconds").Op(":=").Lit(20),
		Id("natsAttempts").Op(":=").Lit(0),
		Id("natsConn").Op(":=").Op("&").Qual("github.com/nats-io/nats.go", "Conn").Values(),
		Var().Id("natsConnErr").Error(),

		// For loop for NATS connection attempts
		For(Id("natsAttempts").Op("<").Id("natsAttemptsMax")).Block(
			List(Id("nc"), Id("err")).Op(":=").Qual("github.com/nats-io/nats.go", "Connect").Call(Id("natsConnString")),
			If(Id("err").Op("==").Nil()).Block(
				Id("natsConn").Op("=").Id("nc"),
				Break(),
			).Else().Block(
				Id("natsConnErr").Op("=").Id("err"),
			),
			Id("natsAttempts").Op("++"),
			Qual("time", "Sleep").Call(Qual("time", "Second").Op("*").Qual("time", "Duration").Call(Id("natsWaitDurationSeconds"))),
		),

		// Check if natsConn is nil
		If(Id("natsConn").Op("==").Nil()).Block(
			Id("e").Dot("Logger").Dot("Fatalf").Call(Lit("timed out trying to establish nats connection: %v"), Id("natsConnErr")),
		),

		// Defer close
		Defer().Id("natsConn").Dot("Close").Call(),
		Line(),

		Comment("jetstream context"),
		List(Id("js"), Err()).Op(":=").Qual(
			fmt.Sprintf("%s/cmd/rest-api/util", gen.ModulePath),
			"InitJetStream",
		).Call(Id("nc")),
		If(Err().Op("!=").Nil()).Block(
			Id("e").Dot("Logger").Dot("Fatalf").Call(
				Lit("failed to initialize nats jet stream: %v"), Err(),
			),
		),
		Line(),

		handlerRegistration,

		routeRegistration,

		versionRegistration,

		If(Id("authEnabled")).Block(
			Id("configDir").Op(":=").Lit("/etc/threeport"),
			Line(),

			Comment("load server certificate and private key"),
			List(Id("cert"), Id("err")).Op(":=").Qual("crypto/tls", "LoadX509KeyPair").Call(
				Qual("path/filepath", "Join").Call(Id("configDir"), Lit("cert/tls.crt")),
				Qual("path/filepath", "Join").Call(Id("configDir"), Lit("cert/tls.key")),
			),
			If(Id("err").Op("!=").Nil()).Block(
				Id("e.Logger.Fatal").Call(Id("err")),
			),
			Line(),

			Comment("load server root certificate authority"),
			List(Id("caCert"), Id("err")).Op(":=").Qual("os", "ReadFile").Call(
				Qual("path/filepath", "Join").Call(Id("configDir"), Lit("ca/tls.crt")),
			),
			If(Id("err").Op("!=").Nil()).Block(
				Id("e.Logger.Fatal").Call(Id("err")),
			),
			Line(),

			Comment("create certificate pool and add server root certificate authority"),
			Id("caCertPool").Op(":=").Qual("crypto/x509", "NewCertPool").Call(),
			Id("caCertPool.AppendCertsFromPEM").Call(Id("caCert")),
			Line(),

			Comment("configure https server"),
			Id("server").Op(":=").Qual("net/http", "Server").Values(Dict{
				Id("Addr"):    Lit(":1323"),
				Id("Handler"): Id("e"),
				Id("TLSConfig"): Op("&").Qual("crypto/tls", "Config").Values(Dict{
					Id("Certificates"): Index().Qual("crypto/tls", "Certificate").Values(Id("cert")),
					Id("RootCAs"):      Id("caCertPool"),
					Id("ClientCAs"):    Id("caCertPool"),
					Id("ClientAuth"):   Qual("crypto/tls", "RequireAndVerifyClientCert"),
				}),
			}),
			Line(),

			Qual("fmt", "Printf").Call(Lit(startupMessage), Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call()),
			Id("configureHealthCheckEndpoint").Call(),
			If(Id("server.ListenAndServeTLS").Call(Lit(""), Lit("")).Op("!=").Qual(
				"net/http", "ErrServerClosed",
			)).Block(
				Id("e.Logger.Fatal").Call(Id("err")),
			),
		).Else().Block(
			Comment("configure http server"),
			Id("server").Op(":=").Qual("net/http", "Server").Values(Dict{
				Id("Addr"):    Lit(":1323"),
				Id("Handler"): Id("e"),
			}),
			Line(),

			Qual("fmt", "Printf").Call(Lit(startupMessage), Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call()),
			Id("configureHealthCheckEndpoint").Call(),
			If(Id("server.ListenAndServe").Call().Op("!=").Qual("net/http", "ErrServerClosed")).Block(
				Id("e.Logger.Fatal").Call(Id("err")),
			),
		),
	)

	f.Comment("configureHealthCheckEndpoint sets up a health check endpoint for the API server")
	f.Func().Id("configureHealthCheckEndpoint").Params().Block(
		Comment("set up health check endpoint"),
		Qual("net/http", "HandleFunc").Call(
			Lit("/readyz"),
			Func().Params(Id("w").Qual("net/http", "ResponseWriter"), Id("r").Op("*").Qual("net/http", "Request")).Block(
				Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
				Id("w").Dot("Write").Call(Index().Byte().Call(Lit("OK"))),
			),
		),
		Line(),
		Go().Qual("net/http", "ListenAndServe").Call(Lit(":8081"), Nil()),
	)

	// write code to file
	genFilepath := filepath.Join("cmd", "rest-api", "main_gen.go")
	_, err = util.WriteCodeToFile(f, genFilepath, true)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	cli.Info(fmt.Sprintf("source code for API main package written to %s", genFilepath))

	return nil
}
