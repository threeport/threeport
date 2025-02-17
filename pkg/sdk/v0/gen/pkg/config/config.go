package config

import (
	"fmt"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenConfig generates the config package that processes CLI user configs.
func GenConfig(gen *gen.Generator) error {
	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			f := NewFile(objCollection.Version)
			f.HeaderComment(util.HeaderCommentGenMod)

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

			for _, apiObject := range objGroup.ApiObjects {
				// defined instance config abstraction
				if apiObject.DefinedInstanceDefinition {
					defInstObject := strings.TrimSuffix(apiObject.TypeName, "Definition")
					defInstConfigObjectName := fmt.Sprintf("%sConfig", defInstObject)
					defInstValuesObjectName := fmt.Sprintf("%sValues", defInstObject)
					defInstMethodVar := strings.ToLower(defInstObject[0:1])
					defInstObjectHuman := strcase.ToDelimited(defInstObject, ' ')

					defObject := apiObject.TypeName
					instObject := fmt.Sprintf("%sInstance", defInstObject)
					defVar := strcase.ToLowerCamel(defObject)
					instVar := strcase.ToLowerCamel(instObject)
					defValuesObjectName := fmt.Sprintf("%sValues", defObject)
					instValuesObjectName := fmt.Sprintf("%sValues", instObject)
					defValuesVar := strcase.ToLowerCamel(defValuesObjectName)
					instValuesVar := strcase.ToLowerCamel(instValuesObjectName)

					f.Comment(fmt.Sprintf(
						"%s contains the config for a %s which is an abstraction",
						defInstConfigObjectName,
						defInstObjectHuman,
					))
					f.Comment(fmt.Sprintf(
						"of a %s definition and %[1]s instance.",
						defInstObjectHuman,
					))
					f.Type().Id(defInstConfigObjectName).Struct(
						Id(defInstObject).Id(defInstValuesObjectName).Tag(map[string]string{"yaml": defInstObject}),
					)
					f.Line()

					f.Comment(fmt.Sprintf(
						"%s contains the attributes needed to manage a %s",
						defInstValuesObjectName,
						defInstObjectHuman,
					))
					f.Comment(fmt.Sprintf(
						"definition and %s instance with a single operation.",
						defInstObjectHuman,
					))
					f.Type().Id(defInstValuesObjectName).Struct(
						Id("Name").Op("*").String().Tag(map[string]string{"yaml": "Name"}),
					)
					f.Line()

					f.Comment(fmt.Sprintf(
						"Create creates a %s definition and instance in the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstValuesObjectName)).Id("Create").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Qual(apiImportPath, defObject),
						Op("*").Qual(apiImportPath, instObject),
						Error(),
					).Block(
						Comment("get operations"),
						List(
							Id("operations"),
							Id(fmt.Sprintf("created%s", defObject)),
							Id(fmt.Sprintf("created%s", instObject)),
						).Op(":=").Id(defInstMethodVar).Dot("GetOperations").Call(
							Line().Id("apiClient"),
							Line().Id("apiEndpoint"),
							Line(),
						),
						Line(),

						Comment("execute create operations"),
						If(Err().Op(":=").Id("operations").Dot("Create").Call(), Err().Op("!=").Nil()).Block(
							Return(Nil(), Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute create operations for %s defined instance with name %%s: %%w",
									defInstObjectHuman,
								)),
								Line().Id(defInstMethodVar).Dot("Name"),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Return(
							Id(fmt.Sprintf("created%s", defObject)),
							Id(fmt.Sprintf("created%s", instObject)),
							Nil(),
						),
					)
					f.Line()

					f.Comment(fmt.Sprintf(
						"Delete deletes a %s definition and instance from the Threeport API.",
						defInstObjectHuman,
					))
					f.Func().Params(Id(defInstMethodVar).Op("*").Id(defInstValuesObjectName)).Id("Delete").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Qual(apiImportPath, defObject),
						Op("*").Qual(apiImportPath, instObject),
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
							Return(Nil(), Nil(), Qual("fmt", "Errorf").Call(
								Line().Lit(fmt.Sprintf(
									"failed to execute delete operations for %s defined instance with name %%s: %%w",
									defInstObjectHuman,
								)),
								Line().Id(defInstMethodVar).Dot("Name"),
								Line().Err(),
								Line(),
							)),
						),
						Line(),

						Return(Nil(), Nil(), Nil()),
					)
					f.Line()

					f.Comment("GetOperations returns a slice of operations used to create or delete a")
					f.Comment(fmt.Sprintf("%s defined instance.", defInstObjectHuman))
					f.Func().Params(
						Id(defInstMethodVar).Op("*").Id(defInstValuesObjectName),
					).Id("GetOperations").Params(
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Params(
						Op("*").Qual("github.com/threeport/threeport/pkg/util/v0", "Operations"),
						Op("*").Qual(apiImportPath, defObject),
						Op("*").Qual(apiImportPath, instObject),
					).Block(
						Var().Id("err").Error(),
						Var().Id(fmt.Sprintf("created%s", defObject)).Qual(apiImportPath, defObject),
						Var().Id(fmt.Sprintf("created%s", instObject)).Qual(apiImportPath, instObject),
						Line(),

						Id("operations").Op(":=").Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operations",
						).Values(),
						Line(),

						Comment(fmt.Sprintf("add %s definition operation", defInstObjectHuman)),
						Id(defValuesVar).Op(":=").Id(defValuesObjectName).Values(
							Dict{
								Line().Id("Name"): Id(defInstMethodVar).Dot("Name").Op(",").Line(),
							},
						),
						Id("operations").Dot("AppendOperation").Call(Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operation",
						).Values(
							Dict{
								Id("Name"): Lit(fmt.Sprintf("%s definition", defInstObjectHuman)),
								Id("Create"): Func().Params().Error().Block(
									List(
										Id(defVar),
										Id("err"),
									).Op(":=").Id(defValuesVar).Dot("Create").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to create %s definition with name %%s: %%w",
												defInstObjectHuman,
											)),
											Id(defInstMethodVar).Dot("Name"),
											Id("err"),
										)),
									),
									Id(fmt.Sprintf("created%s", defObject)).Op("=").Op("*").Id(defVar),
									Return(Nil()),
								),
								Id("Delete"): Func().Params().Error().Block(
									List(
										Op("_"),
										Id("err"),
									).Op("=").Id(defValuesVar).Dot("Delete").Call(
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
												Id(defInstMethodVar).Dot("Name"),
												Id("err"),
											),
										),
									),
									Return(Nil()),
								),
							},
						)),
						Line(),

						Comment(fmt.Sprintf("add %s instance operation", defInstObjectHuman)),
						Id(instValuesVar).Op(":=").Id(instValuesObjectName).Values(
							Dict{
								Line().Id("Name"): Id(defInstMethodVar).Dot("Name").Op(",").Line(),
							},
						),
						Id("operations").Dot("AppendOperation").Call(Qual(
							"github.com/threeport/threeport/pkg/util/v0",
							"Operation",
						).Values(
							Dict{
								Id("Name"): Lit(fmt.Sprintf("%s instance", defInstObjectHuman)),
								Id("Create"): Func().Params().Error().Block(
									List(
										Id(instVar),
										Id("err"),
									).Op(":=").Id(instValuesVar).Dot("Create").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to create %s instance with name %%s: %%w",
												defInstObjectHuman,
											)),
											Id(defInstMethodVar).Dot("Name"),
											Id("err"),
										)),
									),
									Id(fmt.Sprintf("created%s", instObject)).Op("=").Op("*").Id(instVar),
									Return(Nil()),
								),
								Id("Delete"): Func().Params().Error().Block(
									List(
										Op("_"),
										Id("err"),
									).Op("=").Id(instValuesVar).Dot("Delete").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Id("err").Op("!=").Nil()).Block(
										Return(Qual("fmt", "Errorf").Call(
											Lit(fmt.Sprintf(
												"failed to delete %s instance with name %%s: %%w",
												defInstObjectHuman,
											)),
											Id(defInstMethodVar).Dot("Name"),
											Id("err"),
										)),
									),
									Return(Nil()),
								),
							},
						)),
						Line(),

						Return(
							Op("&").Id("operations"),
							Op("&").Id(fmt.Sprintf("created%s", defObject)),
							Op("&").Id(fmt.Sprintf("created%s", instObject)),
						),
					)
				}

				// object config abstraction
				configObjectName := fmt.Sprintf("%sConfig", apiObject.TypeName)
				valuesObjectName := fmt.Sprintf("%sValues", apiObject.TypeName)
				objectVar := strcase.ToLowerCamel(apiObject.TypeName)
				methodVar := strings.ToLower(apiObject.TypeName[0:1])
				objectHuman := strcase.ToDelimited(apiObject.TypeName, ' ')

				f.Comment(fmt.Sprintf(
					"%s contains the config for a %s.",
					configObjectName,
					objectHuman,
				))
				f.Type().Id(configObjectName).Struct(
					Id(apiObject.TypeName).Id(valuesObjectName).Tag(map[string]string{"yaml": apiObject.TypeName}),
				)
				f.Line()

				f.Comment(fmt.Sprintf(
					"%s contains the attributes for the %s",
					valuesObjectName,
					objectHuman,
				))
				f.Comment("config abstraction.")
				f.Type().Id(valuesObjectName).Struct(
					Id("Name").Op("*").String().Tag(map[string]string{"yaml": "Name"}),
				)
				f.Line()

				f.Comment(fmt.Sprintf(
					"Create creates a %s in the Threeport API.",
					objectHuman,
				))
				f.Func().Params(
					Id(methodVar).Op("*").Id(valuesObjectName),
				).Id("Create").Params(
					Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
					Line().Id("apiEndpoint").String(),
					Line(),
				).Params(
					Op("*").Qual(apiImportPath, apiObject.TypeName),
					Error(),
				).Block(
					Comment("validate config"),
					Comment("TODO"),
					Line(),

					Comment(fmt.Sprintf("construct %s object", objectHuman)),
					Id(objectVar).Op(":=").Qual(
						apiImportPath,
						apiObject.TypeName,
					).ValuesFunc(func(g *Group) {
						switch {
						case apiObject.DefinedInstanceDefinition:
							g.Add(Dict{
								Line().Id("Definition"): Qual(
									"github.com/threeport/threeport/pkg/api/v0",
									"Definition",
								).Values(
									Dict{
										Line().Id("Name"): Id(methodVar).Dot("Name").Op(",").Line(),
									},
								).Op(",").Line(),
							})
						case apiObject.DefinedInstanceInstance:
							g.Add(Dict{
								Line().Id("Instance"): Qual(
									"github.com/threeport/threeport/pkg/api/v0",
									"Instance",
								).Values(
									Dict{
										Line().Id("Name"): Id(methodVar).Dot("Name").Op(",").Line(),
									},
								).Op(",").Line(),
							})
						default:
							g.Add(Dict{
								Line().Id("Name"): Id(methodVar).Dot("Name").Op(",").Line(),
							})
						}
					}),
					Line(),

					Comment(fmt.Sprintf("create %s", objectHuman)),
					Id(fmt.Sprintf("created%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
						clientImportPath,
						fmt.Sprintf("Create%s", apiObject.TypeName),
					).Call(
						Line().Id("apiClient"),
						Line().Id("apiEndpoint"),
						Line().Op("&").Id(objectVar),
						Line(),
					),

					If(Id("err").Op("!=").Nil()).Block(
						Return(Nil(), Qual("fmt", "Errorf").Call(
							Lit(fmt.Sprintf(
								"failed to create %s in threeport API: %%w",
								objectHuman,
							)),
							Id("err"),
						)),
					),
					Line(),

					Return(
						Id(fmt.Sprintf("created%s", apiObject.TypeName)),
						Nil(),
					),
				)

				f.Comment(fmt.Sprintf("Delete deletes a %s from the Threeport API.", objectHuman))
				f.Func().Params(Id(methodVar).Op("*").Id(valuesObjectName)).Id("Delete").Params(
					Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
					Line().Id("apiEndpoint").String(),
					Line(),
				).Params(
					Op("*").Qual(apiImportPath, apiObject.TypeName),
					Error(),
				).Block(
					Comment(fmt.Sprintf("get %s by name", objectHuman)),
					Id(objectVar).Op(",").Id("err").Op(":=").Qual(
						clientImportPath,
						fmt.Sprintf("Get%sByName", apiObject.TypeName),
					).Call(
						Line().Id("apiClient"),
						Line().Id("apiEndpoint"),
						Line().Op("*").Id(methodVar).Dot("Name"),
						Line(),
					),
					If(Id("err").Op("!=").Nil()).Block(
						Return(
							Nil(),
							Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to find %s with name %%s: %%w", objectHuman)),
								Id(methodVar).Dot("Name"),
								Id("err"),
							),
						),
					),
					Line(),

					Comment(fmt.Sprintf("delete %s", objectHuman)),
					Id(fmt.Sprintf("deleted%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
						clientImportPath,
						fmt.Sprintf("Delete%s", apiObject.TypeName),
					).Call(
						Line().Id("apiClient"),
						Line().Id("apiEndpoint"),
						Line().Op("*").Id(objectVar).Dot("ID"),
						Line(),
					),
					If(Id("err").Op("!=").Nil()).Block(
						Return(
							Nil(),
							Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to delete %s from Threeport API: %%w", objectHuman)),
								Id("err"),
							),
						),
					),
					Line(),

					Return(Id(fmt.Sprintf("deleted%s", apiObject.TypeName)), Nil()),
				)
			}

			// write code to file if it doesn't already exist
			genFilepath := filepath.Join(
				"pkg",
				"config",
				objCollection.Version,
				fmt.Sprintf("%s.go", strcase.ToSnake(objGroup.Name)),
			)
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

	return nil
}
