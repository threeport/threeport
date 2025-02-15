package controller

import (
	"fmt"
	"path/filepath"

	"github.com/dave/jennifer/jen"
	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenReconcilers generates the reconciler boilerplate for each of a
// controller's reconcilers.
func GenReconcilers(gen *gen.Generator) error {
	for _, objGroup := range gen.ApiObjectGroups {
		for _, obj := range objGroup.ReconciledObjects {
			varObjectName := strcase.ToLowerCamel(obj.Name)

			f := NewFile(objGroup.ControllerPackageName)
			f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

			f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi_v0")
			f.ImportAlias("github.com/threeport/threeport/pkg/api/lib/v0", "tpapi_lib")
			f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "tpclient")
			f.ImportAlias("github.com/threeport/threeport/pkg/client/lib/v0", "tpclient_lib")
			f.ImportAlias("github.com/threeport/threeport/pkg/controller/v0", "controller")
			f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")
			f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
			f.ImportAlias("github.com/threeport/threeport/pkg/event/v0", "event")

			for _, version := range obj.Versions {
				f.ImportAlias(
					fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, version),
					fmt.Sprintf("api_%s", version),
				)
				f.ImportAlias(
					fmt.Sprintf("%s/pkg/client/%s", gen.ModulePath, version),
					fmt.Sprintf("client_%s", version),
				)
			}

			f.Comment(fmt.Sprintf("%[1]sReconciler reconciles system state when a %[1]s", obj.Name))
			f.Comment("is created, updated or deleted.")
			f.Func().Id(fmt.Sprintf("%sReconciler", obj.Name)).Params(
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
						Default().BlockFunc(func(g *Group) {
							g.Comment("pull message off queue")
							g.Id("msg").Op(":=").Id("r").Dot("PullMessage").Call()
							g.If(Id("msg").Op("==").Nil()).Block(
								Continue(),
							)
							g.Line()

							g.Comment("consume message data to capture notification from API")
							g.Id("notif").Op(",").Id("err").Op(":=").Qual(
								"github.com/threeport/threeport/pkg/notifications/v0",
								"ConsumeMessage",
							).Call(Id("msg").Dot("Data"))
							g.If(Id("err").Op("!=").Nil()).Block(
								Id("log").Dot("Error").Call(
									Line().Id("err"), Lit("failed to consume message data from NATS"),
									Line().Lit("msgData"), Id("string").Call(Id("msg").Dot("Data")),
									Line(),
								),
								Id("r").Dot("RequeueRaw").Call(Id("msg")),
								Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
									Lit(fmt.Sprintf(
										"%s reconciliation requeued with identical payload and fixed delay",
										strcase.ToDelimited(obj.Name, ' '),
									)),
								),
								Continue(),
							)
							g.Line()

							g.Comment("determine the correct object version from the notification")
							g.Var().Id(varObjectName).Qual(
								"github.com/threeport/threeport/pkg/api/lib/v0",
								"ReconciledThreeportApiObject",
							)
							g.Switch(Id("notif").Dot("ObjectVersion")).BlockFunc(func(h *Group) {
								for _, version := range obj.Versions {
									h.Case(Lit(version)).Block(
										Id(varObjectName).Op("=").Op("&").Qual(
											fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, version),
											obj.Name,
										).Values(),
									)
								}
								h.Default().Block(
									Id("log").Dot("Error").Call(
										Qual("errors", "New").Call(
											Lit(fmt.Sprintf(
												"received unrecognized version of %s object",
												strcase.ToDelimited(obj.Name, ' '),
											)),
										),
										Lit(""),
									),
									Id("r").Dot("RequeueRaw").Call(Id("msg")),
									Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
										Lit(fmt.Sprintf(
											"%s reconciliation requeued with identical payload and fixed delay",
											strcase.ToDelimited(obj.Name, ' '),
										)),
									),
									Continue(),
								)
							})
							g.Line()

							g.Comment("decode the object that was sent in the notification")
							g.If(Id("err").Op(":=").Id(varObjectName).Dot("DecodeNotifObject").Call(
								Id("notif").Dot("Object"),
							).Op(";").Id("err").Op("!=").Nil().Block(
								Id("log").Dot("Error").Call(
									Id("err"), Lit("failed to marshal object map from consumed notification message"),
								),
								Id("r").Dot("RequeueRaw").Call(Id("msg")),
								Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
									Lit(fmt.Sprintf(
										"%s reconciliation requeued with identical payload and fixed delay",
										strcase.ToDelimited(obj.Name, ' '),
									)),
								),
								Continue(),
							))
							g.Id("log").Op("=").Id("log").Dot("WithValues").Call(
								Lit(fmt.Sprintf(
									"%sID",
									varObjectName,
								)).Op(",").Id(varObjectName).Dot("GetId").Call(),
							)
							g.Line()

							g.Comment("back off the requeue delay as needed")
							g.Id("requeueDelay").Op(":=").Id("controller").Dot("SetRequeueDelay").Call(
								Line().Id("notif").Dot("CreationTime"),
								Line(),
							)
							g.Line()

							g.Comment("check for lock on object")
							g.Id("locked").Op(",").Id("ok").Op(":=").Id("r").Dot("CheckLock").Call(
								Id(varObjectName),
							)
							g.If(Id("locked").Op("||").Id("ok").Op("==").Id("false")).Block(
								Id("r").Dot("Requeue").Call(
									Id(varObjectName).Op(",").Id("requeueDelay").Op(",").Id("msg"),
								),
								Id("log").Dot("V").Call(
									Lit(1),
								).Dot("Info").Call(Lit(fmt.Sprintf(
									"%s reconciliation requeued",
									strcase.ToDelimited(obj.Name, ' '),
								))),
								Continue(),
							)
							g.Line()
							g.Comment("set up handler to unlock and requeue on termination signal")
							g.Go().Func().Params().Block(
								Select().Block(
									Case(Op("<-").Id("osSignals")).Block(
										Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
											"received termination signal, performing unlock and requeue of %s",
											strcase.ToDelimited(obj.Name, ' '),
										))),
										Id("r").Dot("UnlockAndRequeue").Call(
											Id(varObjectName), Id("requeueDelay"), Id("lockReleased"), Id("msg"),
										),
									),
									Case(Op("<-").Id("lockReleased")).Block(
										Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
											"reached end of reconcile loop for %s, closing out signal handler",
											strcase.ToDelimited(obj.Name, ' '),
										))),
									),
								),
							).Call()
							g.Line()
							g.Comment("put a lock on the reconciliation of the created object")
							g.If(Id("ok").Op(":=").Id("r").Dot("Lock").Call(
								Id(varObjectName)).Op(";").Op("!").Id("ok"),
							).Block(
								Id("r").Dot("Requeue").Call(
									Id(varObjectName).Op(",").Id("requeueDelay").Op(",").Id("msg"),
								),
								Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
									"%s reconciliation requeued",
									strcase.ToDelimited(obj.Name, ' '),
								))),
								Continue(),
							)
							g.Line()

							// If the object has a "Data" field with a "persist" tag set to "false", skip
							// the retrieval of the latest object. Otherwise, generate the
							// source code to retrieve the latest object.
							if !objGroup.CheckStructTagMap(obj.Name, "Data", "persist", "false") {
								getLatestObject(g, &obj, gen.ModulePath)
							}

							g.Line()
							g.Comment("determine which operation and act accordingly")
							g.Switch(Id("notif").Dot("Operation")).BlockFunc(func(h *Group) {
								operationCase(h, "create", &obj, varObjectName, gen.ModulePath)
								operationCase(h, "update", &obj, varObjectName, gen.ModulePath)
								operationCase(h, "delete", &obj, varObjectName, gen.ModulePath)
								h.Default().Block(
									Id("log").Dot("Error").Call(
										Line().Id("errors").Dot("New").Call(Lit("unrecognized notifcation operation")),
										Line().Lit("notification included an invalid operation"),
										Line(),
									),
									Id("r").Dot("UnlockAndRequeue").Call(
										Line().Id(varObjectName),
										Line().Id("requeueDelay"),
										Line().Id("lockReleased"),
										Line().Id("msg"),
										Line(),
									),
									Continue(),
								)
							})
							g.Line()

							g.Comment("set the object's Reconciled field to true if not deleted")
							g.If(Id("notif").Dot("Operation").Op("!=").Qual(
								"github.com/threeport/threeport/pkg/notifications/v0",
								"NotificationOperationDeleted",
							).Block(
								Id(fmt.Sprintf(
									"reconciled%s",
									obj.Name,
								)).Op(":=").Qual(
									fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
									obj.Name,
								).Values(Dict{
									Id("Common"): Qual(
										"github.com/threeport/threeport/pkg/api/v0",
										"Common",
									).Values(Dict{
										Id("ID"): Qual(
											"github.com/threeport/threeport/pkg/util/v0",
											"Ptr",
										).Call(Id(varObjectName).Dot("GetId").Call()),
									}),
									Id("Reconciliation"): Qual(
										"github.com/threeport/threeport/pkg/api/v0",
										"Reconciliation",
									).Values(Dict{
										Id("Reconciled"): Qual(
											"github.com/threeport/threeport/pkg/util/v0",
											"Ptr",
										).Call(Lit(true)),
									}),
								}),
								Id(fmt.Sprintf(
									"updated%s",
									obj.Name,
								)).Op(",").Id("err").Op(":=").Qual(
									fmt.Sprintf("%s/pkg/client/v0", gen.ModulePath),
									fmt.Sprintf("Update%s", obj.Name),
								).Call(
									Line().Id("r").Dot("APIClient"),
									Line().Id("r").Dot("APIServer"),
									Line().Op("&").Id(fmt.Sprintf(
										"reconciled%s",
										obj.Name,
									)),
									Line(),
								),
								If(Id("err").Op("!=").Nil()).Block(
									Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
										"failed to update %s to mark as reconciled",
										strcase.ToDelimited(obj.Name, ' '),
									))),
									Id("r").Dot("UnlockAndRequeue").Call(
										Id(varObjectName),
										Id("requeueDelay"),
										Id("lockReleased"),
										Id("msg"),
									),
									Continue(),
								),
								Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(
									Line().Lit(fmt.Sprintf(
										"%s marked as reconciled in API",
										strcase.ToDelimited(obj.Name, ' '),
									)),
									Line().Lit(fmt.Sprintf(
										"%sName",
										strcase.ToDelimited(obj.Name, ' '),
									)), Id(fmt.Sprintf(
										"updated%s",
										obj.Name,
									)).Dot("Name"),
									Line(),
								),
							))
							g.Line()

							g.Comment("release the lock on the reconciliation of the created object")
							g.If(Id("ok").Op(":=").Id("r").Dot("ReleaseLock").Call(
								Id(varObjectName),
								Id("lockReleased"),
								Id("msg"),
								Lit(true)),
								Op("!").Id("ok"),
							).Block(
								Id("log").Dot("Error").Call(
									Qual("errors", "New").Call(
										Lit(fmt.Sprintf(
											"%s remains locked - will unlock when TTL expires",
											strcase.ToDelimited(obj.Name, ' '),
										)),
									),
									Lit(""),
								),
							).Else().Block(
								Id("log").Dot("V").Call(Lit(1)).Dot("Info").Call(Lit(fmt.Sprintf(
									"%s unlocked",
									strcase.ToDelimited(obj.Name, ' '),
								))),
							)
							g.Line()

							g.Comment("log and record event for successful reconciliation")
							g.Id("successMsg").Op(":=").Qual("fmt", "Sprintf").Call(
								Line().Lit(fmt.Sprintf("%s successfully reconciled for %%s operation", strcase.ToDelimited(obj.Name, ' '))),
								Line().Qual("strings", "ToLower").Call(Id("string").Call(Id("notif").Dot("Operation"))),
								Line(),
							)
							g.If(Id("err").Op(":=").Id("r").Dot("EventsRecorder").Dot("RecordEvent").Call(
								Line().Op("&").Qual("github.com/threeport/threeport/pkg/api/v0", "Event").Values(Dict{
									Id("Reason"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
										Qual("github.com/threeport/threeport/pkg/event/v0", "GetSuccessReasonForOperation").Call(Id("notif").Dot("Operation")),
									),
									Id("Note"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(Id("successMsg")),
									Id("Type"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
										Qual("github.com/threeport/threeport/pkg/event/v0", "TypeNormal"),
									),
								}),
								Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetId").Call(),
								Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetVersion").Call(),
								Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetType").Call(),
								Line(),
							).Op(";").Id("err").Op("!=").Nil().Block(
								Id("log").Dot("Error").Call(
									Err(),
									Lit(fmt.Sprintf("failed to record event for successful %s reconciliation", strcase.ToDelimited(obj.Name, ' '))),
								),
							))
							g.Id("log").Dot("Info").Call(Id("successMsg"))
						}),
					),
				),
				Line(),
				Id("r").Dot("Sub").Dot("Unsubscribe").Call(),
				Id("reconcilerLog").Dot("Info").Call(Lit("reconciler shutting down")),
				Id("r").Dot("ShutdownWait").Dot("Done").Call(),
			)

			// write code to file
			genFilepath := filepath.Join(
				"internal",
				objGroup.ControllerShortName,
				fmt.Sprintf("%s_gen.go", strcase.ToSnake(varObjectName)),
			)
			_, err := util.WriteCodeToFile(f, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf("source code for controller reconciler written to %s", genFilepath))
		}
	}

	return nil
}

// getLatestObject generates the source code for a controller's reconcile functions
// to get the latest object if the "persist" field is not present or set to true.
func getLatestObject(
	g *jen.Group,
	obj *gen.ReconciledObject,
	modulePath string,
) {
	objVar := strcase.ToLowerCamel(obj.Name)
	latestObjVar := fmt.Sprintf("latest%s", obj.Name)
	latestObjErrVar := "getLatestErr"

	g.Comment("retrieve latest version of object")

	g.Var().Id(latestObjVar).Qual(
		"github.com/threeport/threeport/pkg/api/lib/v0",
		"ReconciledThreeportApiObject",
	)
	g.Var().Id(latestObjErrVar).Error()
	g.Switch(Id("notif").Dot("ObjectVersion")).BlockFunc(func(h *Group) {
		for _, version := range obj.Versions {
			h.Case(Lit(version)).Block(
				List(Id("latestObject"), Err()).Op(":=").Qual(
					fmt.Sprintf("%s/pkg/client/%s", modulePath, version),
					fmt.Sprintf("Get%sByID", obj.Name),
				).Call(
					Line().Id("r").Dot("APIClient"),
					Line().Id("r").Dot("APIServer"),
					Line().Id(objVar).Dot("GetId").Call(),
					Line(),
				),
				Id(latestObjVar).Op("=").Id("latestObject"),
				Id(latestObjErrVar).Op("=").Err(),
			)
		}
		h.Default().Block(
			Id(latestObjErrVar).Op("=").Qual("errors", "New").Call(
				Lit(fmt.Sprintf(
					"received unrecognized version of %s object",
					strcase.ToDelimited(obj.Name, ' '),
				)),
			),
		)
	})
	g.Line()

	g.Comment("check if error is 404 - if object no longer exists, no need to requeue")
	g.If(Qual("errors", "Is").Call(Id(latestObjErrVar), Qual(
		"github.com/threeport/threeport/pkg/client/lib/v0",
		"ErrObjectNotFound",
	))).Block(
		Id("log").Dot("Info").Call(
			Lit("object no longer exists - halting reconciliation"),
		),
		Id("r").Dot("ReleaseLock").Call(Id(objVar), Id("lockReleased"), Id("msg"), Lit(true)),
		Continue(),
	)
	g.If(Id(latestObjErrVar).Op("!=").Nil()).Block(
		Id("log").Dot("Error").Call(Id(latestObjErrVar), Lit(fmt.Sprintf(
			"failed to get %s by ID from API",
			strcase.ToDelimited(obj.Name, ' '),
		))),
		Id("r").Dot("UnlockAndRequeue").Call(Id(objVar), Id("requeueDelay"), Id("lockReleased"), Id("msg")),
		Continue(),
	)
	g.Id(objVar).Op("=").Id(fmt.Sprintf(
		"latest%s",
		obj.Name,
	))
}

// operationCase generates the source code for each create, update and delete
// case in the operation switch statement.
func operationCase(
	h *jen.Group,
	op string,
	obj *gen.ReconciledObject,
	varObjectName string,
	modulePath string,
) {
	uppoerOp := strcase.ToCamel(op)
	lowerOpPast := op + "d"
	upperOpPast := strcase.ToCamel(lowerOpPast)

	h.Case(Qual(
		"github.com/threeport/threeport/pkg/notifications/v0",
		fmt.Sprintf("NotificationOperation%s", upperOpPast),
	)).BlockFunc(func(i *Group) {
		if op == "create" {
			h.If(Id(varObjectName).Dot("ScheduledForDeletion").Call().Op("!=").Nil()).Block(
				Id("log").Dot("Info").Call(
					Lit(fmt.Sprintf(
						"%s scheduled for deletion - skipping %s",
						strcase.ToDelimited(obj.Name, ' '),
						op,
					)),
				),
				Break(),
			)
		}
		h.Var().Id("operationErr").Error()
		h.Var().Id("customRequeueDelay").Int64()
		h.Switch(Id(varObjectName).Dot("GetVersion").Call()).BlockFunc(func(j *Group) {
			for _, version := range obj.Versions {
				j.Case(Lit(version)).Block(
					List(Id("requeueDelay"), Id("err")).Op(":=").Id(fmt.Sprintf(
						"%s%s%s",
						version,
						obj.Name,
						upperOpPast,
					)).Call(
						Line().Id("r"),
						Line().Id(varObjectName).Assert(Op("*").Qual(
							fmt.Sprintf("%s/pkg/api/%s", modulePath, version),
							obj.Name,
						)),
						Line().Op("&").Id("log"),
						Line(),
					),
					Id("customRequeueDelay").Op("=").Id("requeueDelay"),
					Id("operationErr").Op("=").Id("err"),
				)
			}
			j.Default().Block(
				Id("operationErr").Op("=").Qual(
					"errors",
					"New",
				).Call(Lit(fmt.Sprintf(
					"unrecognized version of %s encountered for creation",
					strcase.ToDelimited(obj.Name, ' '),
				))),
			)
		})
		h.If(Id("operationErr").Op("!=").Nil()).Block(
			Id("errorMsg").Op(":=").Lit(fmt.Sprintf(
				"failed to reconcile %s %s object",
				lowerOpPast,
				strcase.ToDelimited(obj.Name, ' '),
			)),
			Id("log").Dot("Error").Call(
				Id("operationErr"),
				Id("errorMsg"),
			),
			Id("r").Dot("EventsRecorder").Dot("HandleEventOverride").Call(
				Line().Op("&").Qual("github.com/threeport/threeport/pkg/api/v0", "Event").Values(Dict{
					Id("Reason"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
						Qual(
							"github.com/threeport/threeport/pkg/event/v0",
							fmt.Sprintf("ReasonFailed%s", uppoerOp),
						),
					),
					Id("Note"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(Id("errorMsg")),
					Id("Type"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
						Qual("github.com/threeport/threeport/pkg/event/v0", "TypeNormal"),
					),
				}),
				Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetId").Call(),
				Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetVersion").Call(),
				Line().Id(strcase.ToLowerCamel(obj.Name)).Dot("GetType").Call(),
				Line().Id("operationErr"),
				Line().Op("&").Id("log"),
				Line(),
			),
			Id("r").Dot("UnlockAndRequeue").Call(
				Line().Id(varObjectName),
				Line().Id("requeueDelay"),
				Line().Id("lockReleased"),
				Line().Id("msg"),
				Line(),
			),
			Continue(),
		)
		h.If(Id("customRequeueDelay").Op("!=").Lit(0)).Block(
			Id("log").Dot("Info").Call(
				Lit(fmt.Sprintf(
					"%s requeued for future reconciliation",
					op,
				)),
			),
			Id("r").Dot("UnlockAndRequeue").Call(
				Line().Id(varObjectName),
				Line().Id("customRequeueDelay"),
				Line().Id("lockReleased"),
				Line().Id("msg"),
				Line(),
			),
			Continue(),
		)
		if op == "delete" {
			h.Id("deletionTimestamp").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"Ptr",
			).Call(Qual("time", "Now").Call().Dot("UTC").Call())
			h.Id(fmt.Sprintf(
				"deleted%s",
				obj.Name,
			)).Op(":=").Qual(
				fmt.Sprintf("%s/pkg/api/v0", modulePath),
				obj.Name,
			).Values(Dict{
				Id("Common"): Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"Common",
				).Values(Dict{
					Id("ID"): Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"Ptr",
					).Call(Id(varObjectName).Dot("GetId").Call()),
				}),
				Id("Reconciliation"): Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"Reconciliation",
				).Values(Dict{
					Id("Reconciled"): Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"Ptr",
					).Call(Lit(true)),
					Id("DeletionAcknowledged"): Id("deletionTimestamp"),
					Id("DeletionConfirmed"):    Id("deletionTimestamp"),
				}),
			})
			h.Id("_").Op(",").Id("err").Op("=").Qual(
				fmt.Sprintf("%s/pkg/client/v0", modulePath),
				fmt.Sprintf("Update%s", obj.Name),
			).Call(
				Line().Id("r").Dot("APIClient"),
				Line().Id("r").Dot("APIServer"),
				Line().Op("&").Id(fmt.Sprintf(
					"deleted%s",
					obj.Name,
				)),
				Line(),
			)
			h.If(Id("err").Op("!=").Nil()).Block(
				Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
					"failed to update %s to mark as deleted",
					strcase.ToDelimited(obj.Name, ' '),
				))),
				Id("r").Dot("UnlockAndRequeue").Call(
					Id(varObjectName),
					Id("requeueDelay"),
					Id("lockReleased"),
					Id("msg"),
				),
				Continue(),
			)

			h.Id("_").Op(",").Id("err").Op("=").Qual(
				fmt.Sprintf("%s/pkg/client/v0", modulePath),
				fmt.Sprintf("Delete%s", obj.Name),
			).Call(
				Line().Id("r").Dot("APIClient"),
				Line().Id("r").Dot("APIServer"),
				Line().Id(varObjectName).Dot("GetId").Call(),
				Line(),
			)
			h.If(Id("err").Op("!=").Nil()).Block(
				Id("log").Dot("Error").Call(Id("err"), Lit(fmt.Sprintf(
					"failed to delete %s",
					strcase.ToDelimited(obj.Name, ' '),
				))),
				Id("r").Dot("UnlockAndRequeue").Call(
					Id(varObjectName),
					Id("requeueDelay"),
					Id("lockReleased"),
					Id("msg"),
				),
				Continue(),
			)
		}
	})
}
