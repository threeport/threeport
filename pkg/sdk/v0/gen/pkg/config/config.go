package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenConfig generates the config package that processes CLI user configs.
func GenConfig(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()
	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			for _, apiObject := range objGroup.ApiObjects {
				// create defined instance config abstraction if exists and if tptctl commands are enabled
				if apiObject.DefinedInstanceDefinition && apiObject.TptctlCommands {
					defInstObject := strings.TrimSuffix(apiObject.TypeName, "Definition")
					defInstConfigObjectName := fmt.Sprintf("%sConfig", defInstObject)
					defInstValuesObjectName := fmt.Sprintf("%sValues", defInstObject)
					defInstMethodVar := strings.ToLower(defInstObject[0:1])
					defInstObjectHuman := strcase.ToDelimited(defInstObject, ' ')

					defObject := apiObject.TypeName
					instObject := fmt.Sprintf("%sInstance", defInstObject)
					defVar := strcase.ToLowerCamel(defObject)
					instVar := strcase.ToLowerCamel(instObject)
					defsVar := pluralize.Pluralize(defVar, 2, false)
					instsVar := pluralize.Pluralize(instVar, 2, false)
					defValuesObjectName := fmt.Sprintf("%sValues", defObject)
					instValuesObjectName := fmt.Sprintf("%sValues", instObject)
					defConfigObjectName := fmt.Sprintf("%sConfig", defObject)
					instConfigObjectName := fmt.Sprintf("%sConfig", instObject)
					defConfigVar := strcase.ToLowerCamel(defConfigObjectName)
					instConfigVar := strcase.ToLowerCamel(instConfigObjectName)
					defInstValuesVar := strcase.ToLowerCamel(defInstValuesObjectName)
					operatedDefsVar := pluralize.Pluralize(fmt.Sprintf("operated%s", defObject), 2, false)
					operatedInstsVar := pluralize.Pluralize(fmt.Sprintf("operated%s", instObject), 2, false)
					mapToDefInstsFunc := fmt.Sprintf("mapTo%sDefinedInstances", defInstObject)

					f := NewFile(objCollection.Version)
					f.HeaderComment(sdk.HeaderCommentGenMod)

					// set import paths
					apiImportPath := fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", objCollection.Version)
					clientImportPath := fmt.Sprintf("github.com/threeport/threeport/pkg/client/%s", objCollection.Version)
					if gen.Module {
						apiImportPath = fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version)
						clientImportPath = fmt.Sprintf("%s/pkg/client/%s", gen.ModulePath, objCollection.Version)
					}

					f.ImportAlias(apiImportPath, fmt.Sprintf("api_%s", objCollection.Version))
					f.ImportAlias(clientImportPath, fmt.Sprintf("client_%s", objCollection.Version))
					f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
					if gen.Module {
						f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi_v0")
					}

					f.Commentf(
						"%s is a container for a %s which is a config abstraction for",
						defInstConfigObjectName,
						defInstObject,
					)
					f.Commentf(
						"the %s and %s API objects.",
						defObject,
						instObject,
					)
					f.Comment("This abstraction allows users to manage definitions and instances together with single operations")
					f.Comment("rather than separate operations for each API object.")
					f.Type().Id(defInstConfigObjectName).Struct(
						Id(defInstObject).Id(defInstValuesObjectName),
					)
					f.Line()

					f.Commentf(
						"%s contains all the attributes needed to manage the",
						defInstValuesObjectName,
					)
					f.Commentf(
						"%s and %s API objects",
						defObject,
						instObject,
					)
					f.Comment("together with a single operation.")
					f.Type().Id(defInstValuesObjectName).Struct(
						Commentf(
							"TODO: add fields needed for user to manage a %s and %s together",
							defObject,
							instObject,
						),
						Id("Name").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
						Id("Age").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
					)
					f.Line()

					// Get method
					f.Comment(fmt.Sprintf(
						"Get gets a %s definition and instance from the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstConfigObjectName)).Id("Get").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Index().Id(defInstConfigObjectName),
						Error(),
					).Block(
						Comment("get operations"),
						List(
							Id("operations"),
							Id(defsVar),
							Id(instsVar),
						).Op(":=").Id(defInstMethodVar).Dot("GetOperations").Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line(),
						),
						Line(),

						Comment("execute get operations"),
						If(Err().Op(":=").Id("operations").Dot("Get").Call(), Err().Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute get operations for %s defined instances: %%w",
									defInstObjectHuman,
								)),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Comment("assemble the defined instances"),
						Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))).Op(":=").Id(mapToDefInstsFunc).Call(
							Id(defsVar),
							Id(instsVar),
						),
						Line(),

						Return(
							Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))),
							Nil(),
						),
					)
					f.Line()

					// Create method
					f.Comment(fmt.Sprintf(
						"Create creates a %s definition and instance in the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstConfigObjectName)).Id("Create").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Index().Id(defInstConfigObjectName),
						Error(),
					).Block(
						Comment("get operations"),
						List(
							Id("operations"),
							Id(defsVar),
							Id(instsVar),
						).Op(":=").Id(defInstMethodVar).Dot("GetOperations").Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line(),
						),
						Line(),

						Comment("execute create operations"),
						If(Err().Op(":=").Id("operations").Dot("Create").Call(), Err().Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute create operations for %s defined instance with name %%s: %%w",
									defInstObjectHuman,
								)),
								Line().Op("*").Id(defInstMethodVar).Dot(defInstObject).Dot("Name"),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Comment("assemble the defined instances"),
						Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))).Op(":=").Id(mapToDefInstsFunc).Call(
							Id(defsVar),
							Id(instsVar),
						),
						Line(),

						Return(
							Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))),
							Nil(),
						),
					)
					f.Line()

					// Replace method
					f.Comment(fmt.Sprintf(
						"Replace replaces a %s definition and instance in the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstConfigObjectName)).Id("Replace").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line().Id("name").String(),
						Line(),
					).Params(
						Op("*").Index().Id(defInstConfigObjectName),
						Error(),
					).Block(
						Comment("get operations"),
						List(
							Id("operations"),
							Id(defsVar),
							Id(instsVar),
						).Op(":=").Id(defInstMethodVar).Dot("GetOperations").Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line(),
						),
						Line(),

						Comment("execute replace operations"),
						If(Err().Op(":=").Id("operations").Dot("Replace").Call(Id("name")), Err().Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute replace operations for %s defined instance with name %%s: %%w",
									defInstObjectHuman,
								)),
								Line().Id("name"),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Comment("assemble the defined instances"),
						Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))).Op(":=").Id(mapToDefInstsFunc).Call(
							Id(defsVar),
							Id(instsVar),
						),
						Line(),

						Return(
							Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))),
							Nil(),
						),
					)
					f.Line()

					// Delete method
					f.Comment(fmt.Sprintf(
						"Delete deletes a %s definition and instance from the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstConfigObjectName)).Id("Delete").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Index().Id(defInstConfigObjectName),
						Error(),
					).Block(
						Comment("get operations"),
						List(
							Id("operations"),
							Id("_"),
							Id("_"),
						).Op(":=").Id(defInstMethodVar).Dot("GetOperations").Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line(),
						),
						Line(),

						Comment("execute delete operations"),
						If(Err().Op(":=").Id("operations").Dot("Delete").Call(), Err().Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute delete operations for %s defined instance with name %%s: %%w",
									defInstObjectHuman,
								)),
								Line().Op("*").Id(defInstMethodVar).Dot(defInstObject).Dot("Name"),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Return(Nil(), Nil()),
					)
					f.Line()

					// GetOperations method
					f.Comment("GetOperations returns a slice of operations used to get, create, replace or delete")
					f.Comment(fmt.Sprintf("a %s defined instance.", defInstObjectHuman))
					f.Func().Params(
						Id(defInstMethodVar).Op("*").Id(defInstConfigObjectName),
					).Id("GetOperations").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Qual("github.com/threeport/threeport/pkg/util/v0", "Operations"),
						Op("*").Index().Id(fmt.Sprintf("%sConfig", defObject)),
						Op("*").Index().Id(fmt.Sprintf("%sConfig", instObject)),
					).Block(
						Id(defInstValuesVar).Op(":=").Id(defInstMethodVar).Dot(defInstObject),
						Var().Id("err").Error(),
						Var().Id(operatedDefsVar).Index().Id(fmt.Sprintf("%sConfig", defObject)),
						Var().Id(operatedInstsVar).Index().Id(fmt.Sprintf("%sConfig", instObject)),
						Line(),

						Id("operations").Op(":=").Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operations",
						).Values(),
						Line(),

						Commentf("add %s definition operation", defInstObjectHuman),
						Comment("TODO: add appropriate fields to definition values object"),
						Id(defConfigVar).Op(":=").Id(defConfigObjectName).Values(Dict{
							Line().Id(defObject): Id(defValuesObjectName).Values(
								Dict{
									Id("Name"): Id(defInstValuesVar).Dot("Name"),
									Id("Age"):  Id(defInstValuesVar).Dot("Age"),
								},
							).Op(",").Line(),
						}),
						Id("operations").Dot("AppendOperation").Call(Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operation",
						).Values(
							Dict{
								Id("Get"): Func().Params().Error().Block(
									List(
										Id(defsVar),
										Id("err"),
									).Op(":=").Id(defConfigVar).Dot("Get").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to get %s definitions: %%w",
												defInstObjectHuman,
											)),
											Id("err"),
										)),
									),
									Id(operatedDefsVar).Op("=").Id("append").Call(
										Id(operatedDefsVar),
										Op("*").Id(defsVar).Op("..."),
									),
									Return(Nil()),
								),
								Id("Create"): Func().Params().Error().Block(
									List(
										Id(defVar),
										Id("err"),
									).Op(":=").Id(defConfigVar).Dot("Create").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to create %s definition with name %%s: %%w",
												defInstObjectHuman,
											)),
											Op("*").Id(defInstValuesVar).Dot("Name"),
											Id("err"),
										)),
									),
									Id(operatedDefsVar).Op("=").Id("append").Call(
										Id(operatedDefsVar),
										Op("*").Id(defVar),
									),
									Return(Nil()),
								),
								Id("Replace"): Func().Params(Id("name").String()).Error().Block(
									List(
										Id(defVar),
										Id("err"),
									).Op(":=").Id(defConfigVar).Dot("Replace").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
										Id("name"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to replace %s definition with name %%s: %%w",
												defInstObjectHuman,
											)),
											Id("name"),
											Id("err"),
										)),
									),
									Id(operatedDefsVar).Op("=").Id("append").Call(
										Id(operatedDefsVar),
										Op("*").Id(defVar),
									),
									Return(Nil()),
								),
								Id("Delete"): Func().Params().Error().Block(
									List(
										Op("_"),
										Id("err"),
									).Op("=").Id(defConfigVar).Dot("Delete").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(
											Qual("fmt", "Errorf").Call(
												Lit(fmt.Sprintf(
													"failed to delete %s definition with name %%s: %%w",
													defInstObjectHuman,
												)),
												Op("*").Id(defInstValuesVar).Dot("Name"),
												Id("err"),
											),
										),
									),
									Return(Nil()),
								),
								Id("Name"): Lit(fmt.Sprintf("%s definition", defInstObjectHuman)),
							},
						)),
						Line(),

						Commentf("add %s instance operation", defInstObjectHuman),
						Comment("TODO: add appropriate fields to instance values object"),
						Id(instConfigVar).Op(":=").Id(instConfigObjectName).Values(Dict{
							Line().Id(instObject): Id(instValuesObjectName).Values(
								Dict{
									Id("Name"): Id(defInstValuesVar).Dot("Name"),
									Id("Age"):  Id(defInstValuesVar).Dot("Age"),
								},
							).Op(",").Line(),
						}),
						Id("operations").Dot("AppendOperation").Call(Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operation",
						).Values(
							Dict{
								Id("Get"): Func().Params().Error().Block(
									List(
										Id(instsVar),
										Id("err"),
									).Op(":=").Id(instConfigVar).Dot("Get").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to get %s instances: %%w",
												defInstObjectHuman,
											)),
											Id("err"),
										)),
									),
									Id(operatedInstsVar).Op("=").Id("append").Call(
										Id(operatedInstsVar),
										Op("*").Id(instsVar).Op("..."),
									),
									Return(Nil()),
								),
								Id("Create"): Func().Params().Error().Block(
									List(
										Id(instVar),
										Id("err"),
									).Op(":=").Id(instConfigVar).Dot("Create").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to create %s instance with name %%s: %%w",
												defInstObjectHuman,
											)),
											Op("*").Id(defInstValuesVar).Dot("Name"),
											Id("err"),
										)),
									),
									Id(operatedInstsVar).Op("=").Id("append").Call(
										Id(operatedInstsVar),
										Op("*").Id(instVar),
									),
									Return(Nil()),
								),
								Id("Replace"): Func().Params(Id("name").String()).Error().Block(
									List(
										Id(instVar),
										Id("err"),
									).Op(":=").Id(instConfigVar).Dot("Replace").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
										Id("name"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to replace %s instance with name %%s: %%w",
												defInstObjectHuman,
											)),
											Id("name"),
											Id("err"),
										)),
									),
									Id(operatedInstsVar).Op("=").Id("append").Call(
										Id(operatedInstsVar),
										Op("*").Id(instVar),
									),
									Return(Nil()),
								),
								Id("Delete"): Func().Params().Error().Block(
									List(
										Op("_"),
										Id("err"),
									).Op("=").Id(instConfigVar).Dot("Delete").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to delete %s instance with name %%s: %%w",
												defInstObjectHuman,
											)),
											Op("*").Id(defInstValuesVar).Dot("Name"),
											Id("err"),
										)),
									),
									Return(Nil()),
								),
								Id("Name"): Lit(fmt.Sprintf("%s instance", defInstObjectHuman)),
							},
						)),
						Line(),

						Return(
							Op("&").Id("operations"),
							Op("&").Id(operatedDefsVar),
							Op("&").Id(operatedInstsVar),
						),
					)
					f.Line()

					// Add mapping function
					f.Commentf(
						"mapTo%sDefinedInstances maps a slice of %s definition and instance configs",
						defInstObject,
						defInstObjectHuman,
					)
					f.Comment(fmt.Sprintf("to a slice of %s config objects", defInstObjectHuman))
					f.Func().Id(fmt.Sprintf("mapTo%sDefinedInstances", defInstObject)).Params(
						Line().Id(defsVar).Op("*").Index().Id(fmt.Sprintf("%sConfig", defObject)),
						Line().Id(instsVar).Op("*").Index().Id(fmt.Sprintf("%sConfig", instObject)),
						Line(),
					).Params(
						Op("*").Index().Id(defInstConfigObjectName),
					).Block(
						Var().Id(
							fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject)),
						).Index().Id(defInstConfigObjectName),
						For(List(Op("_"), Id("inst")).Op(":=").Range().Op("*").Id(instsVar)).Block(
							For(List(Op("_"), Id("def")).Op(":=").Range().Op("*").Id(defsVar)).Block(
								Id("instName").Op(":=").Op("*").Id("inst").Dot(instObject).Dot("Name"),
								Id("defName").Op(":=").Op("*").Id("def").Dot(defObject).Dot("Name"),
								Comment("a defined instance must have matching names for definition and instance"),
								Comment("and the definition must be associated with the instance"),
								If(Id("instName").Op("==").Id("defName").Op("&&").Op("*").Id("inst").Dot(instObject).Dot(defObject).Dot("Name").Op("==").Op("*").Id("def").Dot(defObject).Dot("Name")).Block(
									Commentf(
										"TODO: add fields needed for user to manage a %s and %s together",
										defObject,
										instObject,
									),
									Id(fmt.Sprintf("%sConfig", strcase.ToLowerCamel(defInstObject))).Op(":=").Id(defInstConfigObjectName).Values(
										Dict{
											Line().Id(defInstObject): Id(defInstValuesObjectName).Values(
												Dict{
													Id("Name"): Id("inst").Dot(instObject).Dot("Name"),
													Id("Age"):  Id("inst").Dot(instObject).Dot("Age"),
												},
											).Op(",").Line(),
										},
									),
									Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))).Op("=").Id("append").Call(
										Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject))),
										Id(fmt.Sprintf("%sConfig", strcase.ToLowerCamel(defInstObject))),
									),
									Comment("an instance can only have one matching definition for a defined instance"),
									Comment("we can break out of the loop after finding the first matching definition"),
									Break(),
								),
							),
						),
						Line(),

						Return(Op("&").Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(defInstObject)))),
					)

					// write code to file if it doesn't already exist and not excluded by SDK config
					genFilepath := filepath.Join(
						"pkg",
						"config",
						objCollection.Version,
						fmt.Sprintf("%s.go", strcase.ToSnake(defInstObject)),
					)
					if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
						cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
					} else {
						fileWritten, err := util.WriteCodeToFile(f, genFilepath, false)
						if err != nil {
							return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
						}
						if fileWritten {
							cli.Info(fmt.Sprintf(
								"source code for config package written to %s",
								genFilepath,
							))
						} else {
							cli.Info(fmt.Sprintf(
								"source code for config package already exists at %s - not overwritten",
								genFilepath,
							))
						}
					}
				}

				// create object config abstraction if tptctl commands are enabled
				if apiObject.TptctlCommands {
					// object config abstraction
					configObjectName := fmt.Sprintf("%sConfig", apiObject.TypeName)
					valuesObjectName := fmt.Sprintf("%sValues", apiObject.TypeName)
					objectVar := strcase.ToLowerCamel(apiObject.TypeName)
					methodVar := strings.ToLower(apiObject.TypeName[0:1])
					objectHuman := strcase.ToDelimited(apiObject.TypeName, ' ')
					configFieldTodoComment := fmt.Sprintf(
						"TODO: add config abstraction fields needed for user to manage a %s", apiObject.TypeName,
					)
					apiObjFieldTodoComment := fmt.Sprintf(
						"TODO: add API object fields as needed for %s", apiObject.TypeName,
					)

					var defObject string
					var defValuesObject string
					if apiObject.DefinedInstanceInstance {
						defObject = fmt.Sprintf("%sDefinition", strings.TrimSuffix(apiObject.TypeName, "Instance"))
						defValuesObject = fmt.Sprintf("%sValues", defObject)
					}

					f := NewFile(objCollection.Version)
					f.HeaderComment(sdk.HeaderCommentGenMod)

					// set import paths
					apiImportPath := fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", objCollection.Version)
					clientImportPath := fmt.Sprintf("github.com/threeport/threeport/pkg/client/%s", objCollection.Version)
					if gen.Module {
						apiImportPath = fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version)
						clientImportPath = fmt.Sprintf("%s/pkg/client/%s", gen.ModulePath, objCollection.Version)
					}

					f.ImportAlias(apiImportPath, fmt.Sprintf("api_%s", objCollection.Version))
					f.ImportAlias(clientImportPath, fmt.Sprintf("client_%s", objCollection.Version))
					f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
					f.ImportAlias("errors", "errors")
					if gen.Module {
						f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi_v0")
					}

					f.Commentf(
						"%s is a config abstraction for the %s API object.",
						configObjectName,
						apiObject.TypeName,
					)
					f.Comment("This abstraction allows users to manage API objects with a simplified set of attributes")
					f.Comment("and remove the need for users to interract with API object details such as unique IDs")
					f.Comment("and foreign keys.")
					f.Type().Id(configObjectName).Struct(
						Id(apiObject.TypeName).Id(valuesObjectName),
					)
					f.Line()

					f.Commentf(
						"%s contains all the attributes needed to manage",
						valuesObjectName,
					)
					f.Commentf("the %s API object.", apiObject.TypeName)
					if apiObject.NameField {
						if apiObject.DefinedInstanceInstance {
							f.Type().Id(valuesObjectName).Struct(
								Comment(configFieldTodoComment),
								Id("Name").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
								Id(defObject).Op("*").Id(defValuesObject).Tag(map[string]string{"json": ",omitempty"}),
								Id("Age").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
							)
						} else {
							f.Type().Id(valuesObjectName).Struct(
								Comment(configFieldTodoComment),
								Id("Name").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
								Id("Age").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
							)
						}
					} else {
						if apiObject.DefinedInstanceInstance {
							f.Type().Id(valuesObjectName).Struct(
								Comment(configFieldTodoComment),
								Id(defObject).Op("*").Id(defValuesObject).Tag(map[string]string{"json": ",omitempty"}),
								Id("Age").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
							)
						} else {
							f.Type().Id(valuesObjectName).Struct(
								Comment(configFieldTodoComment),
								Id("Age").Op("*").String().Tag(map[string]string{"json": ",omitempty"}),
							)
						}
					}
					f.Line()

					// Generate Get method
					f.Comment(fmt.Sprintf(
						"Get gets %ss from the Threeport API.",
						objectHuman,
					))
					if apiObject.NameField {
						f.Comment(fmt.Sprintf(
							"If the name is set in the %s, it will return the %s with that name.",
							valuesObjectName,
							objectHuman,
						))
						f.Comment(fmt.Sprintf(
							"If the name is not set, it will return all %ss.",
							objectHuman,
						))
					}
					f.Func().Params(
						Id(methodVar).Op("*").Id(configObjectName),
					).Id("Get").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Index().Id(configObjectName),
						Error(),
					).BlockFunc(func(g *Group) {
						// Extract values from config object
						g.Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(methodVar).Dot(apiObject.TypeName)
						g.Line()
						// Generate first phase: get API objects
						g.Comment("get API objects")
						if apiObject.NameField {
							g.Var().Id(fmt.Sprintf("%ss", strcase.ToLowerCamel(apiObject.TypeName))).Op("*").Index().Qual(
								apiImportPath,
								apiObject.TypeName,
							)
							g.Switch().Block(
								Comment(fmt.Sprintf("if name is provided, get %s by name", objectHuman)),
								Case(Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op("!=").Nil()).Block(
									List(Id(objectVar), Id("err")).Op(":=").Qual(
										clientImportPath,
										fmt.Sprintf("Get%sByName", apiObject.TypeName),
									).Call(
										Id("apiClient"),
										Id("apiEndpoint"),
										Op("*").Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Nil(), Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf("failed to get %s with name %%s: %%w", objectHuman)),
											Op("*").Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
											Id("err"),
										)),
									),
									Id(fmt.Sprintf("%ss", strcase.ToLowerCamel(apiObject.TypeName))).Op("=").Op("&").Index().Qual(
										apiImportPath,
										apiObject.TypeName,
									).Values(Op("*").Id(objectVar)),
								),
								Comment(fmt.Sprintf("get all %ss", objectHuman)),
								Default().Block(
									List(Id(fmt.Sprintf("all%ss", apiObject.TypeName)), Id("err")).Op(":=").Qual(
										clientImportPath,
										fmt.Sprintf("Get%ss", apiObject.TypeName),
									).Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Nil(), Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf("failed to get %ss from Threeport API: %%w", objectHuman)),
											Id("err"),
										)),
									),
									Id(fmt.Sprintf("%ss", strcase.ToLowerCamel(apiObject.TypeName))).Op("=").Id(fmt.Sprintf("all%ss", apiObject.TypeName)),
								),
							)
						} else {
							g.Comment(fmt.Sprintf("get all %ss", objectHuman))
							g.List(Id(fmt.Sprintf("%ss", strcase.ToLowerCamel(apiObject.TypeName))), Id("err")).Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Get%ss", apiObject.TypeName),
							).Call(
								Id("apiClient"),
								Id("apiEndpoint"),
							)
							g.If(Id("err").Op("!=").Nil()).Block(
								Return(Nil(), Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to get %ss from Threeport API: %%w", objectHuman)),
									Id("err"),
								)),
							)
						}
						g.Line()

						// Generate second phase: assemble config objects from API objects
						g.Comment("assemble config objects from API objects")
						g.Var().Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(apiObject.TypeName))).Index().Id(configObjectName)
						g.For(List(Op("_"), Id(objectVar)).Op(":=").Range().Op("*").Id(
							fmt.Sprintf("%ss", strcase.ToLowerCamel(apiObject.TypeName)),
						)).Block(
							Comment(configFieldTodoComment),
							Id(fmt.Sprintf("%sConfig", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(configObjectName).ValuesFunc(func(h *Group) {
								if apiObject.NameField {
									h.Add(Dict{
										Line().Id(apiObject.TypeName): Id(valuesObjectName).Values(
											Dict{
												Id("Name"): Id(objectVar).Dot("Name"),
												Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
													Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
														Id(objectVar).Dot("CreatedAt"),
													),
												),
											},
										).Op(",").Line(),
									})
								} else {
									h.Add(Dict{
										Line().Id(apiObject.TypeName): Id(valuesObjectName).Values(
											Dict{
												Line().Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
													Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
														Id(objectVar).Dot("CreatedAt"),
													),
												),
											},
										).Op(",").Line(),
									})
								}
							}),
							Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(apiObject.TypeName))).Op("=").Id("append").Call(
								Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(apiObject.TypeName))),
								Id(fmt.Sprintf("%sConfig", strcase.ToLowerCamel(apiObject.TypeName))),
							),
						)
						g.Line()

						g.Return(Op("&").Id(fmt.Sprintf("%sConfigs", strcase.ToLowerCamel(apiObject.TypeName))), Nil())
					})
					f.Line()

					// Generate Create method
					f.Comment(fmt.Sprintf(
						"Create creates a %s in the Threeport API.",
						objectHuman,
					))
					f.Func().Params(
						Id(methodVar).Op("*").Id(configObjectName),
					).Id("Create").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Id(configObjectName),
						Error(),
					).BlockFunc(func(g *Group) {
						// Extract values from config object
						g.Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(methodVar).Dot(apiObject.TypeName)
						g.Line()

						g.Comment("validate config")
						if apiObject.NameField {
							g.If(Id("err").Op(":=").Id(methodVar).Dot("Validate").Call(), Id("err").Op("!=").Nil()).Block(
								Return(Nil(), Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to validate values for %s with name %%s: %%w", objectHuman)),
									Op("*").Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
									Id("err"),
								)),
							)
						} else {
							g.If(Id("err").Op(":=").Id(methodVar).Dot("Validate").Call(), Id("err").Op("!=").Nil()).Block(
								Return(Nil(), Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to validate values for %s: %%w", objectHuman)),
									Id("err"),
								)),
							)
						}
						g.Line()

						g.Comment(fmt.Sprintf("construct %s object", objectHuman))
						g.Comment(apiObjFieldTodoComment)
						g.Id(objectVar).Op(":=").Qual(
							apiImportPath,
							apiObject.TypeName,
						).ValuesFunc(func(h *Group) {
							if apiObject.NameField {
								switch {
								case apiObject.DefinedInstanceDefinition:
									h.Add(Dict{
										Line().Id("Definition"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Definition",
										).Values(
											Dict{
												Line().Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op(",").Line(),
											},
										).Op(",").Line(),
									})
								case apiObject.DefinedInstanceInstance:
									h.Add(Dict{
										Line().Id("Instance"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Instance",
										).Values(
											Dict{
												Line().Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op(",").Line(),
											},
										).Op(",").Line(),
									})
								default:
									h.Add(Dict{
										Line().Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op(",").Line(),
									})
								}
							}
						})
						g.Line()

						g.Comment(fmt.Sprintf("create %s", objectHuman))
						g.Id(fmt.Sprintf("created%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
							clientImportPath,
							fmt.Sprintf("Create%s", apiObject.TypeName),
						).Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line().Op("&").Id(objectVar),
							Line(),
						)

						g.If(Id("err").Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf(
									"failed to create %s in threeport API: %%w",
									objectHuman,
								)),
								Id("err"),
							)),
						)
						g.Line()

						g.Comment(fmt.Sprintf("construct %s config", objectHuman))
						g.Comment(configFieldTodoComment)
						g.Id(fmt.Sprintf("created%sConfig", apiObject.TypeName)).Op(":=").Op("&").Id(configObjectName).Values(
							Dict{
								Line().Id(apiObject.TypeName): Id(valuesObjectName).ValuesFunc(func(h *Group) {
									if apiObject.NameField {
										h.Add(Dict{
											Id("Name"): Id(fmt.Sprintf("created%s", apiObject.TypeName)).Dot("Name"),
											Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
												Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
													Id(fmt.Sprintf("created%s", apiObject.TypeName)).Dot("CreatedAt"),
												),
											),
										})
									} else {
										h.Add(Dict{
											Line().Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
												Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
													Id(fmt.Sprintf("created%s", apiObject.TypeName)).Dot("CreatedAt"),
												),
											).Op(",").Line(),
										})
									}
								}).Op(",").Line(),
							},
						)
						g.Line()

						g.Return(
							Id(fmt.Sprintf("created%sConfig", apiObject.TypeName)),
							Nil(),
						)
					})
					f.Line()

					// Generate Replace method
					f.Comment(fmt.Sprintf(
						"Replace updates the entire %s object in the Threeport API.",
						objectHuman,
					))
					f.Comment(fmt.Sprintf(
						"This is a full replacement of all fields in the %s object.",
						objectHuman,
					))
					if apiObject.NameField {
						f.Comment(fmt.Sprintf(
							"This function takes a name parameter to identify the %s to replace.",
							objectHuman,
						))
						f.Comment("This allows a different name to be provided in the values object for name changes.")
					}
					f.Func().Params(
						Id(methodVar).Op("*").Id(configObjectName),
					).Id("Replace").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						func() Code {
							if !apiObject.NameField {
								return Line().Commentf("NOTE: %s has no name field", apiObject.TypeName)
							}
							//return Line().Comment("shite")
							return Null()
						}(),
						func() Code {
							if !apiObject.NameField {
								return Line().Id("name").String().Op(",").Commentf(
									"TODO: replace name with another parameter that can uniquely identify a %s", objectHuman,
								)
							} else {
								return Line().Id("name").String()
							}
						}(),
						Line(),
					).Params(
						Op("*").Id(configObjectName),
						Error(),
					).BlockFunc(func(g *Group) {
						// Extract values from config object
						g.Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(methodVar).Dot(apiObject.TypeName)
						g.Line()

						g.Comment("validate config")
						g.If(Id("err").Op(":=").Id(methodVar).Dot("Validate").Call(), Id("err").Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("invalid %s config: %%w", objectHuman)),
								Id("err"),
							)),
						)
						g.Line()

						if apiObject.NameField {
							g.Comment(fmt.Sprintf("get existing %s by name", objectHuman))
							g.Id(fmt.Sprintf("existing%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Get%sByName", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Id("name"),
								Line(),
							)
							g.If(Id("err").Op("!=").Nil()).Block(
								Return(Nil(), Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to find %s with name %%s: %%w", objectHuman)),
									Id("name"),
									Id("err"),
								)),
							)
						} else {
							g.Commentf("get existing %s", objectHuman)
							g.Id(fmt.Sprintf("existing%ss", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Get%ssByQueryString", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Qual("fmt", "Sprintf").Call(
									Lit("name=%s"),
									Id("name"),
								).Op(",").Commentf(
									"TODO: replace name with another parameter that can uniquely identify a %s", objectHuman,
								),
								Line(),
							)
							g.If(Id("err").Op("!=").Nil()).Block(
								Return(Nil(), Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to find %s with name %%s: %%w", objectHuman)),
									Id("name"),
									Id("err"),
								)),
							)
							g.Commentf("TODO: add check for zero or multiple %ss found", objectHuman)
						}
						g.Line()

						g.Comment(fmt.Sprintf("construct updated %s object", objectHuman))
						g.Comment(apiObjFieldTodoComment)
						g.Id(fmt.Sprintf("updated%s", apiObject.TypeName)).Op(":=").Op("&").Qual(
							apiImportPath,
							apiObject.TypeName,
						).ValuesFunc(func(h *Group) {
							if apiObject.NameField {
								switch {
								case apiObject.DefinedInstanceDefinition:
									h.Add(Dict{
										Id("Common"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Common",
										).Values(
											Dict{
												Line().Id("ID"): Id(fmt.Sprintf("existing%s", apiObject.TypeName)).Dot("ID").Op(",").Line(),
											},
										),
										Id("Definition"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Definition",
										).Values(
											Dict{
												Line().Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op(",").Line(),
											},
										),
									})
								case apiObject.DefinedInstanceInstance:
									h.Add(Dict{
										Id("Common"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Common",
										).Values(
											Dict{
												Line().Id("ID"): Id(fmt.Sprintf("existing%s", apiObject.TypeName)).Dot("ID").Op(",").Line(),
											},
										),
										Id("Instance"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Instance",
										).Values(
											Dict{
												Line().Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op(",").Line(),
											},
										),
									})
								default:
									h.Add(Dict{
										Id("Common"): Qual(
											"github.com/threeport/threeport/pkg/api/v0",
											"Common",
										).Values(
											Dict{
												Line().Id("ID"): Id(fmt.Sprintf("existing%s", apiObject.TypeName)).Dot("ID").Op(",").Line(),
											},
										),
										Id("Name"): Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
									})
								}
							} else {
								h.Add(Dict{
									Line().Id("Common"): Qual(
										"github.com/threeport/threeport/pkg/api/v0",
										"Common",
									).Values(
										Dict{
											Line().Id("ID"): Call(Op("*").Id(fmt.Sprintf("existing%ss", apiObject.TypeName))).Index(Lit(0)).Dot("ID").Op(",").Line(),
										},
									).Op(",").Line(),
								})
							}
						})
						g.Line()

						g.Comment(fmt.Sprintf("replace %s", objectHuman))
						g.Id(fmt.Sprintf("replaced%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
							clientImportPath,
							fmt.Sprintf("Replace%s", apiObject.TypeName),
						).Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line().Id(fmt.Sprintf("updated%s", apiObject.TypeName)),
							Line(),
						)
						g.If(Id("err").Op("!=").Nil()).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to replace %s in threeport API: %%w", objectHuman)),
								Id("err"),
							)),
						)
						g.Line()

						g.Comment(fmt.Sprintf("construct updated %s config", objectHuman))
						g.Comment(configFieldTodoComment)
						g.Id(fmt.Sprintf("updated%sConfig", apiObject.TypeName)).Op(":=").Op("&").Id(configObjectName).Values(
							Dict{
								Line().Id(apiObject.TypeName): Id(valuesObjectName).ValuesFunc(func(g *Group) {
									if apiObject.NameField {
										g.Add(Dict{
											Id("Name"): Id(fmt.Sprintf("replaced%s", apiObject.TypeName)).Dot("Name"),
											Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
												Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
													Id(fmt.Sprintf("replaced%s", apiObject.TypeName)).Dot("CreatedAt"),
												),
											),
										})
									} else {
										g.Add(Dict{
											Line().Id("Age"): Qual("github.com/threeport/threeport/pkg/util/v0", "Ptr").Call(
												Qual("github.com/threeport/threeport/pkg/util/v0", "GetAgeFormatted").Call(
													Id(fmt.Sprintf("replaced%s", apiObject.TypeName)).Dot("CreatedAt"),
												),
											).Op(",").Line(),
										})
									}
								}).Op(",").Line(),
							},
						)
						g.Line()

						g.Return(
							Id(fmt.Sprintf("updated%sConfig", apiObject.TypeName)),
							Nil(),
						)
					})
					f.Line()

					// Generate Delete method
					f.Comment(fmt.Sprintf("Delete deletes a %s from the Threeport API.", objectHuman))
					f.Func().Params(Id(methodVar).Op("*").Id(configObjectName)).Id("Delete").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Id(configObjectName),
						Error(),
					).BlockFunc(func(g *Group) {
						// Extract values from config object
						g.Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(methodVar).Dot(apiObject.TypeName)
						g.Line()
						if apiObject.NameField {
							g.Comment(fmt.Sprintf("get %s by name", objectHuman))
							g.Id(objectVar).Op(",").Id("err").Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Get%sByName", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Op("*").Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
								Line(),
							)
							g.If(Id("err").Op("!=").Nil()).Block(
								Return(
									Nil(),
									Qual("fmt", "Errorf").Call(
										Lit(fmt.Sprintf("failed to find %s with name %%s: %%w", objectHuman)),
										Op("*").Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name"),
										Id("err"),
									),
								),
							)
						} else {
							g.Comment(fmt.Sprintf("get %s", objectHuman))
							g.Id(objectVar).Op(",").Id("err").Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Get%ssByQueryString", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Qual("fmt", "Sprintf").Call(
									Lit("name=%s"),
									Lit("value"),
								).Op(",").Commentf(
									"TODO: replace name with another parameter that can uniquely identify a %s", objectHuman,
								),
								Line(),
							)
							g.If(Id("err").Op("!=").Nil()).Block(
								Return(
									Nil(),
									Qual("fmt", "Errorf").Call(
										Lit(fmt.Sprintf("failed to find %s: %%w", objectHuman)),
										Id("err"),
									),
								),
							)
							g.Commentf("TODO: add check for zero or multiple %ss found", objectHuman)
						}
						g.Line()

						g.Comment(fmt.Sprintf("delete %s", objectHuman))
						if apiObject.NameField {
							g.Id(fmt.Sprintf("deleted%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
								clientImportPath,
								fmt.Sprintf("Delete%s", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Op("*").Id(objectVar).Dot("ID"),
								Line(),
							)
						} else {
							g.Id("_").Op(",").Id("err").Op("=").Qual(
								clientImportPath,
								fmt.Sprintf("Delete%s", apiObject.TypeName),
							).Call(
								Line().Id("apiClient"),
								Line().Id("apiEndpoint"),
								Line().Op("*").Parens(Op("*").Id(objectVar)).Index(Lit(0)).Dot("ID"),
								Line(),
							)
						}
						g.If(Id("err").Op("!=").Nil()).Block(
							Return(
								Nil(),
								Qual("fmt", "Errorf").Call(
									Lit(fmt.Sprintf("failed to delete %s from Threeport API: %%w", objectHuman)),
									Id("err"),
								),
							),
						)
						g.Line()

						g.Comment(fmt.Sprintf("construct deleted %s config", objectHuman))
						g.Comment(configFieldTodoComment)
						g.Id(fmt.Sprintf("deleted%sConfig", apiObject.TypeName)).Op(":=").Op("&").Id(configObjectName).Values(
							Dict{
								Line().Id(apiObject.TypeName): Id(valuesObjectName).ValuesFunc(func(g *Group) {
									if apiObject.NameField {
										g.Add(Dict{
											Line().Id("Name"): Id(fmt.Sprintf("deleted%s", apiObject.TypeName)).Dot("Name").Op(",").Line(),
										})
									}
								}).Op(",").Line(),
							},
						)
						g.Line()

						g.Return(Id(fmt.Sprintf("deleted%sConfig", apiObject.TypeName)), Nil())
					})

					// Generate Validate method
					f.Comment(fmt.Sprintf(
						"Validate validates inputs to create %ss.",
						objectHuman,
					))
					f.Func().Params(
						Id(methodVar).Op("*").Id(configObjectName),
					).Id("Validate").Params().Params(
						Error(),
					).BlockFunc(func(g *Group) {
						// Extract values from config object
						g.Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Op(":=").Id(methodVar).Dot(apiObject.TypeName)
						g.Id("multiError").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "MultiError").Values()
						g.Line()

						if apiObject.NameField {
							g.Comment("ensure name is set")
							g.If(Id(fmt.Sprintf("%sValues", strcase.ToLowerCamel(apiObject.TypeName))).Dot("Name").Op("==").Nil()).Block(
								Id("multiError").Dot("AppendError").Call(
									Qual("errors", "New").Call(Lit("missing required field in config: Name")),
								),
							)
							g.Line()
						}

						g.Comment("TODO: add additional validation as needed")
						g.Line()

						g.Return(Id("multiError").Dot("Error").Call())
					})
					f.Line()

					// write code to file if it doesn't already exist and not excluded by SDK config
					genFilepath := filepath.Join(
						"pkg",
						"config",
						objCollection.Version,
						fmt.Sprintf("%s.go", strcase.ToSnake(apiObject.TypeName)),
					)
					if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
						cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
					} else {
						fileWritten, err := util.WriteCodeToFile(f, genFilepath, false)
						if err != nil {
							return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
						}
						if fileWritten {
							cli.Info(fmt.Sprintf(
								"source code for config package written to %s",
								genFilepath,
							))
						} else {
							cli.Info(fmt.Sprintf(
								"source code for config package already exists at %s - not overwritten",
								genFilepath,
							))
						}
					}
				}
			}
		}
	}

	return nil
}
