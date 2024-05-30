package controller

import (
	"fmt"
	"path/filepath"

	"github.com/dave/jennifer/jen"
	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenControllerMain generates source code for controllers' main packages.
func GenControllerMain(gen *gen.Generator) error {
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			pluralize := pluralize.NewClient()
			f := NewFile("main")
			f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

			f.ImportAlias("github.com/threeport/threeport/pkg/client/lib/v0", "tpclient_lib")
			f.ImportAlias("github.com/threeport/threeport/pkg/controller/v0", "controller")
			f.ImportAlias("github.com/threeport/threeport/pkg/runtime/v0", "runtime")
			f.ImportAlias(fmt.Sprintf("%s/pkg/config/v0", gen.ModulePath), "config")

			concurrencyFlags := &Statement{}
			for _, obj := range objGroup.ReconciledObjects {
				concurrencyFlags.Var().Id(fmt.Sprintf(
					fmt.Sprintf("%sConcurrentReconciles", strcase.ToLowerCamel(obj.Name)),
				)).Op("=").Qual(
					"github.com/namsral/flag",
					"Int",
				).Call(
					Line().Lit(
						fmt.Sprintf("%s-concurrent-reconciles", strcase.ToKebab(obj.Name)),
					),
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
			for _, obj := range objGroup.ReconciledObjects {
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
					Id("ReconcileFunc"): Qual(
						fmt.Sprintf("%s/internal/%s", gen.ModulePath, objGroup.ControllerShortName),
						fmt.Sprintf("%sReconciler", obj.Name),
					),
					Id("ConcurrentReconciles"): Op("*").Id(
						fmt.Sprintf("%sConcurrentReconciles", strcase.ToLowerCamel(obj.Name)),
					),
					Id("NotifSubject"): Qual(
						fmt.Sprintf("%s/internal/%s/notif", gen.ModulePath, objGroup.ControllerShortName),
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
							"%s/internal/%s",
							gen.ModulePath,
							objGroup.ControllerShortName,
						),
						"LockBucketName",
					),
					Id("Description"): Qual(
						fmt.Sprintf(
							"%s/internal/%s",
							gen.ModulePath,
							objGroup.ControllerShortName,
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
								gen.ModulePath,
								objGroup.ControllerShortName,
							),
							"LockBucketName",
						),
					),
					Qual("os", "Exit").Call(Lit(1)),
				),

				Line().Comment(fmt.Sprintf(
					"check to ensure %s stream has been created by API",
					objGroup.ControllerShortName,
				)),
				Id(fmt.Sprintf(
					"%sFound",
					strcase.ToLowerCamel(objGroup.StreamName),
				)).Op(":=").Lit(false),
				For(Id("stream").Op(":=").Range().Id("js").Dot("StreamNames").Call()).Block(
					If(Id("stream").Op("==").Qual(
						fmt.Sprintf(
							"%s/internal/%s/notif",
							gen.ModulePath,
							objGroup.ControllerShortName,
						),
						objGroup.StreamName,
					)).Block(
						Id(fmt.Sprintf(
							"%sFound",
							strcase.ToLowerCamel(objGroup.StreamName),
						)).Op("=").Lit(true),
					),
				),
				If(Op("!").Id(fmt.Sprintf(
					"%sFound",
					strcase.ToLowerCamel(objGroup.StreamName),
				))).Block(
					Id("log").Dot("Error").Call(
						Qual("errors", "New").Call(Lit("JetStream stream not found")),
						Lit(fmt.Sprintf(
							"failed to find stream with %s stream name",
							objGroup.ControllerShortName,
						)),
						Lit(strcase.ToLowerCamel(objGroup.StreamName)),
						Qual(
							fmt.Sprintf(
								"%s/internal/%s/notif",
								gen.ModulePath,
								objGroup.ControllerShortName,
							),
							objGroup.StreamName,
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
				).Op(":=").Qual(
					"github.com/threeport/threeport/pkg/client/lib/v0",
					"GetHTTPClient",
				).Call(
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
				).BlockFunc(func(g *jen.Group) {
					ConfigurePullSubscription(g, objGroup, durable, gen.ModulePath)
					g.Line().Comment("create exit channel")
					g.Id("shutdownChan").Op(":=").Make(Chan().Bool(), Lit(1))
					g.Id("shutdownChans").Op("=").Append(Id("shutdownChans"), Id("shutdownChan"))

					g.Line().Comment("create reconciler")
					g.Id("reconciler").Op(":=").Id("controller").Dot("Reconciler").Values(Dict{
						Id("Name"):             Id("r").Dot("Name"),
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
						}),
					})

					g.Line().Comment("start reconciler")
					g.Id("go").Id("r").Dot("ReconcileFunc").Call(Op("&").Id("reconciler"))
				}),
				Line(),

				Id("log").Dot("Info").Call(
					Line().Lit(fmt.Sprintf("%s controller started", objGroup.ControllerShortName)),
					Line().Lit("version"), Qual(
						fmt.Sprintf("%s/internal/version", gen.ModulePath),
						"GetVersion",
					).Call(),
					Line().Lit("controllerID"), Id("controllerID").Dot("String").Call(),
					Line().Lit("NATSConnection"), Id("natsConn"),
					Line().Lit("lockBucketName"), Qual(
						fmt.Sprintf(
							"%s/internal/%s",
							gen.ModulePath,
							objGroup.ControllerShortName,
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

				Id("log").Dot("Info").Call(Lit(fmt.Sprintf("%s controller shutting down", objGroup.ControllerShortName))),
				Qual("os", "Exit").Call(Lit(0)),
			)

			f.Func().Id("showHelpAndExit").Params(Id("exitCode").Int()).Block(
				Qual("fmt", "Printf").Call(Lit(fmt.Sprintf(
					"Usage: threeport-%s-controller [options]\n",
					objGroup.ControllerShortName,
				))),
				Qual("fmt", "Println").Call(Lit("options:")),
				Qual(
					"github.com/namsral/flag",
					"PrintDefaults",
				).Call(),
				Qual("os", "Exit").Call(Id("exitCode")),
			)

			// write code to file
			genFilepath := filepath.Join("cmd", objGroup.ControllerName, "main_gen.go")
			_, err := util.WriteCodeToFile(f, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf("source code for API main package written to %s", genFilepath))
		}
	}

	return nil
}

// ConfigurePullSubscription adds a durable consumer to a controller's main package.
func ConfigurePullSubscription(
	g *jen.Group,
	objGroup gen.ApiObjectGroup,
	durable bool,
	modulePath string,
) {
	consumer := Lit("")
	if durable {
		consumer = Id("consumer")
		g.Line().Comment("create JetStream consumer")
		g.Id("consumer").Op(":=").Id("r").Dot("Name").Op("+").Lit("Consumer")
		g.Id("js").Dot("AddConsumer").Call(Qual(
			fmt.Sprintf(
				"%s/internal/%s/notif",
				modulePath,
				objGroup.ControllerShortName,
			),
			objGroup.StreamName,
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
			fmt.Sprintf(
				"%s/internal/%s/notif",
				modulePath,
				objGroup.ControllerShortName,
			),
			objGroup.StreamName,
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
