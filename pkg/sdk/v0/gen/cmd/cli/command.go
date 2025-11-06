package cli

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

// GenCliCommands generates commands for the tptctl CLI tool and its plugins.
func GenCliCommands(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	// set values for threeport and extensions where different
	exampleCmdStr := "tptctl"
	if gen.Module {
		exampleCmdStr = fmt.Sprintf("tptctl %s", strcase.ToKebab(sdkConfig.ModuleName))
	}

	// set import paths for threeport and extensions where different
	apiImportPath := "github.com/threeport/threeport/pkg/api/"
	clientImportPath := "github.com/threeport/threeport/pkg/client/"
	configImportPath := "github.com/threeport/threeport/pkg/config/"
	if gen.Module {
		apiImportPath = fmt.Sprintf("%s/pkg/api/", gen.ModulePath)
		clientImportPath = fmt.Sprintf("%s/pkg/client/", gen.ModulePath)
		configImportPath = fmt.Sprintf("%s/pkg/config/", gen.ModulePath)
	}

	for _, apiObjGroup := range gen.ApiObjectGroups {
		// commandCode contains the standard tptctl commands for a threeport object
		commandCode := NewFile("cmd")
		commandCode.HeaderComment(util.HeaderCommentGenMod)
		commandCode.ImportAlias("gopkg.in/yaml.v2", "yaml")
		commandCode.ImportAlias("github.com/ghodss/yaml", "ghodss_yaml")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/cli/v0", "cli")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/encryption/v0", "encryption")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
		if gen.Module {
			commandCode.ImportAlias("github.com/threeport/threeport/cmd/tptctl/cmd", "tptctl_cmd")
			commandCode.ImportAlias("github.com/threeport/threeport/pkg/config/v0", "tptctl_config")
		} else {
			commandCode.ImportAlias("github.com/threeport/threeport/pkg/config/v0", "config")
		}

		// getOutputCode contains the customized output for `tptctl get` commands
		// this file is written if it doesn't exist, otherwise is left for developer
		// customization
		getOutputCode := NewFile("cmd")
		getOutputCode.HeaderComment(util.HeaderCommentGenMod)
		getOutputCode.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")

		// add config import for get output functions
		for _, apiObj := range apiObjGroup.UnversionedApiObjects {
			if apiObj.TptctlCommands {
				for _, version := range apiObj.Versions {
					getOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", configImportPath, version),
						fmt.Sprintf("config_%s", version),
					)
				}
			}
		}

		// no code will be generated if tptctl is not enabled on API
		// model
		commandsGenerated := false

		// declare flag vars
		nameVar := fmt.Sprintf("%sName", apiObjGroup.ControllerDomainLower)
		configPathVar := fmt.Sprintf("%sConfigPath", apiObjGroup.ControllerDomainLower)
		versionVar := fmt.Sprintf("%sVersion", apiObjGroup.ControllerDomainLower)
		outputVar := fmt.Sprintf("%sOutput", apiObjGroup.ControllerDomainLower)
		commandCode.Var().Defs(
			Id(nameVar).String(),
			Id(configPathVar).String(),
			Id(versionVar).String(),
			Id(outputVar).String(),
		)
		commandCode.Line()

		for _, apiObj := range apiObjGroup.UnversionedApiObjects {
			if apiObj.TptctlCommands {
				commandsGenerated = true

				// set import alias for each API version
				for _, version := range apiObj.Versions {
					commandCode.ImportAlias(
						fmt.Sprintf("%s%s", apiImportPath, version),
						fmt.Sprintf("api_%s", version),
					)
					commandCode.ImportAlias(
						fmt.Sprintf("%s%s", clientImportPath, version),
						fmt.Sprintf("client_%s", version),
					)
					commandCode.ImportAlias(
						fmt.Sprintf("%s%s", configImportPath, version),
						fmt.Sprintf("config_%s", version),
					)
				}
				for _, version := range apiObj.Versions {
					getOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", apiImportPath, version),
						fmt.Sprintf("api_%s", version),
					)
					getOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", clientImportPath, version),
						fmt.Sprintf("client_%s", version),
					)
					getOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", configImportPath, version),
						fmt.Sprintf("config_%s", version),
					)
				}

				// commands for defined instance abstractions
				//if apiObj.DefinedInstanceInstance {
				if apiObj.DefinedInstanceDefinition {
					//rootObj := strings.TrimSuffix(apiObj.TypeName, "Instance")
					rootObj := strings.TrimSuffix(apiObj.TypeName, "Definition")
					rootCmdStr := strcase.ToKebab(rootObj)
					rootCmdStrHuman := strcase.ToDelimited(rootObj, ' ')
					rootObjectVar := strcase.ToLowerCamel(rootObj)
					rootObjectConfigVar := fmt.Sprintf("%sConfig", rootObjectVar)
					objectConfigObj := fmt.Sprintf("%sConfig", rootObj)
					objectValuesObj := fmt.Sprintf("%sValues", rootObj)

					commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
					commandCode.Comment(rootObj)
					commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
					commandCode.Line()

					// defined instance get command
					getCmdVar := fmt.Sprintf("Get%sCmd", pluralize.Pluralize(rootObj, 2, false))
					//getCmdOutputFunc := fmt.Sprintf("output%s%s", apiObj.Version, getCmdVar)

					commandCode.Comment(fmt.Sprintf(
						"%s represents the command '%s get %s'",
						getCmdVar,
						exampleCmdStr,
						pluralize.Pluralize(rootCmdStr, 2, false),
					))
					commandCode.Var().Id(getCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"):     Lit(pluralize.Pluralize(rootCmdStr, 2, false)),
						Id("Aliases"): Index().String().Values(Lit(rootCmdStr)),
						Id("Example"): Lit(fmt.Sprintf(
							"  # get all %s\n  %s get %s\n\n  # get a specific %s\n  %s get %s --name some-%s",
							pluralize.Pluralize(rootCmdStrHuman, 2, false),
							exampleCmdStr,
							pluralize.Pluralize(rootCmdStr, 2, false),
							rootCmdStrHuman,
							exampleCmdStr,
							rootCmdStr,
							rootCmdStr,
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Get %s from the system",
							pluralize.Pluralize(rootCmdStrHuman, 2, false),
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Get %s from the system. Use --name to get a specific %s. A %[2]s is a unified abstraction of a %[2]s definition and %[2]s instance.",
							pluralize.Pluralize(rootCmdStrHuman, 2, false),
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"):       Id("CommandPreRunFunc"),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Module {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("requestedControlPlane"),
								).Op(":=").Qual(
									"github.com/threeport/threeport/cmd/tptctl/cmd",
									"GetClientContext",
								).Call(Id("cmd"))
							} else {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("requestedControlPlane"),
								).Op(":=").Id("GetClientContext").Call(Id("cmd"))
							}
							g.Line()
							g.Comment("flag validation")
							g.If(Err().Op(":=").Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"ValidateConfigNameFlags",
							).Call(
								Line().Id(configPathVar),
								Line().Id(nameVar),
								Line().Lit(rootCmdStrHuman),
								Line(),
							), Err().Op("!=").Nil()).Block(
								Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
									Lit("flag validation failed"), Err()),
								Qual("os", "Exit").Call(Lit(1)),
							)
							g.Line()
							g.Comment(fmt.Sprintf("get %s based on version", rootCmdStrHuman))
							g.Switch(Id(versionVar)).BlockFunc(func(h *Group) {
								for _, version := range apiObj.Versions {
									h.Case(Lit(version)).Block(
										Commentf("load %s values", rootCmdStrHuman),
										Id(fmt.Sprintf("%sConfig", rootObjectVar)).Op(":=").Qual(
											fmt.Sprintf("%s%s", configImportPath, version),
											objectConfigObj,
										).Values(),
										If(Id(configPathVar).Op("!=").Lit("")).Block(
											Id("configContent").Op(",").Err().Op(":=").Qual("os", "ReadFile").Call(
												Id(configPathVar),
											),
											If(Err().Op("!=").Nil()).Block(
												Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
													Lit("failed to read config file"), Err()),
												Qual("os", "Exit").Call(Lit(1)),
											),
											If(Err().Op(":=").Qual("gopkg.in/yaml.v2", "UnmarshalStrict").Call(
												Id("configContent"), Op("&").Id(fmt.Sprintf("%sConfig", rootObjectVar)),
											), Err().Op("!=").Nil()).Block(
												Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
													Lit("failed to unmarshal config file yaml content"), Err()),
												Qual("os", "Exit").Call(Lit(1)),
											),
										).Else().If(Id(nameVar).Op("!=").Lit("")).Block(
											Id(fmt.Sprintf("%sConfig", rootObjectVar)).Op("=").Qual(
												fmt.Sprintf("%s%s", configImportPath, version),
												objectConfigObj,
											).Values(Dict{
												Line().Id(rootObj): Qual(
													fmt.Sprintf("%s%s", configImportPath, version),
													objectValuesObj,
												).Values(Dict{
													Line().Id("Name"): Op("&").Id(nameVar).Op(",").Line(),
												}).Op(",").Line(),
											}),
										),
										Line(),
										Comment(fmt.Sprintf("get %s", rootCmdStrHuman)),
										List(
											Id(fmt.Sprintf("%sConfigs", rootObjectVar)),
											Err(),
										).Op(":=").Id(fmt.Sprintf("%sConfig", rootObjectVar)).Dot("Get").Call(
											Id("apiClient"),
											Id("apiEndpoint"),
										),
										If(Err().Op("!=").Nil()).Block(
											Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
												Lit(fmt.Sprintf("failed to retrieve %s", rootCmdStrHuman)), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Line(),
										Comment(fmt.Sprintf("check if %s exists", rootCmdStrHuman)),
										If(Len(Op("*").Id(fmt.Sprintf("%sConfigs", rootObjectVar))).Op("==").Lit(0)).Block(
											Qual("github.com/threeport/threeport/pkg/cli/v0", "Info").Call(
												Qual("fmt", "Sprintf").Call(
													Line().Lit(fmt.Sprintf(
														"no %s found that are currently managed by %%s threeport control plane",
														pluralize.Pluralize(rootCmdStrHuman, 2, false),
													)),
													Line().Id("requestedControlPlane"),
													Line(),
												),
											),
											Qual("os", "Exit").Call(Lit(0)),
										),
										Line(),
										Comment("write the output"),
										Switch(Id(outputVar)).Block(
											Case(Lit("tabular")).Block(
												If(Err().Op(":=").Id(fmt.Sprintf("outputGet%s%sCmd", version, pluralize.Pluralize(rootObj, 2, false))).Call(
													Id(fmt.Sprintf("%sConfigs", rootObjectVar)),
												), Err().Op("!=").Nil()).Block(
													Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
														Lit("failed to produce output"), Err()),
													Qual("os", "Exit").Call(Lit(1)),
												),
											),
											Case(Lit("yaml")).Block(
												If(Err().Op(":=").Qual(
													"github.com/threeport/threeport/pkg/cli/v0",
													"YamlObjectOutput",
												).Call(
													Op("*").Id(fmt.Sprintf("%sConfigs", rootObjectVar)),
												), Err().Op("!=").Nil()).Block(
													Qual(
														"github.com/threeport/threeport/pkg/cli/v0",
														"Error",
													).Call(
														Lit("failed to produce YAML output"), Err()),
													Qual("os", "Exit").Call(Lit(1)),
												),
											),
											Case(Lit("json")).Block(
												If(Err().Op(":=").Qual(
													"github.com/threeport/threeport/pkg/cli/v0",
													"JsonObjectOutput",
												).Call(
													Op("*").Id(fmt.Sprintf("%sConfigs", rootObjectVar)),
												), Err().Op("!=").Nil()).Block(
													Qual(
														"github.com/threeport/threeport/pkg/cli/v0",
														"Error",
													).Call(Lit("failed to produce JSON output"), Err()),
													Qual("os", "Exit").Call(Lit(1)),
												),
											),
											Default().Block(
												Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
													Lit(""), Qual("fmt", "Errorf").Call(Lit("unrecognized output format: %s"), Id(outputVar))),
												Qual("os", "Exit").Call(Lit(1)),
											),
										),
									)
								}
								h.Default().Block(
									Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
										Lit(""), Qual("errors", "New").Call(Lit("unrecognized object version"))),
									Qual("os", "Exit").Call(Lit(1)),
								)
							})
						}),
					})

					commandCode.Func().Id("init").Params().Block(
						Id("GetCmd").Dot("AddCommand").Call(Id(getCmdVar)),
						Line(),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(nameVar),
							Line().Lit("name"), Lit("n"), Lit(""),
							Lit(fmt.Sprintf("Name of %s.", rootCmdStrHuman)),
							Line(),
						),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(configPathVar),
							Line().Lit("config"), Lit("c"), Lit(""),
							Lit(fmt.Sprintf("Path to file with %s config.", rootCmdStrHuman)),
							Line(),
						),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(versionVar),
							Line().Lit("version"), Lit("v"), Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
							Lit(fmt.Sprintf(
								"Version of %s objects to retrieve. One of: %s",
								rootCmdStrHuman, apiObj.Versions,
							)),
							Line(),
						),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(outputVar),
							Line().Lit("output"), Lit("o"), Lit("tabular"),
							Lit(fmt.Sprintf(
								"Output format for %s objects. One of: [tabular, yaml, json]",
								rootCmdStrHuman,
							)),
							Line(),
						),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"), Lit("i"), Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
					)

					// defined instance create command
					createCmdVar := fmt.Sprintf("Create%sCmd", rootObj)
					configPathField := fmt.Sprintf("%sConfigPath", apiObjGroup.ControllerDomain)
					createdSliceVar := fmt.Sprintf("created%sSlice", rootObj)
					createdRootObjVar := fmt.Sprintf("created%s", rootObj)

					// for models that use configs that reference other files the config
					// path variable must be set on the config object
					setConfigPath := &Statement{}
					if apiObj.TptctlConfigPath || apiObj.DefinedInstanceTptctlConfigPath {
						setConfigPath.Id(rootObjectVar).Dot(configPathField).Op("=").Op("&").Id(configPathVar)
					}

					commandCode.Comment(fmt.Sprintf(
						"%s represents the command '%s create %s'",
						createCmdVar,
						exampleCmdStr,
						rootCmdStr,
					))
					commandCode.Var().Id(createCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"): Lit(rootCmdStr),
						Id("Example"): Lit(fmt.Sprintf(
							"  # create a new %s using a config file\n  %s create %s --config path/to/config.yaml",
							rootCmdStrHuman,
							exampleCmdStr,
							rootCmdStr,
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Create a new %s",
							rootCmdStrHuman,
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Create a new %[1]s. A %[1]s is a unified abstraction of a %[1]s definition and %[1]s instance. This command creates both a new %[1]s definition and %[1]s instance.",
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"):       Id("CommandPreRunFunc"),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Module {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("_"),
								).Op(":=").Qual(
									"github.com/threeport/threeport/cmd/tptctl/cmd",
									"GetClientContext",
								).Call(Id("cmd"))
							} else {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("_"),
								).Op(":=").Id("GetClientContext").Call(Id("cmd"))
							}
							g.Line()
							g.Comment(fmt.Sprintf(
								"read %s config",
								rootCmdStrHuman,
							))
							g.Id("configContent").Op(",").Err().Op(":=").Qual("os", "ReadFile").Call(
								Id(configPathVar),
							)
							g.If(Err().Op("!=").Nil()).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit("failed to read config file"), Err()),
								Qual("os", "Exit").Call(Lit(1)),
							)
							g.Line()
							g.Comment(fmt.Sprintf("create %s based on version", rootCmdStrHuman))
							g.Switch(Id(versionVar)).BlockFunc(func(h *Group) {
								for _, version := range apiObj.Versions {
									h.Case(Lit(version)).Block(
										Var().Id(rootObjectConfigVar).Qual(
											fmt.Sprintf("%s%s", configImportPath, version),
											objectConfigObj,
										),
										If(Err().Op(":=").Qual(
											"gopkg.in/yaml.v2",
											"UnmarshalStrict",
										).Call(Id("configContent"), Op("&").Id(rootObjectConfigVar)), Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit("failed to unmarshal config file yaml content"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Line(),
										Comment(fmt.Sprintf(
											"create %s",
											rootCmdStrHuman,
										)),
										Add(setConfigPath),
										Id(createdSliceVar).Op(",").Err().Op(":=").Id(rootObjectConfigVar).Dot("Create").Call(
											Line().Id("apiClient"),
											Line().Id("apiEndpoint"),
											Line(),
										),
										If(Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit(fmt.Sprintf(
												"failed to create %s",
												rootCmdStrHuman,
											)), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Line(),
										Comment("check the result of the create"),
										If(Id(createdSliceVar).Op("==").Nil().Op("||").Len(Op("*").Id(createdSliceVar)).Op("==").Lit(0)).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit(fmt.Sprintf(
												"failed to create %s",
												rootCmdStrHuman,
											)), Qual("errors", "New").Call(Lit(fmt.Sprintf(
												"no %s received after create", pluralize.Pluralize(rootCmdStrHuman, 2, false),
											)))),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Id(createdRootObjVar).Op(":=").Parens((Op("*").Id(createdSliceVar))).Index(Lit(0)),
										Line(),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s definition and instance with name %%s created",
											rootCmdStrHuman,
										)), Op("*").Id(createdRootObjVar).Dot(rootObj).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Complete",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s %%s created",
											rootCmdStrHuman,
										)), Op("*").Id(createdRootObjVar).Dot(rootObj).Dot("Name"))),
									)
									h.Default().Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(
											Lit(""),
											Qual("errors", "New").Call(
												Lit("unrecognized object version"),
											),
										),
										Qual("os", "Exit").Call(Lit(1)),
									)
								}
							})
						}),
					})

					commandCode.Func().Id("init").Params().Block(
						Id("CreateCmd").Dot("AddCommand").Call(Id(createCmdVar)),
						Line(),
						Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(configPathVar),
							Line().Lit("config"),
							Lit("c"),
							Lit(""),
							Lit(fmt.Sprintf(
								"Path to file with %s config.",
								rootCmdStrHuman,
							)),
							Line(),
						),
						Id(createCmdVar).Dot("MarkFlagRequired").Call(Lit("config")),
						Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"),
							Lit("i"),
							Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
						Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(versionVar),
							Line().Lit("version"),
							Lit("v"),
							Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
							Lit(fmt.Sprintf(
								"Version of %s object to create. One of: %s",
								pluralize.Pluralize(rootCmdStrHuman, 2, false),
								apiObj.Versions,
							)),
							Line(),
						),
					)

					// defined instance delete command
					deleteCmdVar := fmt.Sprintf("Delete%sCmd", rootObj)

					// for models that use configs that reference other files the config
					// path variable must be set on the config object
					setConfigPath = &Statement{}
					if apiObj.TptctlConfigPath {
						setConfigPath.Id(rootObjectVar).Dot(configPathField).Op("=").Op("&").Id(configPathVar)
					}

					commandCode.Comment(fmt.Sprintf(
						"%s represents the command '%s delete %s'",
						deleteCmdVar,
						exampleCmdStr,
						rootCmdStr,
					))
					commandCode.Var().Id(deleteCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"): Lit(rootCmdStr),
						Id("Example"): Lit(fmt.Sprintf(
							"  # delete using a config file\n  %[1]s delete %[2]s --config path/to/config.yaml\n\n  # delete using name\n  %[1]s delete %[2]s --name some-%[2]s",
							exampleCmdStr,
							rootCmdStr,
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Delete an existing %s",
							rootCmdStrHuman,
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Delete an existing %[1]s. This command deletes an existing %[1]s definition and %[1]s instance.",
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"):       Id("CommandPreRunFunc"),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Module {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("_"),
								).Op(":=").Qual(
									"github.com/threeport/threeport/cmd/tptctl/cmd",
									"GetClientContext",
								).Call(Id("cmd"))
							} else {
								g.List(
									Id("apiClient"),
									Id("_"),
									Id("apiEndpoint"),
									Id("_"),
								).Op(":=").Id("GetClientContext").Call(Id("cmd"))
							}
							g.Line()
							g.Comment("flag validation")
							g.If(Id(configPathVar)).Op("==").Lit("").Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit("flag validation failed"), Qual("errors", "New").Call(Lit("config file path is required"))),
							)
							g.Line()
							g.Comment(fmt.Sprintf(
								"read %s config",
								rootCmdStrHuman,
							))
							g.List(
								Id("configContent"),
								Err(),
							).Op(":=").Qual("os", "ReadFile").Call(Id(configPathVar))
							g.If(Err().Op("!=").Nil()).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit("failed to read config file"), Err()),
								Qual("os", "Exit").Call(Lit(1)),
							)
							g.Line()
							g.Comment(fmt.Sprintf("delete %s based on version", rootCmdStrHuman))
							g.Switch().Id(versionVar).BlockFunc(func(h *Group) {
								for _, version := range apiObj.Versions {
									h.Case(Lit(version)).Block(
										Var().Id(rootObjectConfigVar).Qual(
											fmt.Sprintf("%s%s", configImportPath, version),
											objectConfigObj,
										),
										If(Err().Op(":=").Qual(
											"gopkg.in/yaml.v2",
											"UnmarshalStrict",
										).Call(Id("configContent"), Op("&").Id(rootObjectConfigVar)), Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit("failed to unmarshal config file yaml content"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Line(),
										Comment(fmt.Sprintf(
											"delete %s",
											rootCmdStrHuman,
										)),
										Add(setConfigPath),
										Id("_").Op(",").Err().Op("=").Id(rootObjectConfigVar).Dot("Delete").Call(
											Id("apiClient"), Id("apiEndpoint"),
										),
										If(Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit(fmt.Sprintf(
												"failed to delete %s",
												rootCmdStrHuman,
											)), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										Line(),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s definition %%s deleted",
											rootCmdStrHuman,
										)), Op("*").Id(rootObjectConfigVar).Dot(rootObj).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s instance %%s deleted",
											rootCmdStrHuman,
										)), Op("*").Id(rootObjectConfigVar).Dot(rootObj).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Complete",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s %%s deleted",
											rootCmdStrHuman,
										)), Op("*").Id(rootObjectConfigVar).Dot(rootObj).Dot("Name"))),
									)
									h.Default().Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(
											Lit(""),
											Qual("errors", "New").Call(
												Lit("unrecognized object version"),
											),
										),
										Qual("os", "Exit").Call(Lit(1)),
									)
								}
							})
						}),
					})

					commandCode.Func().Id("init").Params().Block(
						Id("DeleteCmd").Dot("AddCommand").Call(Id(deleteCmdVar)),
						Line(),
						Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(configPathVar),
							Line().Lit("config"),
							Lit("c"),
							Lit(""),
							Lit(fmt.Sprintf(
								"Path to file with %s config.",
								rootCmdStrHuman,
							)),
							Line(),
						),
						Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"),
							Lit("i"),
							Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
						Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(versionVar),
							Line().Lit("version"),
							Lit("v"),
							Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
							Lit(fmt.Sprintf(
								"Version of %s object to delete. One of: %s",
								pluralize.Pluralize(rootCmdStrHuman, 2, false),
								apiObj.Versions,
							)),
							Line(),
						),
					)
				}

				cmdStr := strcase.ToKebab(apiObj.TypeName)
				cmdStrHuman := strcase.ToDelimited(apiObj.TypeName, ' ')
				objectVar := strcase.ToLowerCamel(apiObj.TypeName)
				objectConfigVar := fmt.Sprintf("%sConfig", objectVar)
				objectConfigObj := fmt.Sprintf("%sConfig", apiObj.TypeName)
				objectValuesObj := fmt.Sprintf("%sValues", apiObj.TypeName)
				configPathField := fmt.Sprintf("%sConfigPath", apiObjGroup.ControllerDomain)

				commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
				commandCode.Comment(apiObj.TypeName)
				commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
				commandCode.Line()

				// variable declarations
				getCmdVar := fmt.Sprintf("Get%sCmd", pluralize.Pluralize(apiObj.TypeName, 2, false))
				createCmdVar := fmt.Sprintf("Create%sCmd", apiObj.TypeName)
				replaceCmdVar := fmt.Sprintf("Replace%sCmd", apiObj.TypeName)
				deleteCmdVar := fmt.Sprintf("Delete%sCmd", apiObj.TypeName)

				createdObjVar := fmt.Sprintf("created%s", apiObj.TypeName)
				updatedObjVar := fmt.Sprintf("updated%s", apiObj.TypeName)
				deletedObjVar := fmt.Sprintf("deleted%s", apiObj.TypeName)

				commandCode.Comment(fmt.Sprintf(
					"%s represents the command '%s get %s'",
					getCmdVar,
					exampleCmdStr,
					pluralize.Pluralize(cmdStr, 2, false),
				))
				commandCode.Var().Id(getCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"):     Lit(pluralize.Pluralize(cmdStr, 2, false)),
					Id("Aliases"): Index().String().Values(Lit(cmdStr)),
					Id("Example"): Lit(fmt.Sprintf(
						"  # get all %s\n  %s get %s\n\n  # get a specific %s\n  %s get %s --name some-%s",
						pluralize.Pluralize(cmdStrHuman, 2, false),
						exampleCmdStr,
						pluralize.Pluralize(cmdStr, 2, false),
						cmdStrHuman,
						exampleCmdStr,
						cmdStr,
						cmdStr,
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Get %s from the system",
						pluralize.Pluralize(cmdStrHuman, 2, false),
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Get %s from the system. Use --name to get a specific %s.",
						pluralize.Pluralize(cmdStrHuman, 2, false),
						cmdStrHuman,
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"):       Id("CommandPreRunFunc"),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Module {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("requestedControlPlane"),
							).Op(":=").Qual(
								"github.com/threeport/threeport/cmd/tptctl/cmd",
								"GetClientContext",
							).Call(Id("cmd"))
						} else {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("requestedControlPlane"),
							).Op(":=").Id("GetClientContext").Call(Id("cmd"))
						}
						g.Line()
						g.Comment("flag validation")
						g.If(Err().Op(":=").Qual(
							"github.com/threeport/threeport/pkg/cli/v0",
							"ValidateConfigNameFlags",
						).Call(
							Line().Id(configPathVar),
							Line().Id(nameVar),
							Line().Lit(cmdStrHuman),
							Line(),
						), Err().Op("!=").Nil()).Block(
							Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
								Lit("flag validation failed"), Err()),
							Qual("os", "Exit").Call(Lit(1)),
						)
						g.Line()
						g.Switch().Id(versionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Comment("load values"),
									Id(fmt.Sprintf("%sConfig", objectVar)).Op(":=").Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									).Values(),
									If(Id(configPathVar).Op("!=").Lit("")).Block(
										Id("configContent").Op(",").Err().Op(":=").Qual("os", "ReadFile").Call(
											Id(configPathVar),
										),
										If(Err().Op("!=").Nil()).Block(
											Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
												Lit("failed to read config file"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										If(Err().Op(":=").Qual("gopkg.in/yaml.v2", "UnmarshalStrict").Call(
											Id("configContent"), Op("&").Id(fmt.Sprintf("%sConfig", objectVar)),
										), Err().Op("!=").Nil()).Block(
											Qual("github.com/threeport/threeport/pkg/cli/v0", "Error").Call(
												Lit("failed to unmarshal config file yaml content"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
									).Else().If(Id(nameVar).Op("!=").Lit("")).Block(
										Id(fmt.Sprintf("%sConfig", objectVar)).Op("=").Qual(
											fmt.Sprintf("%s%s", configImportPath, version),
											objectConfigObj,
										).Values(Dict{
											Line().Id(apiObj.TypeName): Qual(
												fmt.Sprintf("%s%s", configImportPath, version),
												objectValuesObj,
											).Values(Dict{
												Line().Id("Name"): Op("&").Id(nameVar).Op(",").Line(),
											}).Op(",").Line(),
										}),
									),
									Line(),
									Comment(fmt.Sprintf(
										"get %s",
										pluralize.Pluralize(cmdStrHuman, 2, false),
									)),
									List(
										Id(pluralize.Pluralize(objectVar, 2, false)),
										Err(),
									).Op(":=").Id(fmt.Sprintf("%sConfig", objectVar)).Dot("Get").Call(
										Id("apiClient"),
										Id("apiEndpoint"),
									),
									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit(fmt.Sprintf(
											"failed to retrieve %s",
											pluralize.Pluralize(cmdStrHuman, 2, false),
										)), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Comment(fmt.Sprintf("check if %s exists", cmdStrHuman)),
									If(Len(Op("*").Id(pluralize.Pluralize(objectVar, 2, false))).Op("==").Lit(0)).Block(
										Qual("github.com/threeport/threeport/pkg/cli/v0", "Info").Call(
											Qual("fmt", "Sprintf").Call(
												Line().Lit(fmt.Sprintf(
													"no %s found that are currently managed by %%s threeport control plane",
													pluralize.Pluralize(cmdStrHuman, 2, false),
												)),
												Line().Id("requestedControlPlane"),
												Line(),
											),
										),
										Qual("os", "Exit").Call(Lit(0)),
									),
									Line(),
									Comment("write the output"),
									Switch(Id(outputVar)).Block(
										Case(Lit("tabular")).Block(
											If(
												Err().Op(":=").Id(
													fmt.Sprintf(
														"outputGet%s%sCmd",
														version,
														pluralize.Pluralize(apiObj.TypeName, 2, false),
													),
												).Call(
													Id(pluralize.Pluralize(objectVar, 2, false)),
												),
												Err().Op("!=").Nil(),
											).Block(
												Qual(
													"github.com/threeport/threeport/pkg/cli/v0",
													"Error",
												).Call(Lit("failed to produce output"), Err()),
												Qual("os", "Exit").Call(Lit(1)),
											),
										),
										Case(Lit("yaml")).Block(
											If(Err().Op(":=").Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"YamlObjectOutput",
											).Call(
												Op("*").Id(pluralize.Pluralize(objectVar, 2, false)),
											), Err().Op("!=").Nil()).Block(
												Qual(
													"github.com/threeport/threeport/pkg/cli/v0",
													"Error",
												).Call(Lit("failed to produce YAML output"), Err()),
												Qual("os", "Exit").Call(Lit(1)),
											),
										),
										Case(Lit("json")).Block(
											If(Err().Op(":=").Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"JsonObjectOutput",
											).Call(
												Op("*").Id(pluralize.Pluralize(objectVar, 2, false)),
											), Err().Op("!=").Nil()).Block(
												Qual(
													"github.com/threeport/threeport/pkg/cli/v0",
													"Error",
												).Call(Lit("failed to produce JSON output"), Err()),
												Qual("os", "Exit").Call(Lit(1)),
											),
										),
										Default().Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit(""), Qual("fmt", "Errorf").Call(Lit("unrecognized output format: %s"), Id(outputVar))),
											Qual("os", "Exit").Call(Lit(1)),
										),
									),
								)
							}
							h.Default().Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(
									Lit(""),
									Qual("errors", "New").Call(
										Lit("unrecognized object version"),
									),
								),
								Qual("os", "Exit").Call(Lit(1)),
							)
						})
					}),
				})

				commandCode.Func().Id("init").Params().Block(
					Id("GetCmd").Dot("AddCommand").Call(Id(getCmdVar)),
					Line(),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(nameVar),
						Line().Lit("name"), Lit("n"), Lit(""),
						Lit(fmt.Sprintf("Name of %s.", cmdStrHuman)),
						Line(),
					),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(configPathVar),
						Line().Lit("config"), Lit("c"), Lit(""),
						Lit(fmt.Sprintf("Path to file with %s config.", cmdStrHuman)),
						Line(),
					),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(versionVar),
						Line().Lit("version"), Lit("v"), Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s objects to retrieve. One of: %s",
							cmdStrHuman, apiObj.Versions,
						)),
						Line(),
					),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(outputVar),
						Line().Lit("output"), Lit("o"), Lit("tabular"),
						Lit(fmt.Sprintf(
							"Output format for %s objects. One of: [tabular, yaml, json]",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"), Lit("i"), Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
				)

				// create command
				// for models that use configs that reference other files the config
				// path variable must be set on the config object
				setConfigPath := &Statement{}
				if apiObj.TptctlConfigPath {
					setConfigPath.Id(objectVar).Dot(configPathField).Op("=").Op("&").Id(configPathVar)
				}

				commandCode.Comment(fmt.Sprintf(
					"%s represents the command '%s create %s'",
					createCmdVar,
					exampleCmdStr,
					cmdStr,
				))
				commandCode.Var().Id(createCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  # create a new %s using a config file\n  %s create %s --config path/to/config.yaml",
						cmdStrHuman,
						exampleCmdStr,
						cmdStr,
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Create a new %s",
						cmdStrHuman,
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Create a new %s.",
						cmdStrHuman,
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"):       Id("CommandPreRunFunc"),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Module {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Qual(
								"github.com/threeport/threeport/cmd/tptctl/cmd",
								"GetClientContext",
							).Call(Id("cmd"))
						} else {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Id("GetClientContext").Call(Id("cmd"))
						}
						g.Line()
						g.Comment(fmt.Sprintf(
							"read %s config",
							cmdStrHuman,
						))
						g.Id("configContent").Op(",").Err().Op(":=").Qual("os", "ReadFile").Call(
							Id(configPathVar),
						)
						g.If(Err().Op("!=").Nil()).Block(
							Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"Error",
							).Call(Lit("failed to read config file"), Err()),
							Qual("os", "Exit").Call(Lit(1)),
						)
						g.Comment(fmt.Sprintf("create %s based on version", cmdStrHuman))
						g.Switch().Id(versionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Var().Id(objectConfigVar).Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									),
									If(Err().Op(":=").Qual(
										"gopkg.in/yaml.v2",
										"UnmarshalStrict",
									).Call(Id("configContent"), Op("&").Id(objectConfigVar)), Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit("failed to unmarshal config file yaml content"), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Comment(fmt.Sprintf(
										"create %s",
										cmdStrHuman,
									)),
									Add(setConfigPath),
									Id(createdObjVar).Op(",").Err().Op(":=").Id(objectConfigVar).Dot("Create").Call(
										Id("apiClient"), Id("apiEndpoint"),
									),
									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit(fmt.Sprintf(
											"failed to create %s",
											cmdStrHuman,
										)), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Complete",
									).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
										"%s %%s created",
										cmdStrHuman,
									)), Op("*").Id(createdObjVar).Dot(apiObj.TypeName).Dot("Name"))),
								)
								h.Default().Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(
										Lit(""),
										Qual("errors", "New").Call(
											Lit("unrecognized object version"),
										),
									),
									Qual("os", "Exit").Call(Lit(1)),
								)
							}
						})
					}),
				})

				commandCode.Func().Id("init").Params().Block(
					Id("CreateCmd").Dot("AddCommand").Call(Id(createCmdVar)),
					Line(),
					Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(configPathVar),
						Line().Lit("config"),
						Lit("c"),
						Lit(""),
						Lit(fmt.Sprintf(
							"Path to file with %s config.",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(createCmdVar).Dot("MarkFlagRequired").Call(Lit("config")),
					Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(versionVar),
						Line().Lit("version"),
						Lit("v"),
						Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s object to create. One of: %s",
							pluralize.Pluralize(cmdStrHuman, 2, false),
							apiObj.Versions,
						)),
						Line(),
					),
				)

				// replace command
				commandCode.Comment(fmt.Sprintf(
					"%s represents the command '%s replace %s'",
					replaceCmdVar,
					exampleCmdStr,
					cmdStr,
				))
				commandCode.Var().Id(replaceCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  # replace using a config file\n  %s replace %s --config path/to/config.yaml --name some-%s",
						exampleCmdStr,
						cmdStr,
						cmdStr,
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Replace an existing %s",
						cmdStrHuman,
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Replace an existing %s.\n Note that the entire object will replaced with a PUT request.\n All fields must be provided in the config file.",
						cmdStrHuman,
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"):       Id("CommandPreRunFunc"),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Module {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Qual(
								"github.com/threeport/threeport/cmd/tptctl/cmd",
								"GetClientContext",
							).Call(Id("cmd"))
						} else {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Id("GetClientContext").Call(Id("cmd"))
						}
						g.Line()
						g.Comment(fmt.Sprintf("replace %s based on version", cmdStrHuman))
						g.Switch().Id(versionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Var().Id(objectConfigVar).Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									),
									Comment(fmt.Sprintf(
										"load %s config",
										cmdStrHuman,
									)),
									List(
										Id("configContent"),
										Err(),
									).Op(":=").Qual("os", "ReadFile").Call(Id(configPathVar)),
									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit("failed to read config file"), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									If(Err().Op(":=").Qual(
										"gopkg.in/yaml.v2",
										"UnmarshalStrict",
									).Call(
										Id("configContent"),
										Op("&").Id(objectConfigVar),
									), Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit("failed to unmarshal config file yaml content"), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Comment(fmt.Sprintf(
										"replace %s",
										cmdStrHuman,
									)),
									Add(setConfigPath),
									Id(updatedObjVar).Op(",").Err().Op(":=").Id(objectConfigVar).Dot("Replace").Call(
										Id("apiClient"), Id("apiEndpoint"), Id(nameVar),
									),
									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit(fmt.Sprintf(
											"failed to update %s",
											cmdStrHuman,
										)), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Complete",
									).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
										"%s %%s updated",
										cmdStrHuman,
									)), Op("*").Id(updatedObjVar).Dot(apiObj.TypeName).Dot("Name"))),
								)
								h.Default().Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(
										Lit(""),
										Qual("errors", "New").Call(
											Lit("unrecognized object version"),
										),
									),
									Qual("os", "Exit").Call(Lit(1)),
								)
							}
						})
					}),
				})

				commandCode.Func().Id("init").Params().Block(
					Id("ReplaceCmd").Dot("AddCommand").Call(Id(replaceCmdVar)),
					Line(),
					Id(replaceCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(configPathVar),
						Line().Lit("config"),
						Lit("c"),
						Lit(""),
						Lit(fmt.Sprintf("Path to file with %s config.  The config file must be a complete config, i.e. the provided config will be used to replace the entire existing config for the object with a PUT request.", cmdStrHuman)),
						Line(),
					),
					Id(replaceCmdVar).Dot("MarkFlagRequired").Call(Lit("config")),
					Id(replaceCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(nameVar),
						Line().Lit("name"),
						Lit("n"),
						Lit(""),
						Lit(fmt.Sprintf("Name of existing %s to replace.  If the name in the %[1]s config is different from the name provided here, the name of the existing object will be updated with the name in the config.", cmdStrHuman)),
						Line(),
					),
					Id(replaceCmdVar).Dot("MarkFlagRequired").Call(Lit("name")),
					Id(replaceCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(replaceCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(versionVar),
						Line().Lit("version"),
						Lit("v"),
						Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s object to replace. One of: %s",
							pluralize.Pluralize(cmdStrHuman, 2, false),
							apiObj.Versions,
						)),
						Line(),
					),
				)

				// delete command
				// for models that use configs that reference other files the config
				// path variable must be set on the config object
				setConfigPath = &Statement{}
				if apiObj.TptctlConfigPath {
					setConfigPath.Id(objectVar).Dot(configPathField).Op("=").Op("&").Id(configPathVar)
				}

				commandCode.Comment(fmt.Sprintf(
					"%s represents the command '%s delete %s'",
					deleteCmdVar,
					exampleCmdStr,
					cmdStr,
				))
				commandCode.Var().Id(deleteCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  # delete using a config file\n  %[1]s delete %[2]s --config path/to/config.yaml\n\n  # delete using name\n  %[1]s delete %[2]s --name some-%[2]s",
						exampleCmdStr,
						cmdStr,
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Delete an existing %s",
						cmdStrHuman,
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Delete an existing %s.",
						cmdStrHuman,
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"):       Id("CommandPreRunFunc"),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Module {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Qual(
								"github.com/threeport/threeport/cmd/tptctl/cmd",
								"GetClientContext",
							).Call(Id("cmd"))
						} else {
							g.List(
								Id("apiClient"),
								Id("_"),
								Id("apiEndpoint"),
								Id("_"),
							).Op(":=").Id("GetClientContext").Call(Id("cmd"))
						}
						g.Line()
						g.Comment("flag validation")
						g.If(Err().Op(":=").Qual(
							"github.com/threeport/threeport/pkg/cli/v0",
							"ValidateConfigNameFlags",
						).Call(
							Line().Id(configPathVar),
							Line().Id(nameVar),
							Line().Lit(cmdStrHuman),
							Line(),
						), Err().Op("!=").Nil()).Block(
							Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"Error",
							).Call(Lit("flag validation failed"), Err()),
							Qual("os", "Exit").Call(Lit(1)),
						)
						g.Line()
						g.Comment(fmt.Sprintf("delete %s based on version", cmdStrHuman))
						g.Switch().Id(versionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Var().Id(objectConfigVar).Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									),
									If(Id(configPathVar).Op("!=").Lit("")).Block(
										Comment(fmt.Sprintf(
											"load %s config",
											cmdStrHuman,
										)),
										List(
											Id("configContent"),
											Err(),
										).Op(":=").Qual("os", "ReadFile").Call(Id(configPathVar)),
										If(Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit("failed to read config file"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										If(Err().Op(":=").Qual(
											"gopkg.in/yaml.v2",
											"UnmarshalStrict",
										).Call(
											Id("configContent"),
											Op("&").Id(objectConfigVar),
										), Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit("failed to unmarshal config file yaml content"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
									).Else().Block(
										Id(objectConfigVar).Op("=").Qual(
											fmt.Sprintf("%s%s", configImportPath, version),
											objectConfigObj,
										).Values(Dict{
											Line().Id(apiObj.TypeName): Qual(
												fmt.Sprintf("%s%s", configImportPath, version),
												objectValuesObj,
											).Values(Dict{
												Line().Id("Name"): Op("&").Id(nameVar).Op(",").Line(),
											}).Op(",").Line(),
										}),
									),
									Line(),
									Comment(fmt.Sprintf(
										"delete %s",
										cmdStrHuman,
									)),
									Add(setConfigPath),
									Id(deletedObjVar).Op(",").Err().Op(":=").Id(objectConfigVar).Dot("Delete").Call(
										Id("apiClient"), Id("apiEndpoint"),
									),
									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit(fmt.Sprintf(
											"failed to delete %s",
											cmdStrHuman,
										)), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Line(),
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Complete",
									).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
										"%s %%s deleted",
										cmdStrHuman,
									)), Op("*").Id(deletedObjVar).Dot(apiObj.TypeName).Dot("Name"))),
								)
								h.Default().Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(
										Lit(""),
										Qual("errors", "New").Call(
											Lit("unrecognized object version"),
										),
									),
									Qual("os", "Exit").Call(Lit(1)),
								)
							}
						})
					}),
				})

				commandCode.Func().Id("init").Params().Block(
					Id("DeleteCmd").Dot("AddCommand").Call(Id(deleteCmdVar)),
					Line(),
					Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(configPathVar),
						Line().Lit("config"),
						Lit("c"),
						Lit(""),
						Lit(fmt.Sprintf(
							"Path to file with %s config.",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(nameVar),
						Line().Lit("name"),
						Lit("n"),
						Lit(""),
						Lit(fmt.Sprintf(
							"Name of %s.",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id("cliArgs").Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(versionVar),
						Line().Lit("version"),
						Lit("v"),
						Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s object to delete. One of: %s",
							pluralize.Pluralize(cmdStrHuman, 2, false),
							apiObj.Versions,
						)),
						Line(),
					),
				)
			}
		}

		for _, apiObj := range apiObjGroup.ApiObjects {
			if apiObj.TptctlCommands {
				// defined instance get command output function
				if apiObj.DefinedInstanceDefinition {
					rootObj := strings.TrimSuffix(apiObj.TypeName, "Definition")
					rootObjectVar := strcase.ToLowerCamel(rootObj)
					rootCmdStr := strcase.ToKebab(rootObj)
					getCmdOutputFunc := fmt.Sprintf(
						"outputGet%s%sCmd",
						apiObj.Version,
						pluralize.Pluralize(rootObj, 2, false),
					)

					getOutputCode.Commentf(
						"%s produces the tabular output for the",
						getCmdOutputFunc,
					)
					getOutputCode.Commentf(
						"'get %s' command.",
						pluralize.Pluralize(rootCmdStr, 2, false),
					)
					getOutputCode.Func().Id(getCmdOutputFunc).Params(
						Line().Id(pluralize.Pluralize(rootObjectVar, 2, false)).Op("*").Index().Qual(
							fmt.Sprintf("%s%s", configImportPath, apiObj.Version),
							fmt.Sprintf("%sConfig", rootObj),
						),
						Line(),
					).Error().Block(
						Id("writer").Op(":=").Qual("text/tabwriter", "NewWriter").Call(
							Qual("os", "Stdout"), Lit(4), Lit(4), Lit(4), LitRune(' '), Lit(0),
						),
						Comment("TODO: add columns for each field that users should see"),
						Commentf(
							"TODO: available fields are defined in the %sValues object in pkg/config/%s/%s.go",
							rootObj,
							apiObj.Version,
							strcase.ToSnake(rootObj),
						),
						Qual("fmt", "Fprintln").Call(Id("writer"), Lit("NAME\t AGE")),
						For(
							List(Id("_"), Id(rootObjectVar)).Op(":=").Range().Op("*").Id(pluralize.Pluralize(rootObjectVar, 2, false)).Block(
								Qual("fmt", "Fprintln").Call(
									Line().Id("writer"),
									Line().Op("*").Id(rootObjectVar).Dot(rootObj).Dot("Name").Op(",").Lit("\t"),
									Line().Op("*").Id(rootObjectVar).Dot(rootObj).Dot("Age").Op(",").Line(),
								),
							),
						),
						Id("writer").Dot("Flush").Call(),
						Line(),
						Return(Nil()),
					)
					Line()
				}

				// API object get command output function
				objectVar := strcase.ToLowerCamel(apiObj.TypeName)
				cmdStr := strcase.ToKebab(apiObj.TypeName)
				getCmdOutputFunc := fmt.Sprintf(
					"outputGet%s%sCmd",
					apiObj.Version,
					pluralize.Pluralize(apiObj.TypeName, 2, false),
				)

				getOutputCode.Commentf(
					"%s produces the tabular output for the",
					getCmdOutputFunc,
				)
				getOutputCode.Commentf(
					"`get %s` command.",
					pluralize.Pluralize(cmdStr, 2, false),
				)
				getOutputCode.Func().Id(getCmdOutputFunc).Params(
					Line().Id(pluralize.Pluralize(objectVar, 2, false)).Op("*").Index().Qual(
						fmt.Sprintf("%s%s", configImportPath, apiObj.Version),
						fmt.Sprintf("%sConfig", apiObj.TypeName),
					),
					Line(),
				).Error().Block(
					Id("writer").Op(":=").Qual("text/tabwriter", "NewWriter").Call(
						Qual("os", "Stdout"), Lit(4), Lit(4), Lit(4), LitRune(' '), Lit(0),
					),
					Comment("TODO: add columns for each field that users should see"),
					Commentf(
						"TODO: available fields are defined in the %sValues object in pkg/config/%s/%s.go",
						apiObj.TypeName,
						apiObj.Version,
						strcase.ToSnake(apiObj.TypeName),
					),
					Qual("fmt", "Fprintln").Call(Id("writer"), Lit("NAME\t AGE")),
					For(List(
						Id("_"),
						Id(objectVar),
					).Op(":=").Range().Op("*").Id(pluralize.Pluralize(objectVar, 2, false))).Block(
						Qual("fmt", "Fprintln").Call(
							Line().Id("writer"),
							Line().Op("*").Id(objectVar).Dot(apiObj.TypeName).Dot("Name").Op(",").Lit("\t"),
							Line().Op("*").Id(objectVar).Dot(apiObj.TypeName).Dot("Age").Op(",").Line(),
						),
					),
					Id("writer").Dot("Flush").Call(),
					Line(),
					Return(Nil()),
				)
				Line()
			}
		}

		if commandsGenerated {
			commandsDir := filepath.Join("cmd", "tptctl", "cmd")
			if gen.Module {
				commandsDir = filepath.Join("cmd", strcase.ToSnake(sdkConfig.ModuleName), "cmd")
			}
			// write commands code to file if it doesn't already exist and not excluded by SDK config
			genFilepath := filepath.Join(
				commandsDir,
				fmt.Sprintf("%s.go", util.FilenameSansExt(apiObjGroup.ModelFilename)),
			)
			if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
				cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
			} else {
				fileWritten, err := util.WriteCodeToFile(commandCode, genFilepath, false)
				if err != nil {
					return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
				}
				if fileWritten {
					cli.Info(fmt.Sprintf(
						"source code for %s tptctl commands written to %s",
						apiObjGroup.ControllerDomainLower,
						genFilepath,
					))
				} else {
					cli.Info(fmt.Sprintf(
						"source code for %s tptctl commands already exists at %s - not overwritten",
						apiObjGroup.ControllerDomainLower,
						genFilepath,
					))
				}
			}

			// write get output code to file if it doesn't already exist and not excluded by SDK config
			genFilepath = filepath.Join(
				commandsDir,
				fmt.Sprintf("%s_get_output.go", util.FilenameSansExt(apiObjGroup.ModelFilename)),
			)
			if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
				cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
			} else {
				fileWritten, err := util.WriteCodeToFile(getOutputCode, genFilepath, false)
				if err != nil {
					return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
				}
				if fileWritten {
					cli.Info(fmt.Sprintf(
						"source code for %s tptctl get command output written to %s",
						apiObjGroup.ControllerDomainLower,
						genFilepath,
					))
				} else {
					cli.Info(fmt.Sprintf(
						"source code for %s tptctl get command output already exists at %s - not overwritten",
						apiObjGroup.ControllerDomainLower,
						genFilepath,
					))
				}
			}
		} else if apiObjGroup.ControllerDomainLower != "" {
			cli.Info(fmt.Sprintf(
				"no tptctl commands generated for %s",
				apiObjGroup.ControllerDomainLower,
			))
		}
	}

	return nil
}
