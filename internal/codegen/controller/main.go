package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
)

// controllerMainPackagePath returns the path from the models to the
// controller's main package.
func controllerMainPackagePath(controllerName string) string {
	return filepath.Join("..", "..", "..", "cmd", controllerName)
}

// MainPackage generates the source code for a controller's main package.
func (cc *ControllerConfig) MainPackage() error {
	pluralize := pluralize.NewClient()
	f := NewFile("main")
	f.HeaderComment("generated by 'threeport-codegen controller' - do not edit")

	f.ImportAlias("github.com/threeport/threeport/internal/client", "clientInternal")

	controllerShortName := strings.TrimSuffix(cc.Name, "-controller")
	controllerStreamName := fmt.Sprintf("%sStreamName", strcase.ToCamel(controllerShortName))

	concurrencyFlags := &Statement{}
	for _, obj := range cc.ReconciledObjects {
		concurrencyFlags.Var().Id(fmt.Sprintf(
			"%sConcurrentReconciles",
			strcase.ToLowerCamel(obj),
		)).Op("=").Qual(
			"github.com/namsral/flag",
			"Int",
		).Call(
			Line().Lit(fmt.Sprintf(
				"%s-concurrent-reconciles",
				obj,
			)),
			Line().Lit(1),
			Line().Lit(fmt.Sprintf(
				"Number of concurrent reconcilers to run for %s",
				pluralize.Pluralize(strcase.ToDelimited(obj, ' '), 2, false),
			)),
			Line(),
		)
		concurrencyFlags.Line()
	}

	reconcilerConfigs := &Statement{}
	for _, obj := range cc.ReconciledObjects {
		reconcilerConfigs.Id("reconcilerConfigs").Op("=").Append(Id("reconcilerConfigs").Op(",").Qual(
			"github.com/threeport/threeport/pkg/controller",
			"ReconcilerConfig",
		).Values(Dict{
			Id("Name"): Lit(fmt.Sprintf("%sReconciler", obj)),
			Id("ObjectType"): Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				fmt.Sprintf("ObjectType%s", obj),
			),
			Id("ReconcileFunc"): Qual(
				fmt.Sprintf("github.com/threeport/threeport/internal/%s", controllerShortName),
				fmt.Sprintf("%sReconciler", obj),
			),
			Id("ConcurrentReconciles"): Op("*").Id(fmt.Sprintf(
				"%sConcurrentReconciles",
				strcase.ToLowerCamel(obj),
			)),
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
		Var().Id("log").Qual("github.com/go-logr/logr", "Logger"),
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
			Id("Bucket"):      Id(controllerShortName).Dot("LockBucketName"),
			Id("Description"): Id(controllerShortName).Dot("LockBucketDescr"),
			Id("TTL"):         Qual("time", "Minute").Op("*").Lit(20),
		}),
		List(
			Id("kv"), Id("err"),
		).Op(":=").Id("controller").Dot("CreateLockBucketIfNotExists").Call(Id("js"), Op("&").Id("kvConfig")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to bind to JetStream key-value locking bucket"),
				Lit("lockBucketName"),
				Id(controllerShortName).Dot("LockBucketName"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment(fmt.Sprintf(
			"check to ensure %s stream has been created by API",
			controllerShortName,
		)),
		Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(controllerStreamName),
		)).Op(":=").Lit(false),
		For(Id("stream").Op(":=").Range().Id("js").Dot("StreamNames").Call()).Block(
			If(Id("stream").Op("==").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				controllerStreamName,
			)).Block(
				Id(fmt.Sprintf(
					"%sFound",
					strcase.ToLowerCamel(controllerStreamName),
				)).Op("=").Lit(true),
			),
		),
		If(Op("!").Id(fmt.Sprintf(
			"%sFound",
			strcase.ToLowerCamel(controllerStreamName),
		))).Block(
			Id("log").Dot("Error").Call(
				Qual("errors", "New").Call(Lit("JetStream stream not found")),
				Lit(fmt.Sprintf(
					"failed to find stream with %s stream name",
					controllerShortName,
				)),
				Lit(strcase.ToLowerCamel(controllerStreamName)),
				Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					controllerStreamName,
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
		).Op(":=").Qual("github.com/threeport/threeport/internal/client", "GetHTTPClient").Call(Op("*").Id("authEnabled")),
		If(Err().Op("!=").Nil()).Block(
			Id("log").Dot("Error").Call(
				Err(),
				Lit("failed to create http client"),
			),
			Qual("os", "Exit").Call(Lit(1)),
		),

		Line().Comment("configure and start reconcilers"),
		Var().Id("reconcilerConfigs").Index().Qual(
			"github.com/threeport/threeport/pkg/controller",
			"ReconcilerConfig",
		),
		reconcilerConfigs,

		For(
			Id("_").Op(",").Id("r").Op(":=").Range().Id("reconcilerConfigs"),
		).Block(
			Line().Comment("create JetStream consumer"),
			Id("consumer").Op(":=").Id("r").Dot("Name").Op("+").Lit("Consumer"),
			Id("subject").Op(",").Id("err").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"GetSubjectByReconcilerName",
			).Call(Id("r").Dot("Name")),
			If(Id("err").Op("!=").Nil()).Block(
				Id("log").Dot("Error").Call(
					Id("err"),
					Lit("failed to get notification subject by reconciler name"),
					Lit("reconcilerName"),
					Id("r").Dot("Name"),
				),
				Qual("os", "Exit").Call(Lit(1)),
			),
			Id("js").Dot("AddConsumer").Call(Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				controllerStreamName,
			).Op(",").Op("&").Qual(
				"github.com/nats-io/nats.go",
				"ConsumerConfig",
			).Values(Dict{
				Id("Durable"): Id("consumer"),
				Id("AckPolicy"): Qual(
					"github.com/nats-io/nats.go",
					"AckExplicitPolicy",
				),
				Id("FilterSubject"): Id("subject"),
			}),
			),

			Line().Comment("create durable pull subscription"),
			Id("sub").Op(",").Id("err").Op(":=").Id("js").Dot("PullSubscribe").Call(
				Id("subject"),
				Id("consumer"),
				Qual(
					"github.com/nats-io/nats.go",
					"BindStream",
				).Call(Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					controllerStreamName,
				)),
			),

			Line().Comment("create exit channel"),
			Id("shutdownChan").Op(":=").Make(Chan().Bool(), Lit(1)),
			Id("shutdownChans").Op("=").Append(Id("shutdownChans"), Id("shutdownChan")),

			Line().Comment("create reconciler"),
			Id("reconciler").Op(":=").Id("controller").Dot("Reconciler").Values(Dict{
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
			}),

			Line().Comment("start reconciler"),
			Id("go").Id("r").Dot("ReconcileFunc").Call(Op("&").Id("reconciler")),
		),
		Line(),

		Id("log").Dot("Info").Call(
			Line().Lit(fmt.Sprintf("%s controller started", controllerShortName)),
			Line().Lit("version"), Qual(
				"github.com/threeport/threeport/internal/version",
				"GetVersion",
			).Call(),
			Line().Lit("controllerID"), Id("controllerID").Dot("String").Call(),
			Line().Lit("NATSConnection"), Id("natsConn"),
			Line().Lit("lockBucketName"), Id(controllerShortName).Dot("LockBucketName"),
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

		Id("log").Dot("Info").Call(Lit(fmt.Sprintf("%s controller shutting down", controllerShortName))),
		Qual("os", "Exit").Call(Lit(0)),
	)

	f.Func().Id("showHelpAndExit").Params(Id("exitCode").Int()).Block(
		Qual("fmt", "Printf").Call(Lit(fmt.Sprintf("Usage: threeport-%s-controller [options]\n", controllerShortName))),
		Qual("fmt", "Println").Call(Lit("options:")),
		Id("flag").Dot("PrintDefaults").Call(),
		Qual("os", "Exit").Call(Id("exitCode")),
	)

	// write code to file
	genFilename := "main_gen.go"
	genFilepath := filepath.Join(controllerMainPackagePath(cc.Name), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed open file to write generated code for %s main package: %w", cc.Name, err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for %s main package: %w", cc.Name, err)
	}
	fmt.Printf("code generation complete for %s main package\n", cc.Name)

	return nil

}
