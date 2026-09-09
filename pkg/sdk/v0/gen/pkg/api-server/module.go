package apiserver

import (
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenCoreModuleRegistration generates the code for registering the core module in the database.
func GenCoreModuleRegistration(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	f := NewFile("v0")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "api_v0")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/lib/v0", "apilib")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0/routes", "routes")

	f.Comment("RegisterModule registers the module information in the database.")
	f.Func().Id("RegisterModule").Params(
		Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
	).Error().Block(
		Comment("register ModuleApi object"),
		List(Id("moduleApi"), Id("err")).Op(":=").Id("upsertModuleApi").Call(Id("db")),
		If(Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit("failed to ensure ModuleApi object was present in database: %w"),
				Id("err"),
			)),
		),
		Line(),
		Comment("register ModuleController, ModuleObject, and ModuleApiRoute objects"),
		If(Id("err").Op(":=").Id("upsertModuleControllersObjectsRoutes").Call(
			Id("db"),
			Id("moduleApi"),
		).Op(";").Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit("failed to ensure ModuleController and ModuleObject objects were present in database: %w"),
				Id("err"),
			)),
		),
		Line(),
		Return(Nil()),
	)
	f.Line()

	// upsertModuleApi function
	f.Comment("upsertModuleApi creates or updates the module API object information in the database.")
	f.Func().Id("upsertModuleApi").Params(
		Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
	).Params(
		Op("*").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApi"),
		Error(),
	).Block(
		Id("apiEndpoint").Op(":=").Qual("os", "Getenv").Call(Lit("THREEPORT_API_ENDPOINT")),
		If(Id("apiEndpoint").Op("==").Lit("")).Block(
			Return(
				Nil(),
				Qual("fmt", "Errorf").Call(Lit("THREEPORT_API_ENDPOINT is not set in environment")),
			),
		),
		Line(),
		Id("moduleApi").Op(":=").Qual(
			"github.com/threeport/threeport/pkg/api/v0",
			"ModuleApi",
		).Values(Dict{
			Id("Name"): Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"Ptr",
			).Call(Qual(
				"github.com/threeport/threeport/pkg/api/lib/v0",
				"CoreModuleName",
			)),
			Id("Core"): Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"Ptr",
			).Call(Lit(true)),
			Id("ApiNamespace"): Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"Ptr",
			).Call(Qual(
				"github.com/threeport/threeport/pkg/api/lib/v0",
				"CoreApiNamespace",
			)),
			Id("Endpoint"): Qual(
				"github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(Id("apiEndpoint")),
		}),
		Line(),
		If(Id("result").Op(":=").Id("db").Dot("Where").Call(
			Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApi").Values(Dict{
				Id("Name"): Id("moduleApi").Dot("Name"),
			}),
		).Dot("FirstOrCreate").Call(Op("&").Id("moduleApi")).Op(";").Id("result").Dot("Error").Op("!=").Nil()).Block(
			Return(
				Nil(),
				Qual("fmt", "Errorf").Call(Lit("failed to save module API: %w"), Id("result").Dot("Error")),
			),
		),
		Line(),
		Return(Op("&").Id("moduleApi"), Nil()),
	)
	f.Line()

	// upsertModuleControllersObjectsRoutes function
	f.Comment("upsertModuleControllersObjectsRoutes creates or updates the module controllers, objects, and routes in the database.")
	f.Func().Id("upsertModuleControllersObjectsRoutes").Params(
		Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
		Id("moduleApi").Op("*").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApi"),
	).Params(
		Error(),
	).BlockFunc(func(g *Group) {
		g.Id("threeportNamespace").Op(":=").Qual("os", "Getenv").Call(Lit("THREEPORT_CONTROL_PLANE_NAMESPACE"))
		g.If(Id("threeportNamespace").Op("==").Lit("")).Block(
			Return(
				Qual("fmt", "Errorf").Call(Lit("THREEPORT_CONTROL_PLANE_NAMESPACE is not set in environment")),
			),
		)
		g.Line()
		g.Var().Id("controller").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleController")
		g.Var().Id("object").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleObject")
		g.Var().Id("route").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute")
		g.Var().Id("result").Op("*").Qual("gorm.io/gorm", "DB")
		g.Line()
		for _, objGroup := range gen.ApiObjectGroups {
			g.Comment("///////////////////////////////////////////////////////////////////////////////")
			g.Comment(fmt.Sprintf("registering controllers, objects and routes for %s object group", objGroup.ControllerDomain))
			g.Comment("///////////////////////////////////////////////////////////////////////////////")
			// register controllers for API groups with reconciled objects
			if len(objGroup.ReconciledObjects) > 0 {
				g.Comment(fmt.Sprintf("registering controller %s", objGroup.ControllerName))
				g.Id("controller").Op("=").Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"ModuleController",
				).Values(Dict{
					Id("Name"): Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"Ptr",
					).Call(Lit(objGroup.ControllerName)),
					Id("DeploymentName"): Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"Ptr",
					).Call(
						Id("threeportNamespace").Op("+").Lit(fmt.Sprintf("/threeport-%s", objGroup.ControllerName)),
					),
					Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
				})
				g.Id("result").Op("=").Id("db").Dot("Where").Call(
					Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleController").Values(Dict{
						Id("Name"): Id("controller").Dot("Name"),
					}),
				).Dot("FirstOrCreate").Call(Op("&").Id("controller"))
				g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
					Return(
						Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register %s: %%w",
								objGroup.ControllerName,
							)),
							Id("result").Dot("Error"),
						),
					),
				)
				g.Line()
				// register objects for API groups with reconciled objects
				for _, apiObj := range objGroup.ApiObjects {
					g.Comment(fmt.Sprintf("registering object %s", apiObj.TypeName))
					// include the controller ID for reconciled objects - omit controller ID for non-reconciled objects
					if apiObj.Reconciler {
						g.Id("object").Op("=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Dict{
							Id("Name"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.TypeName)),
							Id("Version"): Qual(
								"github.com/threeport/threeport/pkg/util/v0", "Ptr",
							).Call(Lit(apiObj.Version)),
							Id("Description"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.Description)),
							Id("ModuleApiID"):        Id("moduleApi").Dot("ID"),
							Id("ModuleControllerID"): Id("controller").Dot("ID"),
						})
					} else {
						g.Id("object").Op("=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Dict{
							Id("Name"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.TypeName)),
							Id("Version"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.Version)),
							Id("Description"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.Description)),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						})
					}
					g.Id("result").Op("=").Id("db").Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleObject").Values(Dict{
							Id("Name"):        Id("object").Dot("Name"),
							Id("Version"):     Id("object").Dot("Version"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("object"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Line()
					g.Comment(fmt.Sprintf("registering routes for %s", apiObj.TypeName))
					g.Id("route").Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Qual(
							fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
							fmt.Sprintf(
								"Path%sVersions",
								apiObj.TypeName,
							),
						)),
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id("object")),
					})
					g.Id("result").Op("=").Id("db").Dot("Omit").Call(
						Lit("ModuleObjects.*"),
					).Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
							Id("Path"):        Id("route").Dot("Path"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id("object")),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("route"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register version route for %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Id("route").Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Qual(
							fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
							fmt.Sprintf(
								"Path%s",
								pluralize.Pluralize(apiObj.TypeName, 2, false),
							),
						)),
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id("object")),
					})
					g.Id("result").Op("=").Id("db").Dot("Omit").Call(
						Lit("ModuleObjects.*"),
					).Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
							Id("Path"):        Id("route").Dot("Path"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id("object")),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("route"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register object route for %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Line()
				}
			} else {
				// register objects for API groups without reconciled objects
				for _, apiObj := range objGroup.ApiObjects {
					g.Comment(fmt.Sprintf("registering object %s", apiObj.TypeName))
					g.Id("object").Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleObject",
					).Values(Dict{
						Id("Name"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Lit(apiObj.TypeName)),
						Id("Version"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Lit(apiObj.Version)),
						Id("Description"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Lit(apiObj.Description)),
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
					})
					g.Id("result").Op("=").Id("db").Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleObject").Values(Dict{
							Id("Name"):        Id("object").Dot("Name"),
							Id("Version"):     Id("object").Dot("Version"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("object"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Line()
					g.Comment(fmt.Sprintf("registering routes for %s", apiObj.TypeName))
					g.Id("route").Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Qual(
							fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
							fmt.Sprintf(
								"Path%sVersions",
								apiObj.TypeName,
							),
						)),
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id("object")),
					})
					g.Id("result").Op("=").Id("db").Dot("Omit").Call(
						Lit("ModuleObjects.*"),
					).Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
							Id("Path"):        Id("route").Dot("Path"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id("object")),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("route"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register version route for %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Id("route").Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(Qual(
							fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
							fmt.Sprintf(
								"Path%s",
								pluralize.Pluralize(apiObj.TypeName, 2, false),
							),
						)),
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id("object")),
					})
					g.Id("result").Op("=").Id("db").Dot("Omit").Call(
						Lit("ModuleObjects.*"),
					).Dot("Where").Call(
						Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
							Id("Path"):        Id("route").Dot("Path"),
							Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id("object")),
						}),
					).Dot("FirstOrCreate").Call(Op("&").Id("route"))
					g.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to register object route for %s: %%w",
								apiObj.TypeName,
							)),
							Id("result").Dot("Error"),
						)),
					)
					g.Line()
				}
			}
		}
		g.Line()
		g.Comment("registering custom routes")
		g.For(List(Id("_"), Id("customRoute")).Op(":=").Range().Op("*").Qual(
			"github.com/threeport/threeport/pkg/api-server/v0/routes",
			"CustomRoutes",
		).Call(Nil())).BlockFunc(func(h *Group) {
			h.Comment("query the module objects for the custom route by name and version")
			h.Id("moduleObjects").Op(":=").Index().Op("*").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ModuleObject",
			).Values()
			h.For(List(Id("_"), Id("apiObject")).Op(":=").Range().Op("*").Id("customRoute").Dot("ApiObjects")).Block(
				Id("moduleObj").Op(":=").Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"ModuleObject",
				).Values(),
				Id("moduleResult").Op(":=").Id("db").Dot("Where").Call(
					Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleObject").Values(Dict{
						Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
						Id("Name"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
							Id("apiObject").Dot("Name"),
						),
						Id("Version"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
							Id("apiObject").Dot("Version"),
						),
					}),
				).Dot("Find").Call(Op("&").Id("moduleObj")),
				If(Id("moduleResult").Dot("Error").Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit("failed to query module object for %s: %w"),
						Id("apiObject").Dot("Name"),
						Id("moduleResult").Dot("Error"),
					)),
				),
				Id("moduleObjects").Op("=").Append(Id("moduleObjects"), Op("&").Id("moduleObj")),
			)
			h.Id("route").Op("=").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ModuleApiRoute",
			).Values(Dict{
				Id("ModuleApiID"): Id("moduleApi").Dot("ID"),
				Id("Path"): Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"Ptr",
				).Call(
					Id("customRoute").Dot("Path"),
				),
				Id("ModuleObjects"): Id("moduleObjects"),
			})
			h.Id("result").Op("=").Id("db").Dot("Omit").Call(
				Lit("ModuleObjects.*"),
			).Dot("Where").Call(
				Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
					Id("Path"):          Id("route").Dot("Path"),
					Id("ModuleApiID"):   Id("moduleApi").Dot("ID"),
					Id("ModuleObjects"): Id("moduleObjects"),
				}),
			).Dot("FirstOrCreate").Call(Op("&").Id("route"))
			h.If(Id("result").Dot("Error").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to register custom route for %s: %w"),
					Id("customRoute").Dot("Path"),
					Id("result").Dot("Error"),
				)),
			)
		})
		g.Line()
		g.Return(Nil())
	})
	f.Line()

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join(
		"pkg",
		"api-server",
		"v0",
		"module_gen.go",
	)
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for module registration written to %s", genFilepath))
	}

	return nil
}

// GenModuleRegistration generates the code for registering the extension module
// with the Threeport API server.
func GenModuleRegistration(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	f := NewFile("v0")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	// Add imports
	f.ImportAlias(fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath), "api_v0")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tp_api")
	f.ImportAlias("github.com/threeport/threeport/pkg/client/v0", "tp_client")
	f.ImportAlias("github.com/threeport/threeport/pkg/client/lib/v0", "tp_client_lib")
	f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "tp_util")
	f.ImportAlias(fmt.Sprintf("%s/pkg/api-server/v0/routes", gen.ModulePath), "routes")

	// Generate module name constant
	moduleName := fmt.Sprintf(
		"%s/%s-module-api",
		sdkConfig.ApiNamespace,
		strcase.ToKebab(sdkConfig.ModuleName),
	)
	f.Const().Defs(
		Id("moduleName").Op("=").Lit(moduleName),
	)
	f.Line()

	// emit the module API lookup helper so the registration entrypoint stays flat
	f.Comment("resolveModuleApi looks up the module API by name. It returns the record")
	f.Comment("when one is registered under that name, and otherwise reports that a fresh")
	f.Comment("record must be created.")
	f.Func().Id("resolveModuleApi").Params(
		Id("tpApiClient").Op("*").Qual("net/http", "Client"),
		Id("tpApiAddr").String(),
	).Params(
		Op("*").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApi"),
		Bool(),
		Error(),
	).Block(
		List(Id("existingModApi"), Id("err")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/client/v0",
			"GetModuleApiByName",
		).Call(
			Id("tpApiClient"),
			Id("tpApiAddr"),
			Id("moduleName"),
		),
		If(Id("err").Op("==").Nil()).Block(
			Qual("log", "Printf").Call(
				Lit("register-module: found existing module api %q"),
				Id("moduleName"),
			),
			Return(Id("existingModApi"), False(), Nil()),
		),
		If(Op("!").Qual("errors", "Is").Call(
			Id("err"),
			Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrObjectNotFound"),
		)).Block(
			Return(Nil(), False(), Qual("fmt", "Errorf").Call(
				Lit("failed to check for existing module API: %w"),
				Id("err"),
			)),
		),
		Line(),
		Return(Nil(), True(), Nil()),
	)
	f.Line()

	// emit the controller create-then-rebind helper
	f.Comment("upsertModuleController creates the module controller. When the create")
	f.Comment("hits a name conflict, it looks up the existing row by name alone and, if")
	f.Comment("that row points at a different module_api_id, updates it to the current")
	f.Comment("one so the module claims the controller instead of colliding with it.")
	f.Func().Id("upsertModuleController").Params(
		Id("tpApiClient").Op("*").Qual("net/http", "Client"),
		Id("tpApiAddr").String(),
		Id("controller").Op("*").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleController"),
		Id("moduleApiID").Op("*").Uint(),
	).Params(
		Op("*").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleController"),
		Error(),
	).Block(
		List(Id("created"), Id("createErr")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/client/v0",
			"CreateModuleController",
		).Call(
			Id("tpApiClient"),
			Id("tpApiAddr"),
			Id("controller"),
		),
		If(Id("createErr").Op("==").Nil()).Block(
			Return(Id("created"), Nil()),
		),
		If(Op("!").Qual("errors", "Is").Call(
			Id("createErr"),
			Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
		)).Block(
			Return(Nil(), Id("createErr")),
		),
		Line(),
		Qual("log", "Printf").Call(
			Lit("register-module: controller %q create hit conflict; checking which module api owns it"),
			Op("*").Id("controller").Dot("Name"),
		),
		List(Id("existing"), Id("lookupErr")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/client/v0",
			"GetModuleControllerByName",
		).Call(
			Id("tpApiClient"),
			Id("tpApiAddr"),
			Op("*").Id("controller").Dot("Name"),
		),
		If(Id("lookupErr").Op("!=").Nil()).Block(
			Return(Nil(), Qual("fmt", "Errorf").Call(
				Lit("failed to look up conflicting controller %q by name: %w"),
				Op("*").Id("controller").Dot("Name"),
				Id("lookupErr"),
			)),
		),
		If(Id("existing").Dot("ModuleApiID").Op("!=").Nil().Op("&&").
			Op("*").Id("existing").Dot("ModuleApiID").Op("==").Op("*").Id("moduleApiID")).Block(
			Qual("log", "Printf").Call(
				Lit("register-module: controller %q already bound to current module api %d"),
				Op("*").Id("controller").Dot("Name"),
				Op("*").Id("moduleApiID"),
			),
			Return(Id("existing"), Nil()),
		),
		// render the prior module API id as text, none when unset
		Id("priorModuleApi").Op(":=").Lit("none"),
		If(Id("existing").Dot("ModuleApiID").Op("!=").Nil()).Block(
			Id("priorModuleApi").Op("=").Qual("fmt", "Sprintf").Call(
				Lit("%d"),
				Op("*").Id("existing").Dot("ModuleApiID"),
			),
		),
		Qual("log", "Printf").Call(
			Lit("register-module: rebinding controller %q from module api %s to %d"),
			Op("*").Id("controller").Dot("Name"),
			Id("priorModuleApi"),
			Op("*").Id("moduleApiID"),
		),
		Id("existing").Dot("ModuleApiID").Op("=").Id("moduleApiID"),
		Id("existing").Dot("DeploymentName").Op("=").Id("controller").Dot("DeploymentName"),
		List(Id("updated"), Id("upErr")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/client/v0",
			"UpdateModuleController",
		).Call(
			Id("tpApiClient"),
			Id("tpApiAddr"),
			Id("existing"),
		),
		If(Id("upErr").Op("!=").Nil()).Block(
			Return(Nil(), Qual("fmt", "Errorf").Call(
				Lit("failed to rebind controller %q to current module api: %w"),
				Op("*").Id("controller").Dot("Name"),
				Id("upErr"),
			)),
		),
		Return(Id("updated"), Nil()),
	)
	f.Line()

	// Generate RegisterModule function
	f.Comment("RegisterModule calls the Threeport API to register the module with core Threeport.")
	f.Comment("This ensures that module object requests are proxied to the module API by the")
	f.Comment("Threeport API server.")
	f.Func().Id("RegisterModule").Params(
		Line().Id("tpApiClient").Op("*").Qual("net/http", "Client"),
		Line().Id("tpApiAddr").String(),
		Line().Id("moduleApiEndpoint").String(),
		Line().Id("moduleNamespace").String(),
		Line(),
	).Error().BlockFunc(func(g *Group) {
		g.Comment("look up the module API so a re-registration reuses the existing")
		g.Comment("record along with the controller and object rows that reference it")
		g.List(Id("existingModApi"), Id("createFresh"), Id("err")).Op(":=").Id("resolveModuleApi").Call(
			Id("tpApiClient"),
			Id("tpApiAddr"),
		)
		g.If(Id("err").Op("!=").Nil()).Block(
			Return(Id("err")),
		)
		g.If(Id("createFresh")).Block(
			Comment("register the module in the Threeport API with the ModuleApi object"),
			Qual("log", "Printf").Call(
				Lit("register-module: no existing module api; creating %q"),
				Id("moduleName"),
			),
			Id("moduleApi").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ModuleApi",
			).Values(Dict{
				Id("Endpoint"): Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"Ptr",
				).Call(Id("moduleApiEndpoint")),
				Id("Name"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(Id("moduleName")),
				Id("ApiNamespace"): Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"Ptr",
				).Call(Qual(
					fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
					"ApiNamespace",
				)),
			}),
			List(Id("createdModApi"), Id("createErr")).Op(":=").Qual(
				"github.com/threeport/threeport/pkg/client/v0",
				"CreateModuleApi",
			).Call(
				Id("tpApiClient"),
				Id("tpApiAddr"),
				Op("&").Id("moduleApi"),
			),
			If(Id("createErr").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to create module API object in Threeport API: %w"),
					Id("createErr"),
				)),
			),
			Id("existingModApi").Op("=").Id("createdModApi"),
		)

		for _, objGroup := range gen.ApiObjectGroups {
			g.Comment("///////////////////////////////////////////////////////////////////////////////")
			g.Comment(fmt.Sprintf("registering controllers, objects and routes for %s object group", objGroup.ControllerDomain))
			g.Comment("///////////////////////////////////////////////////////////////////////////////")
			controllerVar := strcase.ToLowerCamel(objGroup.ControllerName)
			controllerSliceVar := fmt.Sprintf("%sSlice", controllerVar)
			controllerErrVar := fmt.Sprintf("%sErr", controllerVar)
			if len(objGroup.ReconciledObjects) > 0 {
				g.Comment(fmt.Sprintf("registering controller %s", objGroup.ControllerName))
				g.List(Id(controllerSliceVar), Id(controllerErrVar)).Op(":=").Qual(
					"github.com/threeport/threeport/pkg/client/v0",
					"GetModuleControllersByQueryString",
				).Call(
					Line().Id("tpApiClient"),
					Line().Id("tpApiAddr"),
					Line().Qual("fmt", "Sprintf").Call(
						Lit("name=%s&moduleapiid=%d"),
						Lit(objGroup.ControllerName),
						Op("*").Id("existingModApi").Dot("ID"),
					),
					Line(),
				)
				g.If(Id(controllerErrVar).Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf("failed to check for existing controller %s: %%w", objGroup.ControllerName)),
						Id(controllerErrVar),
					)),
				)
				g.Var().Id(controllerVar).Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					"ModuleController",
				)
				g.If(Len(Op("*").Id(controllerSliceVar)).Op("==").Lit(0)).Block(
					Comment("controller doesn't exist - create it"),
					Id("controller").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleController",
					).Values(Dict{
						Id("DeploymentName"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(
							Id("moduleNamespace").Op("+").Lit(fmt.Sprintf("/threeport-%s", objGroup.ControllerName)),
						),
						Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
						Id("Name"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(
							Lit(objGroup.ControllerName),
						),
					}),
					Comment("the lookup above is scoped to this module api, so a row"),
					Comment("under a different one is invisible here and only shows up"),
					Comment("as a name conflict on the create"),
					List(Id("createdController"), Id("err")).Op(":=").Id("upsertModuleController").Call(
						Id("tpApiClient"),
						Id("tpApiAddr"),
						Op("&").Id("controller"),
						Id("existingModApi").Dot("ID"),
					),
					If(Id("err").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf("failed to register module controller %s in Threeport API: %%w", objGroup.ControllerName)),
							Id("err"),
						)),
					),
					Id(controllerVar).Op("=").Op("*").Id("createdController"),
					Line(),
				).Else().If(Len(Op("*").Id(controllerSliceVar)).Op("==").Lit(1)).Block(
					Id(controllerVar).Op("=").Parens(Op("*").Id(controllerSliceVar)).Index(Lit(0)),
				).Else().Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf("expected 0 or 1 module controllers for %s, got %%d", objGroup.ControllerName)),
						Len(Op("*").Id(controllerSliceVar)),
					)),
				)
				g.Line()
				for _, apiObj := range objGroup.ApiObjects {
					objectVar := strcase.ToLowerCamel(apiObj.TypeName)
					objectSliceVar := fmt.Sprintf("%sSlice", objectVar)
					objectErrVar := fmt.Sprintf("%sErr", objectVar)
					routeVar := fmt.Sprintf("%sRoute", objectVar)
					routeErrVar := fmt.Sprintf("%sRouteErr", objectVar)
					if apiObj.Reconciler {
						g.Comment(fmt.Sprintf("registering object %s", apiObj.TypeName))
						g.Var().Id(objectVar).Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						)
						g.List(Id(objectSliceVar), Id(objectErrVar)).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"GetModuleObjectsByQueryString",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Qual("fmt", "Sprintf").Call(
								Lit(fmt.Sprintf("name=%s&moduleapiid=%%d", apiObj.TypeName)),
								Op("*").Id("existingModApi").Dot("ID"),
							),
							Line(),
						)
						g.If(Id(objectErrVar).Op("!=").Nil()).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to check for existing object %s: %%w", apiObj.TypeName)),
								Id(objectErrVar),
							)),
						)
						g.If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(0)).Block(
							Id("object").Op(":=").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Dict{
								Id("Name"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.TypeName)),
								Id("Version"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.Version)),
								Id("Description"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.Description)),
								Id("ModuleApiID"):        Id("existingModApi").Dot("ID"),
								Id("ModuleControllerID"): Id(controllerVar).Dot("ID"),
							}),
							List(Id("objectResult"), Id("err")).Op(":=").Qual(
								"github.com/threeport/threeport/pkg/client/v0",
								"CreateModuleObject",
							).Call(
								Id("tpApiClient"),
								Id("tpApiAddr"),
								Op("&").Id("object"),
							),
							If(Id("err").Op("!=").Nil()).Block(
								Return(Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to create module object %s in Threeport API: %%w", apiObj.TypeName)),
									Id("err"),
								)),
							),
							Id(objectVar).Op("=").Op("*").Id("objectResult"),
						).Else().If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(1)).Block(
							Id(objectVar).Op("=").Parens(Op("*").Id(objectSliceVar)).Index(Lit(0)),
						).Else().Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("expected 0 or 1 module objects for %s, got %%d", apiObj.TypeName)),
								Len(Op("*").Id(objectSliceVar)),
							)),
						)
						g.Line()
						g.Comment(fmt.Sprintf("registering routes for %s", apiObj.TypeName))
						g.Id(routeVar).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleApiRoute",
						).Values(Dict{
							Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id(objectVar)),
							Id("Path"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(
								Qual(
									fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
									fmt.Sprintf("Path%sVersions", apiObj.TypeName),
								),
							),
						})
						g.List(Id("_"), Id(routeErrVar)).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"CreateModuleApiRouteWithModuleObjectReferences",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Op("&").Id(routeVar).Op(",").Line(),
						)
						g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
							Id(routeErrVar),
							Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
						)).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to create module route for %s version in Threeport API: %%w", apiObj.TypeName)),
								Id(routeErrVar),
							)),
						)
						g.Id(routeVar).Op("=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleApiRoute",
						).Values(Dict{
							Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id(objectVar)),
							Id("Path"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(
								Qual(
									fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
									fmt.Sprintf("Path%s", pluralize.Pluralize(apiObj.TypeName, 2, false)),
								),
							),
						})
						g.List(Id("_"), Id(routeErrVar)).Op("=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"CreateModuleApiRouteWithModuleObjectReferences",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Op("&").Id(routeVar).Op(",").Line(),
						)
						g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
							Id(routeErrVar),
							Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
						)).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to create module route for %s in Threeport API: %%w", apiObj.TypeName)),
								Id(routeErrVar),
							)),
						)
						g.Line()
					} else {
						g.Comment(fmt.Sprintf("registering object %s", apiObj.TypeName))
						g.Var().Id(objectVar).Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						)
						g.List(Id(objectSliceVar), Id(objectErrVar)).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"GetModuleObjectsByQueryString",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Qual("fmt", "Sprintf").Call(
								Lit(fmt.Sprintf("name=%s&moduleapiid=%%d", apiObj.TypeName)),
								Op("*").Id("existingModApi").Dot("ID"),
							),
							Line(),
						)
						g.If(Id(objectErrVar).Op("!=").Nil()).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to check for existing object %s: %%w", apiObj.TypeName)),
								Id(objectErrVar),
							)),
						)
						g.If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(0)).Block(
							Id("object").Op(":=").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Dict{
								Id("Name"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.TypeName)),
								Id("Version"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.Version)),
								Id("Description"): Qual(
									"github.com/threeport/threeport/pkg/util/v0",
									"Ptr",
								).Call(Lit(apiObj.Description)),
								Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
							}),
							List(Id("objectResult"), Id("err")).Op(":=").Qual(
								"github.com/threeport/threeport/pkg/client/v0", "CreateModuleObject",
							).Call(
								Id("tpApiClient"),
								Id("tpApiAddr"),
								Op("&").Id("object"),
							),
							If(Id("err").Op("!=").Nil()).Block(
								Return(Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to create module object %s in Threeport API: %%w", apiObj.TypeName)),
									Id("err"),
								)),
							),
							Id(objectVar).Op("=").Op("*").Id("objectResult"),
						).Else().If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(1)).Block(
							Id(objectVar).Op("=").Parens(Op("*").Id(objectSliceVar)).Index(Lit(0)),
						).Else().Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("expected 0 or 1 module objects for %s, got %%d", apiObj.TypeName)),
								Len(Op("*").Id(objectSliceVar)),
							)),
						)
						g.Line()
						g.Comment(fmt.Sprintf("registering routes for %s", apiObj.TypeName))
						g.Id(routeVar).Op(":=").Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleApiRoute").Values(Dict{
							Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id(objectVar)),
							Id("Path"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(
								Qual(fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath), fmt.Sprintf("Path%sVersions", apiObj.TypeName)),
							),
						})
						g.List(Id("_"), Id(routeErrVar)).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"CreateModuleApiRouteWithModuleObjectReferences",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Op("&").Id(routeVar).Op(",").Line(),
						)
						g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
							Id(routeErrVar),
							Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
						)).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to create module route for %s version in Threeport API: %%w", apiObj.TypeName)),
								Id(routeErrVar),
							)),
						)
						g.Id(routeVar).Op("=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleApiRoute",
						).Values(Dict{
							Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
							Id("ModuleObjects"): Index().Op("*").Qual(
								"github.com/threeport/threeport/pkg/api/v0",
								"ModuleObject",
							).Values(Op("&").Id(objectVar)),
							Id("Path"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(
								Qual(
									fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
									fmt.Sprintf("Path%s", pluralize.Pluralize(apiObj.TypeName, 2, false)),
								),
							),
						})
						g.List(Id("_"), Id(routeErrVar)).Op("=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"CreateModuleApiRouteWithModuleObjectReferences",
						).Call(
							Line().Id("tpApiClient"),
							Line().Id("tpApiAddr"),
							Line().Op("&").Id(routeVar).Op(",").Line(),
						)
						g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
							Id(routeErrVar),
							Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
						)).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to create module route for %s in Threeport API: %%w", apiObj.TypeName)),
								Id(routeErrVar),
							)),
						)
						g.Line()
					}
				}
			} else {
				for _, apiObj := range objGroup.ApiObjects {
					objectVar := strcase.ToLowerCamel(apiObj.TypeName)
					objectSliceVar := fmt.Sprintf("%sSlice", objectVar)
					objectErrVar := fmt.Sprintf("%sErr", objectVar)
					routeVar := fmt.Sprintf("%sRoute", objectVar)
					routeErrVar := fmt.Sprintf("%sRouteErr", objectVar)
					g.Comment(fmt.Sprintf("registering object %s", apiObj.TypeName))
					g.Var().Id(objectVar).Qual("github.com/threeport/threeport/pkg/api/v0", "ModuleObject")
					g.List(Id(objectSliceVar), Id(objectErrVar)).Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/v0",
						"GetModuleObjectsByQueryString",
					).Call(
						Line().Id("tpApiClient"),
						Line().Id("tpApiAddr"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit(fmt.Sprintf("name=%s&moduleapiid=%%d", apiObj.TypeName)),
							Op("*").Id("existingModApi").Dot("ID"),
						),
						Line(),
					)
					g.If(Id(objectErrVar).Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf("failed to check for existing object %s: %%w", apiObj.TypeName)),
							Id(objectErrVar),
						)),
					)
					g.If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(0)).Block(
						Id("object").Op(":=").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Dict{
							Id("Name"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.TypeName)),
							Id("Version"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.Version)),
							Id("Description"): Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"Ptr",
							).Call(Lit(apiObj.Description)),
							Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
						}),
						List(Id("objectResult"), Id("err")).Op(":=").Qual(
							"github.com/threeport/threeport/pkg/client/v0",
							"CreateModuleObject",
						).Call(
							Id("tpApiClient"),
							Id("tpApiAddr"),
							Op("&").Id("object"),
						),
						If(Id("err").Op("!=").Nil()).Block(
							Return(Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to create module object %s in Threeport API: %%w", apiObj.TypeName)),
								Id("err"),
							)),
						),
						Id(objectVar).Op("=").Op("*").Id("objectResult"),
					).Else().If(Len(Op("*").Id(objectSliceVar)).Op("==").Lit(1)).Block(
						Id(objectVar).Op("=").Parens(Op("*").Id(objectSliceVar)).Index(Lit(0)),
					).Else().Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf("expected 0 or 1 module objects for %s, got %%d", apiObj.TypeName)),
							Len(Op("*").Id(objectSliceVar)),
						)),
					)
					g.Line()
					g.Comment(fmt.Sprintf("registering routes for %s", apiObj.TypeName))
					g.Id(routeVar).Op(":=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id(objectVar)),
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(
							Qual(
								fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
								fmt.Sprintf("Path%sVersions", apiObj.TypeName),
							),
						),
					})
					g.List(Id("_"), Id(routeErrVar)).Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/v0",
						"CreateModuleApiRouteWithModuleObjectReferences",
					).Call(
						Line().Id("tpApiClient"),
						Line().Id("tpApiAddr"),
						Line().Op("&").Id(routeVar).Op(",").Line(),
					)
					g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
						Id(routeErrVar),
						Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
					)).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf("failed to create module route for %s version in Threeport API: %%w", apiObj.TypeName)),
							Id(routeErrVar),
						)),
					)
					g.Id(routeVar).Op("=").Qual(
						"github.com/threeport/threeport/pkg/api/v0",
						"ModuleApiRoute",
					).Values(Dict{
						Id("ModuleApiID"): Id("existingModApi").Dot("ID"),
						Id("ModuleObjects"): Index().Op("*").Qual(
							"github.com/threeport/threeport/pkg/api/v0",
							"ModuleObject",
						).Values(Op("&").Id(objectVar)),
						Id("Path"): Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Ptr",
						).Call(
							Qual(
								fmt.Sprintf("%s/pkg/api/v0", gen.ModulePath),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObj.TypeName, 2, false)),
							),
						),
					})
					g.List(Id("_"), Id(routeErrVar)).Op("=").Qual(
						"github.com/threeport/threeport/pkg/client/v0",
						"CreateModuleApiRouteWithModuleObjectReferences",
					).Call(
						Line().Id("tpApiClient"),
						Line().Id("tpApiAddr"),
						Line().Op("&").Id(routeVar).Op(",").Line(),
					)
					g.If(Id(routeErrVar).Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
						Id(routeErrVar),
						Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
					)).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf("failed to create module route for %s in Threeport API: %%w", apiObj.TypeName)),
							Id(routeErrVar),
						)),
					)
					g.Line()
				}
			}
		}
		g.Line()
		g.Comment("registering custom routes")
		g.For(List(Id("_"), Id("customRoute")).Op(":=").Range().Op("*").Qual(
			fmt.Sprintf("%s/pkg/api-server/v0/routes", gen.ModulePath),
			"CustomRoutes",
		).Call(Nil())).BlockFunc(func(h *Group) {
			h.Comment("query the module objects for the custom route by name and version")
			h.Id("moduleObjects").Op(":=").Index().Op("*").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ModuleObject",
			).Values()
			h.For(List(Id("_"), Id("apiObject")).Op(":=").Range().Op("*").Id("customRoute").Dot("ApiObjects")).Block(
				List(Id("modObj"), Id("err")).Op(":=").Qual(
					"github.com/threeport/threeport/pkg/client/v0",
					"GetModuleObjectsByQueryString",
				).Call(
					Line().Id("tpApiClient"),
					Line().Id("tpApiAddr"),
					Line().Qual("fmt", "Sprintf").Call(
						Lit("name=%s&version=%s"),
						Id("apiObject").Dot("Name"),
						Id("apiObject").Dot("Version"),
					),
					Line(),
				),
				If(Id("err").Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit("failed to retrieve module object %s: %w"),
						Id("apiObject").Dot("Name"),
						Id("err"),
					)),
				),
				Id("moduleObjects").Op("=").Append(Id("moduleObjects"), Op("&").Parens(Op("*").Id("modObj")).Index(Lit(0))),
			)
			h.Id("route").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ModuleApiRoute",
			).Values(Dict{
				Id("ModuleApiID"):   Id("existingModApi").Dot("ID"),
				Id("ModuleObjects"): Id("moduleObjects"),
				Id("Path"): Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"Ptr",
				).Call(
					Id("customRoute").Dot("Path"),
				),
			})
			h.List(Id("_"), Id("customRouteErr")).Op(":=").Qual(
				"github.com/threeport/threeport/pkg/client/v0",
				"CreateModuleApiRouteWithModuleObjectReferences",
			).Call(
				Line().Id("tpApiClient"),
				Line().Id("tpApiAddr"),
				Line().Op("&").Id("route"),
				Line(),
			)
			h.If(Id("customRouteErr").Op("!=").Nil().Op("&&").Op("!").Qual("errors", "Is").Call(
				Id("customRouteErr"),
				Qual("github.com/threeport/threeport/pkg/client/lib/v0", "ErrConflict"),
			)).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to create module route for custom route %s: %w"),
					Id("customRoute").Dot("Path"),
					Id("customRouteErr"),
				)),
			)
		})
		g.Line()
		g.Return(Nil())
	})

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join(
		"pkg",
		"api-server",
		"v0",
		"module_gen.go",
	)
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for module registration written to %s", genFilepath))
	}

	return nil
}
