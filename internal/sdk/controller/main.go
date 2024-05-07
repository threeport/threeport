package controller

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
	"github.com/threeport/threeport/internal/sdk"
)

// controllerMainPackagePath returns the path from the models to the
// controller's main package.
func controllerMainPackagePath(controllerName string) string {
	return filepath.Join("cmd", controllerName)
}

// MainPackage generates the source code for a controller's main package.
func (cc *ControllerConfig) MainPackage() error {
	pluralize := pluralize.NewClient()
	f := NewFile("main")
	f.HeaderComment("generated by 'threeport-sdk gen' for controller scaffolding - do not edit")

	f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "client")
	f.ImportAlias("github.com/threeport/threeport/pkg/client/v1", "client_v1")
	f.ImportAlias("github.com/threeport/threeport/pkg/event/v0", "event_v0")
	f.ImportAlias("github.com/threeport/threeport/pkg/controller/v0", "controller")

	//controllerShortName := strings.TrimSuffix(cc.Name, "-controller")
	//controllerStreamName := fmt.Sprintf("%sStreamName", strcase.ToCamel(cc.ShortName))

	concurrencyFlags := &Statement{}
	for _, obj := range cc.ReconciledObjects {
		concurrencyFlags.Var().Id(fmt.Sprintf(
			"%sConcurrentReconciles",
			fmt.Sprintf("%s_%s", obj.Version, strcase.ToLowerCamel(obj.Name)),
		)).Op("=").Qual(
			"github.com/namsral/flag",
			"Int",
		).Call(
			Line().Lit(fmt.Sprintf(
				"%s-concurrent-reconciles",
				fmt.Sprintf("%s-%s", obj.Version, strcase.ToKebab(obj.Name)),
			)),
			Line().Lit(1),
			Line().Lit(fmt.Sprintf(
				"Number of concurrent reconcilers to run for %s",
				pluralize.Pluralize(strcase.ToDelimited(obj.Name, ' '), 2, false),
			)),
			Line(),
		)
		concurrencyFlags.Line()
	}

	reconcilerConfigs := &Statement{}
	durable := true
	for _, obj := range cc.ReconciledObjects {
		// If any of the reconciled objects has a struct tag with "persist" set to
		// "false", then set the controller's consumer to be ephemeral. This will
		// prevent all of the nats streams associated with this controller from
		// being persisted to disk, and will result in nats messages being lost in the
		// event of a nats server failure or restart.
		if obj.DisableNotificationPersistence {
			durable = false
		}
		reconcilerConfigs.Id("reconcilerConfigs").Op("=").Append(Id("reconcilerConfigs").Op(",").Qual(
			"github.com/threeport/threeport/pkg/controller/v0",
			"ReconcilerConfig",
		).Values(Dict{
			Id("Name"): Lit(fmt.Sprintf("%sReconciler", obj.Name)),
			Id("ObjectType"): Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", obj.Version),
				fmt.Sprintf("ObjectType%s", obj.Name),
			),
			Id("ObjectVersion"): Lit(sdk.GetLatestObjectVersion(obj.Name)),
			Id("ReconcileFunc"): Qual(
				fmt.Sprintf("github.com/threeport/threeport/internal/%s", cc.ShortName),
				fmt.Sprintf("%sReconciler", obj.Name),
			),
			Id("ConcurrentReconciles"): Op("*").Id(fmt.Sprintf(
				"%sConcurrentReconciles",
				fmt.Sprintf("%s_%s", obj.Version, strcase.ToLowerCamel(obj.Name)),
			)),
			Id("NotifSubject"): Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", obj.Version),
				fmt.Sprintf("%sSubject", obj.Name),
			),
		}))
		reconcilerConfigs.Line()
	}

	f.Func().Id("main").Params().Block(
		Comment("flags"),
		concurrencyFlags,
		Var().Id("apiServer").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("api-server").Op(",").Lit("threeport-api-server").Op(",").Lit("Threepoort REST API server endpoint"),
		),
		Var().Id("msgBrokerHost").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-host").Op(",").Lit("").Op(",").Lit("Threeport message broker hostname"),
		),
		Var().Id("msgBrokerPort").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-port").Op(",").Lit("").Op(",").Lit("Threeport message broker port"),
		),
		Var().Id("msgBrokerUser").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-user").Op(",").Lit("").Op(",").Lit("Threeport message broker user"),
		),
		Var().Id("msgBrokerPassword").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-password").Op(",").Lit("").Op(",").Lit("Threeport message broker user password"),
		),
		Var().Id("shutdownPort").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("shutdown-port").Op(",").Lit("8181").Op(",").Lit("Port to listen for shutdown calls"),
		),
		Var().Id("verbose").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("verbose").Op(",").Lit(false).Op(",").Lit("Write logs with v(1).InfoLevel and above"),
		),
		Var().Id("help").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("help").Op(",").Lit(false).Op(",").Lit("Show help info"),
		),
		Var().Id("authEnabled").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("auth-enabled").Op(",").Lit(true).Op(",").Lit("Enable client certificate authentication (default is true)"),
		),
		Qual(
			"github.com/namsral/flag",
			"Parse",
		).Call(),

		Line(),
		Var().Id("log").Qual("github.com/go-logr/logr", "Logger"),
		Var().Id("encryptionKey").Op("=").Qual("os", "Getenv").Call(Lit("ENCRYPTION_KEY")),
		If(Id("encryptionKey").Op("==").Lit("")).Block(
			Id("log").Dot("Error").Call(
				Qual("errors", "New").Call(Lit("environment variable ENCRYPTION_KEY is not set")), Lit("encryption key not found"),
			),
		),

		Line(),
		If(Op("*").Id("help").Block(
			Id("showHelpAndExit").Call(Lit(0)),
		)),

		Line(),
		Comment("controller instance ID"),
		Id("controllerID").Op(":=").Qual(
			"github.com/google/uuid",
			"New",
		).Call(),

		Line().Comment("logging setup"),
		Switch(Op("*").Id("verbose")).Block(
			Case(Lit(true)).Block(
				List(Id("zapLog"), Id("err")).Op(":=").Qual("go.uber.org/zap", "NewDevelopment").Call(),
				If(Err().Op("!=").Nil()).Block(
					Panic(Qual("fmt", "Sprintf").Call(Lit("failed to set up development logging: %v"), Id("err"))),
				),
				Id("log").Op("=").Qual(
					"github.com/go-logr/zapr",
					"NewLogger",
				).Call(Id("zapLog")).Dot("WithValues").Call(Lit("controllerID"), Id("controllerID")),
			),
			Default().Block(
				List(Id("zapLog"), Id("err")).Op(":=").Qual("go.uber.org/zap", "NewProduction").Call(),
				If(Err().Op("!=").Nil()).Block(
					Panic(Qual("fmt", "Sprintf").Call(Lit("failed to set up production logging: %v"), Id("err"))),
				),
				Id("log").Op("=").Qual(
					"github.com/go-logr/zapr",
					"NewLogger",
				).Call(Id("zapLog")).Dot("WithValues").Call(Lit("controllerID"), Id("controllerID")),
			),
		),

		Line().Comment("connect to NATS server"),
		Id("natsConn").Op(":=").Qual("fmt", "Sprintf").Call(
			Line().Lit("nats://%s:%s@%s:%s"),
			Line().Op("*").Id("msgBrokerUser"),
			Line().Op("*").Id("msgBrokerPassword"),
			Line().Op("*").Id("msgBrokerHost"),
			Line().Op("*").Id("msgBrokerPort"),
			Line(),
		),
		List(Id("nc"), Id("err")).Op(":=").Qual(
			"github.com/nats-io/nats.go",
			"Connect",
		).Call(Id("natsConn")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to connect to NATS message broker"),
				Lit("NATSConnection"),
				Id("natsConn"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("create JetStream context"),
		List(Id("js"), Id("err")).Op(":=").Id("nc").Dot("JetStream").Call(Qual(
			"github.com/nats-io/nats.go",
			"PublishAsyncMaxPending",
		).Call(Lit(256))),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(Err(), Lit("failed to create JetStream context")),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("JetStream key-value store setup"),
		Id("kvConfig").Op(":=").Qual("github.com/nats-io/nats.go", "KeyValueConfig").Values(Dict{
			Id("Bucket"): Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/internal/%s",
					cc.ShortName,
				),
				"LockBucketName",
			),
			Id("Description"): Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/internal/%s",
					cc.ShortName,
				),
				"LockBucketDescr",
			),
			Id("TTL"): Qual("time", "Minute").Op("*").Lit(20),
		}),
		List(
			Id("kv"), Id("err"),
		).Op(":=").Id("controller").Dot("CreateLockBucketIfNotExists").Call(Id("js"), Op("&").Id("kvConfig")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to bind to JetStream key-value locking bucket"),
				Lit("lockBucketName"),
				Qual(
					fmt.Sprintf(
						"github.com/threeport/threeport/internal/%s",
						cc.ShortName,
					),
					"LockBucketName",
				),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment(fmt.Sprintf(
			"check to ensure %s stream has been created by API",
			cc.ShortName,
		)),
		Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(cc.StreamName),
		)).Op(":=").Lit(false),
		For(Id("stream").Op(":=").Range().Id("js").Dot("StreamNames").Call()).Block(
			If(Id("stream").Op("==").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				cc.StreamName,
			)).Block(
				Id(fmt.Sprintf(
					"%sFound",
					strcase.ToLowerCamel(cc.StreamName),
				)).Op("=").Lit(true),
			),
		),
		If(Op("!").Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(cc.StreamName),
		))).Block(
			Id("log").Dot("Error").Call(
				Qual("errors", "New").Call(Lit("JetStream stream not found")),
				Lit(fmt.Sprintf(
					"failed to find stream with %s stream name",
					cc.ShortName,
				)),
				Lit(strcase.ToLowerCamel(cc.StreamName)),
				Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					cc.StreamName,
				),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("create a channel and wait group used for graceful shut downs"),
		Var().Id("shutdownChans").Index().Chan().Bool(),
		Var().Id("shutdownWait").Qual("sync", "WaitGroup"),
		Line(),
		Comment("configure http client for calls to threeport API"),
		List(
			Id("apiClient"), Id("err"),
		).Op(":=").Qual("github.com/threeport/threeport/pkg/client/v0", "GetHTTPClient").Call(
			Op("*").Id("authEnabled").Op(",").Lit("").Op(",").Lit("").Op(",").Lit("").Op(",").Lit(""),
		),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to create http client"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("configure and start reconcilers"),
		Var().Id("reconcilerConfigs").Index().Qual(
			"github.com/threeport/threeport/pkg/controller/v0",
			"ReconcilerConfig",
		),
		reconcilerConfigs,

		For(
			Id("_").Op(",").Id("r").Op(":=").Range().Id("reconcilerConfigs"),
		).BlockFunc(func(g *Group) {
			cc.ConfigurePullSubscription(g, durable, nil)
			g.Line().Comment("create exit channel")
			g.Id("shutdownChan").Op(":=").Make(Chan().Bool(), Lit(1))
			g.Id("shutdownChans").Op("=").Append(Id("shutdownChans"), Id("shutdownChan"))

			g.Line().Comment("create reconciler")
			g.Id("reconciler").Op(":=").Id("controller").Dot("Reconciler").Values(Dict{
				Id("Name"):             Id("r").Dot("Name"),
				Id("ObjectType"):       Id("r").Dot("ObjectType"),
				Id("APIServer"):        Op("*").Id("apiServer"),
				Id("APIClient"):        Id("apiClient"),
				Id("JetStreamContext"): Id("js"),
				Id("Sub"):              Id("sub"),
				Id("KeyValue"):         Id("kv"),
				Id("ControllerID"):     Id("controllerID"),
				Id("Log"):              Op("&").Id("log"),
				Id("Shutdown"):         Id("shutdownChan"),
				Id("ShutdownWait"):     Op("&").Id("shutdownWait"),
				Id("EncryptionKey"):    Id("encryptionKey"),
				Id("EventsRecorder"): Op("&").Qual(
					"github.com/threeport/threeport/pkg/event/v0",
					"EventRecorder",
				).Values(Dict{
					Id("APIClient"): Id("apiClient"),
					Id("APIServer"): Op("*").Id("apiServer"),
					Id("ObjectType"): Qual("fmt", "Sprintf").Call(
						Line().Lit("%s.%s"),
						Line().Id("r").Dot("ObjectVersion"),
						Line().Id("r").Dot("ObjectType"),
						Line(),
					),
					Id("ReportingController"): Lit(
						fmt.Sprintf("%sController", strcase.ToCamel(cc.ShortName)),
					),
					Id("ReportingInstance"): Qual("os", "Getenv").Call(Lit("HOSTNAME")),
					Id("ControllerID"):      Id("controllerID").Dot("String").Call(),
				}),
			})

			g.Line().Comment("start reconciler")
			g.Id("go").Id("r").Dot("ReconcileFunc").Call(Op("&").Id("reconciler"))
		}),
		Line(),

		Id("log").Dot("Info").Call(
			Line().Lit(fmt.Sprintf("%s controller started", cc.ShortName)),
			Line().Lit("version"), Qual(
				"github.com/threeport/threeport/internal/version",
				"GetVersion",
			).Call(),
			Line().Lit("controllerID"), Id("controllerID").Dot("String").Call(),
			Line().Lit("NATSConnection"), Id("natsConn"),
			Line().Lit("lockBucketName"), Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/internal/%s",
					cc.ShortName,
				),
				"LockBucketName",
			),
			Line(),
		),
		Line(),

		Comment("add a shutdown endpoint for graceful shutdowns"),
		Id("mux").Op(":=").Qual("net/http", "NewServeMux").Call(),
		Id("server").Op(":=").Qual("net/http", "Server").Values(Dict{
			Id("Addr"):    Lit(":").Op("+").Op("*").Id("shutdownPort"),
			Id("Handler"): Id("mux"),
		}),
		Id("mux").Dot("HandleFunc").Call(
			Lit("/shutdown").Op(",").Func().Params(
				Id("w").Qual("net/http", "ResponseWriter").Op(",").Id("r").Op("*").Qual("net/http", "Request"),
			).Block(
				For(Id("_").Op(",").Id("c").Op(":=").Range().Id("shutdownChans")).Block(
					Id("c").Op("<-").Lit(true),
				),
				Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
				Qual("fmt", "Fprintf").Call(Id("w").Op(",").Lit("shutting down\n")),
				Id("shutdownWait").Dot("Add").Call(Lit(1)),
				Id("go").Func().Params().Block(
					Id("server").Dot("Shutdown").Call(Qual("context", "Background").Call()),
					Id("shutdownWait").Dot("Done").Call(),
				).Params(),
			),
		),

		Line(),
		Comment("set up health check endpoint"),
		Id("http.HandleFunc").Call(
			Lit("/readyz"),
			Func().Params(
				Id("w").Qual("net/http", "ResponseWriter"),
				Id("r").Op("*").Qual("net/http", "Request"),
			).Block(
				Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
				Id("w").Dot("Write").Call(Index().Byte().Call(Lit("OK"))),
			),
		),

		Go().Id("http.ListenAndServe").Call(Lit(":8081"), Nil()),
		Line(),

		Comment("run shutdown endpoint server"),
		If(
			Err().Op(":=").Id("server").Dot("ListenAndServe").Call(),
			Err().Op("!=").Nil().Op("&&").Err().Op("!=").Qual("net/http", "ErrServerClosed"),
		).Block(
			Id("log").Dot("Error").Call(Id("err"), Lit("failed to run server for shutdown endpoint")),
		),
		Line(),

		Comment("wait for reconcilers to finish"),
		Id("shutdownWait").Dot("Wait").Call(),
		Line(),

		Id("log").Dot("Info").Call(Lit(fmt.Sprintf("%s controller shutting down", cc.ShortName))),
		Qual("os", "Exit").Call(Lit(0)),
	)

	f.Func().Id("showHelpAndExit").Params(Id("exitCode").Int()).Block(
		Qual("fmt", "Printf").Call(Lit(fmt.Sprintf("Usage: threeport-%s-controller [options]\n", cc.ShortName))),
		Qual("fmt", "Println").Call(Lit("options:")),
		Id("flag").Dot("PrintDefaults").Call(),
		Qual("os", "Exit").Call(Id("exitCode")),
	)

	// write code to file
	genFilename := "main_gen.go"
	genFilepath := filepath.Join(controllerMainPackagePath(cc.Name), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for %s main package: %w", cc.Name, err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for %s main package: %w", cc.Name, err)
	}
	fmt.Printf("code generation complete for %s main package\n", cc.Name)

	return nil
}

// ExtensionMainPackage generates the source code for a controller's main package in an extension.
func (cc *ControllerConfig) ExtensionMainPackage(modulePath string) error {
	pluralize := pluralize.NewClient()
	f := NewFile("main")
	f.HeaderComment("generated by 'threeport-sdk gen' for controller scaffolding - do not edit")

	f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "client")
	f.ImportAlias("github.com/threeport/threeport/pkg/controller/v0", "controller")
	f.ImportAlias(fmt.Sprintf("%s/pkg/config/v0", modulePath), "config")

	//controllerShortName := strings.TrimSuffix(cc.Name, "-controller")
	//controllerStreamName := fmt.Sprintf("%sStreamName", strcase.ToCamel(cc.ShortName))

	concurrencyFlags := &Statement{}
	for _, obj := range cc.ReconciledObjects {
		concurrencyFlags.Var().Id(fmt.Sprintf(
			"%sConcurrentReconciles",
			fmt.Sprintf("%s_%s", obj.Version, strcase.ToLowerCamel(obj.Name)),
		)).Op("=").Qual(
			"github.com/namsral/flag",
			"Int",
		).Call(
			Line().Lit(fmt.Sprintf(
				"%s-concurrent-reconciles",
				fmt.Sprintf("%s-%s", obj.Version, strcase.ToKebab(obj.Name)),
			)),
			Line().Lit(1),
			Line().Lit(fmt.Sprintf(
				"Number of concurrent reconcilers to run for %s",
				pluralize.Pluralize(strcase.ToDelimited(obj.Name, ' '), 2, false),
			)),
			Line(),
		)
		concurrencyFlags.Line()
	}

	reconcilerConfigs := &Statement{}
	durable := true
	for _, obj := range cc.ReconciledObjects {
		// If any of the reconciled objects has a struct tag with "persist" set to
		// "false", then set the controller's consumer to be ephemeral. This will
		// prevent all of the nats streams associated with this controller from
		// being persisted to disk, and will result in nats messages being lost in the
		// event of a nats server failure or restart.
		if obj.DisableNotificationPersistence {
			durable = false
		}
		reconcilerConfigs.Id("reconcilerConfigs").Op("=").Append(Id("reconcilerConfigs").Op(",").Qual(
			"github.com/threeport/threeport/pkg/controller/v0",
			"ReconcilerConfig",
		).Values(Dict{
			Id("Name"): Lit(fmt.Sprintf("%sReconciler", obj.Name)),
			Id("ObjectType"): Qual(
				fmt.Sprintf("%s/pkg/api/v0", modulePath),
				fmt.Sprintf("ObjectType%s", obj.Name),
			),
			Id("ReconcileFunc"): Qual(
				fmt.Sprintf("%s/internal/%s", modulePath, cc.ShortName),
				fmt.Sprintf("%sReconciler", obj.Name),
			),
			Id("ConcurrentReconciles"): Op("*").Id(fmt.Sprintf(
				"%sConcurrentReconciles",
				fmt.Sprintf("%s_%s", obj.Version, strcase.ToLowerCamel(obj.Name)),
			)),
			Id("NotifSubject"): Qual(
				fmt.Sprintf("%s/pkg/api/v0", modulePath),
				fmt.Sprintf("%sSubject", obj.Name),
			),
		}))
		reconcilerConfigs.Line()
	}

	f.Func().Id("main").Params().Block(
		Comment("flags"),
		concurrencyFlags,
		Var().Id("apiServer").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("api-server").Op(",").Lit("threeport-api-server").Op(",").Lit("Threepoort REST API server endpoint"),
		),
		Var().Id("msgBrokerHost").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-host").Op(",").Lit("").Op(",").Lit("Threeport message broker hostname"),
		),
		Var().Id("msgBrokerPort").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-port").Op(",").Lit("").Op(",").Lit("Threeport message broker port"),
		),
		Var().Id("msgBrokerUser").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-user").Op(",").Lit("").Op(",").Lit("Threeport message broker user"),
		),
		Var().Id("msgBrokerPassword").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("msg-broker-password").Op(",").Lit("").Op(",").Lit("Threeport message broker user password"),
		),
		Var().Id("shutdownPort").Op("=").Qual(
			"github.com/namsral/flag",
			"String",
		).Call(
			Lit("shutdown-port").Op(",").Lit("8181").Op(",").Lit("Port to listen for shutdown calls"),
		),
		Var().Id("verbose").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("verbose").Op(",").Lit(false).Op(",").Lit("Write logs with v(1).InfoLevel and above"),
		),
		Var().Id("help").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("help").Op(",").Lit(false).Op(",").Lit("Show help info"),
		),
		Var().Id("authEnabled").Op("=").Qual(
			"github.com/namsral/flag",
			"Bool",
		).Call(
			Lit("auth-enabled").Op(",").Lit(true).Op(",").Lit("Enable client certificate authentication (default is true)"),
		),
		Qual(
			"github.com/namsral/flag",
			"Parse",
		).Call(),

		Line(),
		Var().Id("log").Qual("github.com/go-logr/logr", "Logger"),
		Var().Id("encryptionKey").Op("=").Qual("os", "Getenv").Call(Lit("ENCRYPTION_KEY")),
		If(Id("encryptionKey").Op("==").Lit("")).Block(
			Id("log").Dot("Error").Call(
				Qual("errors", "New").Call(Lit("environment variable ENCRYPTION_KEY is not set")), Lit("encryption key not found"),
			),
		),

		Line(),
		If(Op("*").Id("help").Block(
			Id("showHelpAndExit").Call(Lit(0)),
		)),

		Line(),
		Comment("controller instance ID"),
		Id("controllerID").Op(":=").Qual(
			"github.com/google/uuid",
			"New",
		).Call(),

		Line().Comment("logging setup"),
		Switch(Op("*").Id("verbose")).Block(
			Case(Lit(true)).Block(
				List(Id("zapLog"), Id("err")).Op(":=").Qual("go.uber.org/zap", "NewDevelopment").Call(),
				If(Err().Op("!=").Nil()).Block(
					Panic(Qual("fmt", "Sprintf").Call(Lit("failed to set up development logging: %v"), Id("err"))),
				),
				Id("log").Op("=").Qual(
					"github.com/go-logr/zapr",
					"NewLogger",
				).Call(Id("zapLog")).Dot("WithValues").Call(Lit("controllerID"), Id("controllerID")),
			),
			Default().Block(
				List(Id("zapLog"), Id("err")).Op(":=").Qual("go.uber.org/zap", "NewProduction").Call(),
				If(Err().Op("!=").Nil()).Block(
					Panic(Qual("fmt", "Sprintf").Call(Lit("failed to set up production logging: %v"), Id("err"))),
				),
				Id("log").Op("=").Qual(
					"github.com/go-logr/zapr",
					"NewLogger",
				).Call(Id("zapLog")).Dot("WithValues").Call(Lit("controllerID"), Id("controllerID")),
			),
		),

		Line().Comment("config setup"),
		Id("err").Op(":=").Qual(
			fmt.Sprintf("%s/pkg/config/v0", modulePath),
			"InitServerConfig",
		).Call(),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to initialize controller config"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("connect to NATS server"),
		Id("natsConn").Op(":=").Qual("fmt", "Sprintf").Call(
			Line().Lit("nats://%s:%s@%s:%s"),
			Line().Op("*").Id("msgBrokerUser"),
			Line().Op("*").Id("msgBrokerPassword"),
			Line().Op("*").Id("msgBrokerHost"),
			Line().Op("*").Id("msgBrokerPort"),
			Line(),
		),
		List(Id("nc"), Id("err")).Op(":=").Qual(
			"github.com/nats-io/nats.go",
			"Connect",
		).Call(Id("natsConn")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to connect to NATS message broker"),
				Lit("NATSConnection"),
				Id("natsConn"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("create JetStream context"),
		List(Id("js"), Id("err")).Op(":=").Id("nc").Dot("JetStream").Call(Qual(
			"github.com/nats-io/nats.go",
			"PublishAsyncMaxPending",
		).Call(Lit(256))),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(Err(), Lit("failed to create JetStream context")),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("JetStream key-value store setup"),
		Id("kvConfig").Op(":=").Qual("github.com/nats-io/nats.go", "KeyValueConfig").Values(Dict{
			Id("Bucket"): Qual(
				fmt.Sprintf(
					"%s/internal/%s",
					modulePath,
					cc.ShortName,
				),
				"LockBucketName",
			),
			Id("Description"): Qual(
				fmt.Sprintf(
					"%s/internal/%s",
					modulePath,
					cc.ShortName,
				),
				"LockBucketDescr",
			),
			Id("TTL"): Qual("time", "Minute").Op("*").Lit(20),
		}),
		List(
			Id("kv"), Id("err"),
		).Op(":=").Id("controller").Dot("CreateLockBucketIfNotExists").Call(Id("js"), Op("&").Id("kvConfig")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to bind to JetStream key-value locking bucket"),
				Lit("lockBucketName"),
				Qual(
					fmt.Sprintf(
						"%s/internal/%s",
						modulePath,
						cc.ShortName,
					),
					"LockBucketName",
				),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment(fmt.Sprintf(
			"check to ensure %s stream has been created by API",
			cc.ShortName,
		)),
		Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(cc.StreamName),
		)).Op(":=").Lit(false),
		For(Id("stream").Op(":=").Range().Id("js").Dot("StreamNames").Call()).Block(
			If(Id("stream").Op("==").Qual(
				fmt.Sprintf("%s/pkg/api/v0", modulePath),
				cc.StreamName,
			)).Block(
				Id(fmt.Sprintf(
					"%sFound",
					strcase.ToLowerCamel(cc.StreamName),
				)).Op("=").Lit(true),
			),
		),
		If(Op("!").Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(cc.StreamName),
		))).Block(
			Id("log").Dot("Error").Call(
				Qual("errors", "New").Call(Lit("JetStream stream not found")),
				Lit(fmt.Sprintf(
					"failed to find stream with %s stream name",
					cc.ShortName,
				)),
				Lit(strcase.ToLowerCamel(cc.StreamName)),
				Qual(
					fmt.Sprintf("%s/pkg/api/v0", modulePath),
					cc.StreamName,
				),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("create a channel and wait group used for graceful shut downs"),
		Var().Id("shutdownChans").Index().Chan().Bool(),
		Var().Id("shutdownWait").Qual("sync", "WaitGroup"),
		Line(),
		Comment("configure http client for calls to threeport API"),
		List(
			Id("apiClient"), Id("err"),
		).Op(":=").Qual("github.com/threeport/threeport/pkg/client/v0", "GetHTTPClient").Call(
			Op("*").Id("authEnabled").Op(",").Lit("").Op(",").Lit("").Op(",").Lit("").Op(",").Lit(""),
		),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to create http client"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("configure and start reconcilers"),
		Var().Id("reconcilerConfigs").Index().Qual(
			"github.com/threeport/threeport/pkg/controller/v0",
			"ReconcilerConfig",
		),
		reconcilerConfigs,

		For(
			Id("_").Op(",").Id("r").Op(":=").Range().Id("reconcilerConfigs"),
		).BlockFunc(func(g *Group) {
			cc.ConfigurePullSubscription(g, durable, &modulePath)
			g.Line().Comment("create exit channel")
			g.Id("shutdownChan").Op(":=").Make(Chan().Bool(), Lit(1))
			g.Id("shutdownChans").Op("=").Append(Id("shutdownChans"), Id("shutdownChan"))

			g.Line().Comment("create reconciler")
			g.Id("reconciler").Op(":=").Id("controller").Dot("Reconciler").Values(Dict{
				Id("Name"):             Id("r").Dot("Name"),
				Id("ObjectType"):       Id("r").Dot("ObjectType"),
				Id("APIServer"):        Op("*").Id("apiServer"),
				Id("APIClient"):        Id("apiClient"),
				Id("JetStreamContext"): Id("js"),
				Id("Sub"):              Id("sub"),
				Id("KeyValue"):         Id("kv"),
				Id("ControllerID"):     Id("controllerID"),
				Id("Log"):              Op("&").Id("log"),
				Id("Shutdown"):         Id("shutdownChan"),
				Id("ShutdownWait"):     Op("&").Id("shutdownWait"),
				Id("EncryptionKey"):    Id("encryptionKey"),
			})

			g.Line().Comment("start reconciler")
			g.Id("go").Id("r").Dot("ReconcileFunc").Call(Op("&").Id("reconciler"))
		}),
		Line(),

		Id("log").Dot("Info").Call(
			Line().Lit(fmt.Sprintf("%s controller started", cc.ShortName)),
			Line().Lit("version"), Qual(
				fmt.Sprintf("%s/internal/version", modulePath),
				"GetVersion",
			).Call(),
			Line().Lit("controllerID"), Id("controllerID").Dot("String").Call(),
			Line().Lit("NATSConnection"), Id("natsConn"),
			Line().Lit("lockBucketName"), Qual(
				fmt.Sprintf(
					"%s/internal/%s",
					modulePath,
					cc.ShortName,
				),
				"LockBucketName",
			),
			Line(),
		),
		Line(),

		Comment("add a shutdown endpoint for graceful shutdowns"),
		Id("mux").Op(":=").Qual("net/http", "NewServeMux").Call(),
		Id("server").Op(":=").Qual("net/http", "Server").Values(Dict{
			Id("Addr"):    Lit(":").Op("+").Op("*").Id("shutdownPort"),
			Id("Handler"): Id("mux"),
		}),
		Id("mux").Dot("HandleFunc").Call(
			Lit("/shutdown").Op(",").Func().Params(
				Id("w").Qual("net/http", "ResponseWriter").Op(",").Id("r").Op("*").Qual("net/http", "Request"),
			).Block(
				For(Id("_").Op(",").Id("c").Op(":=").Range().Id("shutdownChans")).Block(
					Id("c").Op("<-").Lit(true),
				),
				Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
				Qual("fmt", "Fprintf").Call(Id("w").Op(",").Lit("shutting down\n")),
				Id("shutdownWait").Dot("Add").Call(Lit(1)),
				Id("go").Func().Params().Block(
					Id("server").Dot("Shutdown").Call(Qual("context", "Background").Call()),
					Id("shutdownWait").Dot("Done").Call(),
				).Params(),
			),
		),

		Line(),
		Comment("set up health check endpoint"),
		Id("http.HandleFunc").Call(
			Lit("/readyz"),
			Func().Params(
				Id("w").Qual("net/http", "ResponseWriter"),
				Id("r").Op("*").Qual("net/http", "Request"),
			).Block(
				Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
				Id("w").Dot("Write").Call(Index().Byte().Call(Lit("OK"))),
			),
		),

		Go().Id("http.ListenAndServe").Call(Lit(":8081"), Nil()),
		Line(),

		Comment("run shutdown endpoint server"),
		If(
			Err().Op(":=").Id("server").Dot("ListenAndServe").Call(),
			Err().Op("!=").Nil().Op("&&").Err().Op("!=").Qual("net/http", "ErrServerClosed"),
		).Block(
			Id("log").Dot("Error").Call(Id("err"), Lit("failed to run server for shutdown endpoint")),
		),
		Line(),

		Comment("wait for reconcilers to finish"),
		Id("shutdownWait").Dot("Wait").Call(),
		Line(),

		Id("log").Dot("Info").Call(Lit(fmt.Sprintf("%s controller shutting down", cc.ShortName))),
		Qual("os", "Exit").Call(Lit(0)),
	)

	f.Func().Id("showHelpAndExit").Params(Id("exitCode").Int()).Block(
		Qual("fmt", "Printf").Call(Lit(fmt.Sprintf("Usage: threeport-%s-controller [options]\n", cc.ShortName))),
		Qual("fmt", "Println").Call(Lit("options:")),
		Id("flag").Dot("PrintDefaults").Call(),
		Qual("os", "Exit").Call(Id("exitCode")),
	)

	// write code to file
	genFilename := "main_gen.go"
	genFilepath := filepath.Join(controllerMainPackagePath(cc.Name), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for %s main package: %w", cc.Name, err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for %s main package: %w", cc.Name, err)
	}
	fmt.Printf("code generation complete for %s main package\n", cc.Name)

	return nil

}

// ConfigurePullSubscription adds a durable consumer to a controller's main package.
func (cc *ControllerConfig) ConfigurePullSubscription(g *Group, durable bool, moduleOverride *string) {
	modulePath := "github.com/threeport/threeport"
	if moduleOverride != nil {
		modulePath = *moduleOverride
	}
	consumer := Lit("")
	if durable {
		consumer = Id("consumer")
		g.Line().Comment("create JetStream consumer")
		g.Id("consumer").Op(":=").Id("r").Dot("Name").Op("+").Lit("Consumer")
		g.Id("js").Dot("AddConsumer").Call(Qual(
			fmt.Sprintf("%s/pkg/api/v0", modulePath),
			cc.StreamName,
		).Op(",").Op("&").Qual(
			"github.com/nats-io/nats.go",
			"ConsumerConfig",
		).Values(Dict{
			Id("Durable"): consumer,
			Id("AckPolicy"): Qual(
				"github.com/nats-io/nats.go",
				"AckExplicitPolicy",
			),
			Id("FilterSubject"): Id("r").Dot("NotifSubject"),
		}),
		)
	}

	g.Line().Comment("create durable pull subscription")
	g.Id("sub").Op(",").Id("err").Op(":=").Id("js").Dot("PullSubscribe").Call(
		Id("r").Dot("NotifSubject"),
		consumer,
		Qual(
			"github.com/nats-io/nats.go",
			"BindStream",
		).Call(Qual(
			fmt.Sprintf("%s/pkg/api/v0", modulePath),
			cc.StreamName,
		)),
	)
	g.If(Id("err").Op("!=").Nil()).Block(
		Id("log").Dot("Error").Call(
			Id("err"),
			Lit("failed to create pull subscription for reconciler notifications"),
			Lit("reconcilerName"),
			Id("r").Dot("Name"),
		),
		Qual("os", "Exit").Call(Lit(1)),
	)
}
