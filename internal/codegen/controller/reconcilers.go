package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
)

// controllerInternalPackagePath returns the path from the models to the
// controller's internal package where reconciler functions live.
func controllerInternalPackagePath(controllerShortName string) string {
	return filepath.Join("..", "..", "..", "internal", controllerShortName)
}

// Reconcilers generates the source code for a controller's reconcile functions.
func (cc *ControllerConfig) Reconcilers() error {
	controllerShortName := strings.TrimSuffix(cc.Name, "-controller")

	for _, obj := range cc.ReconciledObjects {
		f := NewFile(controllerShortName)
		f.HeaderComment("generated by 'threeport-codegen controller' - do not edit")

		f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "client")

		f.Comment(fmt.Sprintf("%[1]sReconciler reconciles system state when a %[1]s", obj))
		f.Comment("is created, updated or deleted.")
		f.Func().Id(fmt.Sprintf("%sReconciler", obj)).Params(
			Id("r").Op("*").Qual(
				"github.com/threeport/threeport/pkg/controller",
				"Reconciler",
			),
		).Block(
			Id("r").Dot("ShutdownWait").Dot("Add").Call(Lit(1)),
			Id("reconcilerLog").Op(":=").Id("r").Dot("Log").Dot("WithValues").Call(
				Lit("reconcilerName"), Id("r").Dot("Name"),
			),
			Id("reconcilerLog").Dot("Info").Call(Lit("reconciler started")),
			Id("shutdown").Op(":=").Lit(false),
			Line(),

			For().Block(
				Comment("create a fresh log object per reconciliation loop so we don't"),
				Comment("accumulate values across multiple loops"),
				Id("log").Op(":=").Id("r").Dot("Log").Dot("WithValues").Call(
					Lit("reconcilerName"), Id("r").Dot("Name"),
				),
				Line(),

				If(Id("shutdown")).Block(
					Break(),
				),
				Line(),

				Comment("check for shutdown instruction"),
				Select().Block(
					Case(Op("<-").Id("r").Dot("Shutdown")).Block(
						Id("shutdown").Op("=").Lit(true),
					),
					Default().Block(
						Comment("pull message off queue"),
						Id("msg").Op(":=").Id("r").Dot("PullMessage").Call(),
						If(Id("msg").Op("==").Nil()).Block(
							Continue(),
						),
						Line(),

						Comment("consume message data to capture notification from API"),
						Id("notif").Op(",").Id("err").Op(":=").Qual(
							"github.com/threeport/threeport/pkg/notifications",
							"ConsumeMessage",
						).Call(Id("msg").Dot("Data")),
						If(Id("err").Op("!=").Nil()).Block(
							Id("log").Dot("Error").Call(
								Line().Id("err"), Lit("failed to consume message data from NATS"),
								Line().Lit("msgSubject"), Id("msg").Dot("Subject"),
								Line().Lit("msgData"), Id("string").Call(Id("msg").Dot("Data")),
								Line(),
							),
							Go().Id("r").Dot("RequeueRaw").Call(Id("msg").Dot("Subject"), Id("msg").Dot("Data")),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
								Lit(fmt.Sprintf(
									"%s reconciliation requeued with identical payload and fixed delay",
									strcase.ToDelimited(obj, ' '),
								)),
							),
							Continue(),
						),
						Line(),

						Comment("decode the object that was created"),
						Var().Id(strcase.ToLowerCamel(obj)).Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							obj,
						),
						Qual(
							"github.com/mitchellh/mapstructure",
							"Decode",
						).Call(
							Id("notif").Dot("Object"), Op("&").Id(strcase.ToLowerCamel(obj)),
						),
						Id("log").Op("=").Id("log").Dot("WithValues").Call(
							Lit(fmt.Sprintf(
								"%sID",
								strcase.ToLowerCamel(obj),
							)).Op(",").Id(strcase.ToLowerCamel(obj)).Dot("ID"),
						),
						Line(),

						Comment("back off the requeue delay as needed"),
						Id("requeueDelay").Op(":=").Id("controller").Dot("SetRequeueDelay").Call(
							Line().Id("notif").Dot("LastRequeueDelay"),
							Line().Id("controller").Dot("DefaultInitialRequeueDelay"),
							Line().Id("controller").Dot("DefaultMaxRequeueDelay"),
							Line(),
						),
						Line(),

						Comment("build the notif payload for requeues"),
						Id("notifPayload").Op(",").Id("err").Op(":=").Id(strcase.ToLowerCamel(obj)).Dot("NotificationPayload").Call(
							Line().Id("notif").Dot("Operation"),
							Line().Lit(true),
							Line().Id("requeueDelay"),
							Line(),
						),
						If(Id("err").Op("!=").Nil()).Block(
							Id("log").Dot("Error").Call(
								Id("err").Op(",").Lit("failed to build notification payload for requeue"),
							),
							Go().Id("r").Dot("RequeueRaw").Call(
								Id("msg").Dot("Subject").Op(",").Id("msg").Dot("Data"),
							),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
								Lit(fmt.Sprintf(
									"%s reconciliation requeued with identical payload and fixed delay",
									strcase.ToDelimited(obj, ' '),
								)),
							),
							Continue(),
						),
						Line(),

						Comment("check for lock on object"),
						Id("locked").Op(",").Id("ok").Op(":=").Id("r").Dot("CheckLock").Call(
							Op("&").Id(strcase.ToLowerCamel(obj)),
						),
						If(Id("locked").Op("||").Id("ok").Op("==").Id("false")).Block(
							Go().Id("r").Dot("Requeue").Call(
								Op("&").Id(strcase.ToLowerCamel(obj)).Op(",").Id("msg").Dot("Subject").Op(",").Id("notifPayload").Op(",").Id("requeueDelay"),
							),
							Id("log").Dot("V").Call(
								Lit(1),
							).Dot("Info").Call(Lit(fmt.Sprintf(
								"%s reconciliation requeued",
								strcase.ToDelimited(obj, ' '),
							))),
							Continue(),
						),
						Line(),

						Comment("put a lock on the reconciliation of the created object"),
						If(Id("ok").Op(":=").Id("r").Dot("Lock").Call(
							Op("&").Id(strcase.ToLowerCamel(obj))).Op(";").Op("!").Id("ok"),
						).Block(
							Go().Id("r").Dot("Requeue").Call(
								Op("&").Id(strcase.ToLowerCamel(obj)).Op(",").Id("msg").Dot("Subject").Op(",").Id("notifPayload").Op(",").Id("requeueDelay"),
							),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
								"%s reconciliation requeued",
								strcase.ToDelimited(obj, ' '),
							))),
							Continue(),
						),
						Line(),

						Comment("retrieve latest version of object if requeued"),
						If(Id("notif").Dot("Requeue")).Block(
							Id(fmt.Sprintf(
								"latest%s",
								obj,
							)).Op(",").Id("err").Op(":=").Qual(
								"github.com/threeport/threeport/pkg/client/v0",
								fmt.Sprintf("Get%sByID", obj),
							).Call(
								Line().Op("*").Id(strcase.ToLowerCamel(obj)).Dot("ID"),
								Line().Id("r").Dot("APIServer"),
								Line().Lit(""),
								Line(),
							),
							Comment("check if error is 404 - if object no longer exists, no need to requeue"),
							If(Qual("errors", "Is").Call(Id("err"), Qual(
								"github.com/threeport/threeport/pkg/client/v0",
								"ErrorObjectNotFound",
							))).Block(
								Id("log").Dot("Info").Call(Qual(
									"fmt", "Sprintf",
								).Call(
									Line().Lit("object with ID %d no longer exists - halting reconciliation"),
									Line().Op("*").Id(fmt.Sprintf(
										"%s",
										strcase.ToLowerCamel(obj),
									)).Dot("ID"),
									Line(),
								)),
								Id("r").Dot("ReleaseLock").Call(Op("&").Id(strcase.ToLowerCamel(obj))),
								Continue(),
							),
							If(Id("err").Op("!=").Nil()).Block(
								Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
									"failed to get %s by ID from API",
									strcase.ToDelimited(obj, ' '),
								))),
								Id("r").Dot("UnlockAndRequeue").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("msg").Dot("Subject"), Id("notifPayload"), Id("requeueDelay")),
								Continue(),
							),
							Id(strcase.ToLowerCamel(obj)).Op("=").Op("*").Id(fmt.Sprintf(
								"latest%s",
								obj,
							)),
						),
						Line(),

						Comment("determine which operation and act accordingly"),
						Switch(Id("notif").Dot("Operation")).Block(
							Case(Qual(
								"github.com/threeport/threeport/pkg/notifications",
								"NotificationOperationCreated",
							)).Block(
								If(Err().Op(":=").Id(fmt.Sprintf(
									"%sCreated",
									strcase.ToLowerCamel(obj),
								)).Call(
									Id("r"),
									Op("&").Id(strcase.ToLowerCamel(obj)),
									Op("&").Id("log"),
								), Err().Op("!=").Nil()).Block(
									Id("log").Dot("Error").Call(
										Err(), Lit(fmt.Sprintf(
											"failed to reconcile created %s object",
											strcase.ToDelimited(obj, ' '),
										)),
									),
									Id("r").Dot("UnlockAndRequeue").Call(
										Line().Op("&").Id(strcase.ToLowerCamel(obj)),
										Line().Id("msg").Dot("Subject"),
										Line().Id("notifPayload"),
										Line().Id("requeueDelay"),
										Line(),
									),
									Continue(),
								),
							),
							Case(Qual(
								"github.com/threeport/threeport/pkg/notifications",
								"NotificationOperationDeleted",
							)).Block(
								If(Err().Op(":=").Id(fmt.Sprintf(
									"%sDeleted",
									strcase.ToLowerCamel(obj),
								)).Call(
									Id("r"),
									Op("&").Id(strcase.ToLowerCamel(obj)),
									Op("&").Id("log"),
								), Err().Op("!=").Nil()).Block(
									Id("log").Dot("Error").Call(
										Err(), Lit(fmt.Sprintf(
											"failed to reconcile deleted %s object",
											strcase.ToDelimited(obj, ' '),
										)),
									),
									Id("r").Dot("UnlockAndRequeue").Call(
										Line().Op("&").Id(strcase.ToLowerCamel(obj)),
										Line().Id("msg").Dot("Subject"),
										Line().Id("notifPayload"),
										Line().Id("requeueDelay"),
										Line(),
									),
									Continue(),
								),
							),
							Default().Block(
								Id("log").Dot("Error").Call(
									Line().Id("errors").Dot("New").Call(Lit("unrecognized notifcation operation")),
									Line().Lit("notification included an invalid operation"),
									Line(),
								),
								Id("r").Dot("UnlockAndRequeue").Call(
									Line().Op("&").Id(strcase.ToLowerCamel(obj)),
									Line().Id("msg").Dot("Subject"),
									Line().Id("notifPayload"),
									Line().Id("requeueDelay"),
									Line(),
								),
								Continue(),
							),
							Line(),
						),
						Line(),

						Comment("set the object's Reconciled field to true"),
						Id("objectReconciled").Op(":=").Lit(true),
						Id(fmt.Sprintf(
							"reconciled%s",
							obj,
						)).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							obj,
						).Values(Dict{
							Id("Common"): Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"Common",
							).Values(Dict{
								Id("ID"): Id(strcase.ToLowerCamel(obj)).Dot("ID"),
							}),
							Id("Reconciled"): Op("&").Id("objectReconciled"),
						}),
						Id(fmt.Sprintf(
							"updated%s",
							obj,
						)).Op(",").Id("err").Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							fmt.Sprintf("Update%s", obj),
						).Call(
							Line().Op("&").Id(fmt.Sprintf(
								"reconciled%s",
								obj,
							)),
							Line().Id("r").Dot("APIServer"),
							Line().Lit(""),
							Line(),
						),
						If(Id("err").Op("!=").Nil()).Block(
							Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
								"failed to update %s to mark as reconciled",
								strcase.ToDelimited(obj, ' '),
							))),
							Id("r").Dot("UnlockAndRequeue").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("msg").Dot("Subject"), Id("notifPayload"), Id("requeueDelay")),
							Continue(),
						),
						Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
							Line().Lit(fmt.Sprintf(
								"%s marked as reconciled in API",
								strcase.ToDelimited(obj, ' '),
							)),
							Line().Lit(fmt.Sprintf(
								"%sName",
								strcase.ToDelimited(obj, ' '),
							)), Id(fmt.Sprintf(
								"updated%s",
								obj,
							)).Dot("Name"),
							Line(),
						),
						Line(),

						Comment("release the lock on the reconciliation of the created object"),
						If(Id("ok").Op(":=").Id("r").Dot("ReleaseLock").Call(Op("&").Id(strcase.ToLowerCamel(obj))), Op("!").Id("ok")).Block(
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
								"%s remains locked - will unlock when TTL expires",
								strcase.ToDelimited(obj, ' '),
							))),
						).Else().Block(
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
								"%s unlocked",
								strcase.ToDelimited(obj, ' '),
							))),
						),
						Line(),

						Id("log").Dot("Info").Call(Lit(fmt.Sprintf(
							"%s successfully reconciled",
							strcase.ToDelimited(obj, ' '),
						))),
					),
				),
			),
			Line(),
			Id("r").Dot("Sub").Dot("Unsubscribe").Call(),
			Id("reconcilerLog").Dot("Info").Call(Lit("reconciler shutting down")),
			Id("r").Dot("ShutdownWait").Dot("Done").Call(),
		)

		// write code to file
		genFilename := fmt.Sprintf("%s_gen.go", strcase.ToSnake(obj))
		genFilepath := filepath.Join(controllerInternalPackagePath(controllerShortName), genFilename)
		file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed open file to write generated code for %s reconciler: %w", obj, err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code for %s reconciler: %w", obj, err)
		}
		fmt.Printf("code generation complete for %s reconciler\n", obj)
	}

	return nil
}
