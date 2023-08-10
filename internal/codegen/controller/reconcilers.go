package controller

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
)

// controllerInternalPackagePath returns the path from the models to the
// controller's internal package where reconciler functions live.
func controllerInternalPackagePath(packageName string) string {
	return filepath.Join("..", "..", "..", "internal", packageName)
}

// Reconcilers generates the source code for a controller's reconcile functions.
func (cc *ControllerConfig) Reconcilers() error {
	for _, obj := range cc.ReconciledObjects {
		f := NewFile(cc.PackageName)
		f.HeaderComment("generated by 'threeport-codegen controller' - do not edit")

		f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "client")
		f.ImportAlias("github.com/threeport/threeport/pkg/controller/v0", "controller")
		f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")

		f.Comment(fmt.Sprintf("%[1]sReconciler reconciles system state when a %[1]s", obj))
		f.Comment("is created, updated or deleted.")
		f.Func().Id(fmt.Sprintf("%sReconciler", obj)).Params(
			Id("r").Op("*").Qual(
				"github.com/threeport/threeport/pkg/controller/v0",
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

			Comment("create a channel to receive OS signals"),
			Id("osSignals").Op(":=").Make(
				Chan().Qual("os", "Signal"),
				Lit(1),
			),
			Id("lockReleased").Op(":=").Make(
				Chan().Bool(),
				Lit(1),
			),
			Line(),
			Comment("register the os signals channel to receive SIGINT and SIGTERM signals"),
			Qual("os/signal", "Notify").Call(
				Id("osSignals"),
				Qual("syscall", "SIGINT"),
				Qual("syscall", "SIGTERM"),
			),
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
							"github.com/threeport/threeport/pkg/notifications/v0",
							"ConsumeMessage",
						).Call(Id("msg").Dot("Data")),
						If(Id("err").Op("!=").Nil()).Block(
							Id("log").Dot("Error").Call(
								Line().Id("err"), Lit("failed to consume message data from NATS"),
								Line().Lit("msgData"), Id("string").Call(Id("msg").Dot("Data")),
								Line(),
							),
							Id("r").Dot("RequeueRaw").Call(Id("msg")),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
								Lit(fmt.Sprintf(
									"%s reconciliation requeued with identical payload and fixed delay",
									strcase.ToDelimited(obj, ' '),
								)),
							),
							Continue(),
						),
						Line(),

						Comment("decode the object that was sent in the notification"),
						Var().Id(strcase.ToLowerCamel(obj)).Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							obj,
						),
						If(Id("err").Op(":=").Id(strcase.ToLowerCamel(obj)).Dot("DecodeNotifObject").Call(
							Id("notif").Dot("Object"),
						).Op(";").Id("err").Op("!=").Nil().Block(
							Id("log").Dot("Error").Call(
								Id("err"), Lit("failed to marshal object map from consumed notification message"),
							),
							Id("r").Dot("RequeueRaw").Call(Id("msg")),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
								Lit(fmt.Sprintf(
									"%s reconciliation requeued with identical payload and fixed delay",
									strcase.ToDelimited(obj, ' '),
								)),
							),
							Continue(),
						)),
						Id("log").Op("=").Id("log").Dot("WithValues").Call(
							Lit(fmt.Sprintf(
								"%sID",
								strcase.ToLowerCamel(obj),
							)).Op(",").Id(strcase.ToLowerCamel(obj)).Dot("ID"),
						),
						Line(),

						Comment("back off the requeue delay as needed"),
						Id("requeueDelay").Op(":=").Id("controller").Dot("SetRequeueDelay").Call(
							Line().Id("notif").Dot("CreationTime"),
							Line(),
						),
						Line(),

						Comment("check for lock on object"),
						Id("locked").Op(",").Id("ok").Op(":=").Id("r").Dot("CheckLock").Call(
							Op("&").Id(strcase.ToLowerCamel(obj)),
						),
						If(Id("locked").Op("||").Id("ok").Op("==").Id("false")).Block(
							Id("r").Dot("Requeue").Call(
								Op("&").Id(strcase.ToLowerCamel(obj)).Op(",").Id("requeueDelay").Op(",").Id("msg"),
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
						Comment("set up handler to unlock and requeue on termination signal"),
						Go().Func().Params().Block(
							Select().Block(
								Case(Op("<-").Id("osSignals")).Block(
									Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
										"received termination signal, performing unlock and requeue of %s",
										strcase.ToDelimited(obj, ' '),
									))),
									Id("r").Dot("UnlockAndRequeue").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("requeueDelay"), Id("lockReleased"), Id("msg")),
								),
								Case(Op("<-").Id("lockReleased")).Block(
									Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
										"reached end of reconcile loop for %s, closing out signal handler",
										strcase.ToDelimited(obj, ' '),
									))),
								),
							),
						).Call(),
						Line(),
						Comment("put a lock on the reconciliation of the created object"),
						If(Id("ok").Op(":=").Id("r").Dot("Lock").Call(
							Op("&").Id(strcase.ToLowerCamel(obj))).Op(";").Op("!").Id("ok"),
						).Block(
							Id("r").Dot("Requeue").Call(
								Op("&").Id(strcase.ToLowerCamel(obj)).Op(",").Id("requeueDelay").Op(",").Id("msg"),
							),
							Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
								"%s reconciliation requeued",
								strcase.ToDelimited(obj, ' '),
							))),
							Continue(),
						),
						Line(),

						Comment("retrieve latest version of object unless object was"),
						Comment("deleted (in which case we have the latest version)"),
						If(
							Id("notif").Dot("Operation").Op("!=").Qual(
								"github.com/threeport/threeport/pkg/notifications/v0",
								"NotificationOperationDeleted",
							),
						).Block(
							Id(fmt.Sprintf(
								"latest%s",
								obj,
							)).Op(",").Id("err").Op(":=").Qual(
								"github.com/threeport/threeport/pkg/client/v0",
								fmt.Sprintf("Get%sByID", obj),
							).Call(
								Line().Id("r").Dot("APIClient"),
								Line().Id("r").Dot("APIServer"),
								Line().Op("*").Id(strcase.ToLowerCamel(obj)).Dot("ID"),
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
								Id("r").Dot("ReleaseLock").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("lockReleased"), Id("msg"), Lit(true)),
								Continue(),
							),
							If(Id("err").Op("!=").Nil()).Block(
								Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
									"failed to get %s by ID from API",
									strcase.ToDelimited(obj, ' '),
								))),
								Id("r").Dot("UnlockAndRequeue").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("requeueDelay"), Id("lockReleased"), Id("msg")),
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
								"github.com/threeport/threeport/pkg/notifications/v0",
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
										Line().Id("requeueDelay"),
										Line().Id("lockReleased"),
										Line().Id("msg"),
										Line(),
									),
									Continue(),
								),
							),
							Case(Qual(
								"github.com/threeport/threeport/pkg/notifications/v0",
								"NotificationOperationUpdated",
							)).Block(
								If(Err().Op(":=").Id(fmt.Sprintf(
									"%sUpdated",
									strcase.ToLowerCamel(obj),
								)).Call(
									Id("r"),
									Op("&").Id(strcase.ToLowerCamel(obj)),
									Op("&").Id("log"),
								), Err().Op("!=").Nil()).Block(
									Id("log").Dot("Error").Call(
										Err(), Lit(fmt.Sprintf(
											"failed to reconcile updated %s object",
											strcase.ToDelimited(obj, ' '),
										)),
									),
									Id("r").Dot("UnlockAndRequeue").Call(
										Line().Op("&").Id(strcase.ToLowerCamel(obj)),
										Line().Id("requeueDelay"),
										Line().Id("lockReleased"),
										Line().Id("msg"),
										Line(),
									),
									Continue(),
								),
							),
							Case(Qual(
								"github.com/threeport/threeport/pkg/notifications/v0",
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
										Line().Id("requeueDelay"),
										Line().Id("lockReleased"),
										Line().Id("msg"),
										Line(),
									),
								).Else().Block(
									Id("r").Dot("ReleaseLock").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("lockReleased"), Id("msg"), Lit(true)),
									Id("log").Dot("Info").Call(Lit(fmt.Sprintf(
										"%s successfully reconciled",
										strcase.ToDelimited(obj, ' '),
									))),
								),
								Continue(),
							),
							Default().Block(
								Id("log").Dot("Error").Call(
									Line().Id("errors").Dot("New").Call(Lit("unrecognized notifcation operation")),
									Line().Lit("notification included an invalid operation"),
									Line(),
								),
								Id("r").Dot("UnlockAndRequeue").Call(
									Line().Op("&").Id(strcase.ToLowerCamel(obj)),
									Line().Id("requeueDelay"),
									Line().Id("lockReleased"),
									Line().Id("msg"),
									Line(),
								),
								Continue(),
							),
							Line(),
						),
						Line(),

						Comment("set the object's Reconciled field to true if not deleted"),
						If(Id("notif").Dot("Operation").Op("!=").Qual(
							"github.com/threeport/threeport/pkg/notifications/v0",
							"NotificationOperationDeleted",
						).Block(
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
								Line().Id("r").Dot("APIClient"),
								Line().Id("r").Dot("APIServer"),
								Line().Op("&").Id(fmt.Sprintf(
									"reconciled%s",
									obj,
								)),
								Line(),
							),
							If(Id("err").Op("!=").Nil()).Block(
								Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
									"failed to update %s to mark as reconciled",
									strcase.ToDelimited(obj, ' '),
								))),
								Id("r").Dot("UnlockAndRequeue").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("requeueDelay"), Id("lockReleased"), Id("msg")),
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
						)),
						Line(),

						Comment("release the lock on the reconciliation of the created object"),
						If(Id("ok").Op(":=").Id("r").Dot("ReleaseLock").Call(Op("&").Id(strcase.ToLowerCamel(obj)), Id("lockReleased"), Id("msg"), Lit(true)), Op("!").Id("ok")).Block(
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
		genFilepath := filepath.Join(controllerInternalPackagePath(cc.PackageName), genFilename)
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
