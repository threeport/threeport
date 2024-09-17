package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenCliCommands generates commands for the tptctl CLI tool.
func GenCliCommands(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	// used to configure dynamically-generated function call parameters
	multiLineCall := Options{
		Close:     ")",
		Multi:     true,
		Open:      "(",
		Separator: ",",
	}

	// used to configure dynamically-generated function signature parameters
	multiLineParams := Options{
		Close:     "",
		Multi:     true,
		Open:      "",
		Separator: ",",
	}

	// set values for threeport and extensions where different
	exampleCmdStr := "tptctl"
	cliArgsVar := "cliArgs"
	if gen.Extension {
		exampleCmdStr = fmt.Sprintf("tptctl %s", strcase.ToKebab(sdkConfig.ExtensionName))
		cliArgsVar = "CliArgs"
	}

	// set import paths for threeport and extensions where different
	apiImportPath := "github.com/threeport/threeport/pkg/api/"
	clientImportPath := "github.com/threeport/threeport/pkg/client/"
	configImportPath := "github.com/threeport/threeport/pkg/config/"
	if gen.Extension {
		apiImportPath = fmt.Sprintf("%s/pkg/api/", gen.ModulePath)
		clientImportPath = fmt.Sprintf("%s/pkg/client/", gen.ModulePath)
		configImportPath = fmt.Sprintf("%s/pkg/config/", gen.ModulePath)
	}

	for _, apiObjGroup := range gen.ApiObjectGroups {
		// commandCode contains the standard tptctl commands for a threeport object
		commandCode := NewFile("cmd")
		commandCode.HeaderComment(util.HeaderCommentGenNoEdit)
		commandCode.ImportAlias("gopkg.in/yaml.v2", "yaml")
		commandCode.ImportAlias("github.com/ghodss/yaml", "ghodss_yaml")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/cli/v0", "cli")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/encryption/v0", "encryption")
		commandCode.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
		if gen.Extension {
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

		// describeOutputCode contains the customized output for `tptctl describe` commands
		// this file is written if it doesn't exist, otherwise is left for developer
		// customization
		describeOutputCode := NewFile("cmd")
		describeOutputCode.HeaderComment(util.HeaderCommentGenMod)
		describeOutputCode.ImportAlias("github.com/threeport/threeport/pkg/cli/v0", "cli")

		// no code will be generated if tptctl is not enabled on API
		// model
		commandsGenerated := false
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
				for _, version := range apiObj.Versions {
					describeOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", apiImportPath, version),
						fmt.Sprintf("api_%s", version),
					)
					describeOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", clientImportPath, version),
						fmt.Sprintf("client_%s", version),
					)
					describeOutputCode.ImportAlias(
						fmt.Sprintf("%s%s", configImportPath, version),
						fmt.Sprintf("config_%s", version),
					)
				}

				// commands for defined instance abstractions
				if apiObj.DefinedInstanceInstance {
					rootObj := strings.TrimSuffix(apiObj.TypeName, "Instance")
					rootCmdStr := strcase.ToKebab(rootObj)
					rootCmdStrHuman := strcase.ToDelimited(rootObj, ' ')
					rootObjectVar := strcase.ToLowerCamel(rootObj)
					rootObjectConfigVar := fmt.Sprintf("%sConfig", rootObjectVar)
					instanceObj := rootObj + "Instance"
					instanceVar := rootObjectVar + "Instance"
					instanceHuman := strcase.ToDelimited(instanceVar, ' ')

					commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
					commandCode.Comment(rootObj)
					commandCode.Comment("///////////////////////////////////////////////////////////////////////////////")
					commandCode.Line()

					// defined instance get command
					getCmdVar := fmt.Sprintf("Get%sCmd", pluralize.Pluralize(rootObj, 2, false))
					getClientFunc := fmt.Sprintf("Get%s%s", rootObj, "Instances")
					getCmdOutputFunc := fmt.Sprintf("output%s", getCmdVar)

					commandCode.Comment(fmt.Sprintf(
						"%s represents the %s command",
						getCmdVar,
						rootCmdStr,
					))
					commandCode.Var().Id(getCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"): Lit(pluralize.Pluralize(rootCmdStr, 2, false)),
						Id("Example"): Lit(fmt.Sprintf(
							"  %s get %s",
							exampleCmdStr,
							pluralize.Pluralize(rootCmdStr, 2, false),
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Get %s from the system",
							pluralize.Pluralize(rootCmdStrHuman, 2, false),
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Get %s from the system.\n\nA %[2]s is a simple abstraction of %[2]s definitions and %[2]s instances.\nThis command displays all instances and the definitions used to configure them.",
							pluralize.Pluralize(rootCmdStrHuman, 2, false),
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"): util.QualifiedOrLocal(
							gen.Extension,
							"github.com/threeport/threeport/cmd/tptctl/cmd",
							"CommandPreRunFunc",
						),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Extension {
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
							g.Comment(fmt.Sprintf(
								"get %s",
								pluralize.Pluralize(rootCmdStrHuman, 2, false),
							))
							for _, version := range apiObj.Versions {
								g.List(Id(
									fmt.Sprintf("%s%s", version, pluralize.Pluralize(instanceVar, 2, false)),
								), Err()).Op(":=").Qual(
									fmt.Sprintf("%s%s", clientImportPath, version),
									getClientFunc,
								).Call(Id("apiClient"), Id("apiEndpoint"))
								g.If(Err().Op("!=").Nil()).Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(Lit(fmt.Sprintf(
										"failed to retrieve %s instances",
										rootCmdStrHuman,
									)), Err()),
									Qual("os", "Exit").Call(Lit(1)),
								)
								g.Line()
							}
							g.Line()
							g.Comment("write the output")
							objLenCheck := &Statement{}
							for i, version := range apiObj.Versions {
								objLenCheck.Len(Op("*").Id(
									fmt.Sprintf("%s%s", version, pluralize.Pluralize(instanceVar, 2, false)),
								)).Op("==").Lit(0)
								if i < len(apiObj.Versions)-1 {
									objLenCheck.Op("&&")
								}
							}
							g.If(objLenCheck).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Info",
								).Call(Qual("fmt", "Sprintf").Call(
									Line().Lit(fmt.Sprintf(
										"No %s currently managed by %%s threeport control plane",
										pluralize.Pluralize(instanceHuman, 2, false),
									)),
									Line().Id("requestedControlPlane").Op(",").Line(),
								)),
								Qual("os", "Exit").Call(Lit(0)),
							)
							g.If(
								Err().Op(":=").Id(getCmdOutputFunc).CustomFunc(multiLineCall, func(h *Group) {
									for _, version := range apiObj.Versions {
										h.Id(
											fmt.Sprintf("%s%s", version, pluralize.Pluralize(instanceVar, 2, false)),
										)
									}
									h.Id("apiClient")
									h.Id("apiEndpoint")
								}),
								Err().Op("!=").Nil(),
							).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit("failed to produce output: %s"), Err()),
								Qual("os", "Exit").Call(Lit(0)),
							)
						}),
					})

					commandCode.Func().Id("init").Params().Block(
						Id("GetCmd").Dot("AddCommand").Call(Id(getCmdVar)),
						Line(),
						Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"),
							Lit("i"),
							Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
					)

					// defined instance create command
					createCmdVar := fmt.Sprintf("Create%sCmd", rootObj)
					objectConfigObj := fmt.Sprintf("%sConfig", rootObj)
					createConfigPathVar := fmt.Sprintf("create%sConfigPath", rootObj)
					configPathField := fmt.Sprintf("%sConfigPath", apiObjGroup.ControllerDomain)
					createdDefObjVar := fmt.Sprintf("created%sDefinition", rootObj)
					createdInstObjVar := fmt.Sprintf("created%sInstance", rootObj)
					createDefInstVersionVar := fmt.Sprintf("create%sVersion", rootObj)

					// for models that use configs that reference other files the config
					// path variable must be set on the config object
					setConfigPath := &Statement{}
					if apiObj.TptctlConfigPath || apiObj.DefinedInstanceTptctlConfigPath {
						setConfigPath.Id(rootObjectVar).Dot(configPathField).Op("=").Id(createConfigPathVar)
					}

					commandCode.Var().Defs(
						Id(createConfigPathVar).String(),
						Id(createDefInstVersionVar).String(),
					)

					commandCode.Comment(fmt.Sprintf(
						"%s represents the %s command",
						createCmdVar,
						rootCmdStr,
					))
					commandCode.Var().Id(createCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"): Lit(rootCmdStr),
						Id("Example"): Lit(fmt.Sprintf(
							"  %s create %s --config path/to/config.yaml",
							exampleCmdStr,
							rootCmdStr,
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Create a new %s",
							rootCmdStrHuman,
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Create a new %[1]s. This command creates a new %[1]s definition and %[1]s instance based on the %[1]s config.",
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"): util.QualifiedOrLocal(
							gen.Extension,
							"github.com/threeport/threeport/cmd/tptctl/cmd",
							"CommandPreRunFunc",
						),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Extension {
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
								Id(createConfigPathVar),
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
							g.Switch(Id(createDefInstVersionVar)).BlockFunc(func(h *Group) {
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
										Id(rootObjectVar).Op(":=").Id(rootObjectConfigVar).Dot(rootObj),
										Add(setConfigPath),
										Id(createdDefObjVar).Op(",").Id(createdInstObjVar).Op(",").Err().Op(":=").Id(rootObjectVar).Dot("Create").Call(
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
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s definition %%s created",
											rootCmdStrHuman,
										)), Op("*").Id(createdDefObjVar).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s instance %%s created",
											rootCmdStrHuman,
										)), Op("*").Id(createdInstObjVar).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Complete",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s %%s created",
											rootCmdStrHuman,
										)), Id(rootObjectConfigVar).Dot(rootObj).Dot("Name"))),
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
							Line().Op("&").Id(createConfigPathVar),
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
							Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"),
							Lit("i"),
							Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
						Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(createDefInstVersionVar),
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
					deleteConfigPathVar := fmt.Sprintf("delete%sConfigPath", rootObj)
					deleteNameVar := fmt.Sprintf("delete%sName", rootObj)
					deleteDefInstVersionVar := fmt.Sprintf("delete%sVersion", rootObj)

					// for models that use configs that reference other files the config
					// path variable must be set on the config object
					setConfigPath = &Statement{}
					if apiObj.TptctlConfigPath {
						setConfigPath.Id(rootObjectVar).Dot(configPathField).Op("=").Id(deleteConfigPathVar)
					}

					commandCode.Var().Defs(
						Id(deleteConfigPathVar).String(),
						Id(deleteNameVar).String(),
						Id(deleteDefInstVersionVar).String(),
					)

					commandCode.Comment(fmt.Sprintf(
						"%s represents the %s command",
						deleteCmdVar,
						rootCmdStr,
					))
					commandCode.Var().Id(deleteCmdVar).Op("=").Op("&").Qual(
						"github.com/spf13/cobra",
						"Command",
					).Values(Dict{
						Id("Use"): Lit(rootCmdStr),
						Id("Example"): Lit(fmt.Sprintf(
							"  # delete based on config file\n  %[1]s delete %[2]s --config path/to/config.yaml\n\n  # delete based on name\n  %[1]s delete %[2]s --name some-%[2]s",
							exampleCmdStr,
							rootCmdStr,
						)),
						Id("Short"): Lit(fmt.Sprintf(
							"Delete an existing %s",
							rootCmdStrHuman,
						)),
						Id("Long"): Lit(fmt.Sprintf(
							"Delete an existing %[1]s. This command deletes an existing %[1]s definition and %[1]s instance based on the %[1]s config.",
							rootCmdStrHuman,
						)),
						Id("SilenceUsage"): True(),
						Id("PreRun"): util.QualifiedOrLocal(
							gen.Extension,
							"github.com/threeport/threeport/cmd/tptctl/cmd",
							"CommandPreRunFunc",
						),
						Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
							"github.com/spf13/cobra",
							"Command",
						), Id("args").Index().String()).BlockFunc(func(g *Group) {
							if gen.Extension {
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
							g.If(Id(deleteConfigPathVar)).Op("==").Lit("").Block(
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
							).Op(":=").Qual("os", "ReadFile").Call(Id(deleteConfigPathVar))
							g.If(Err().Op("!=").Nil()).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit("failed to read config file"), Err()),
								Qual("os", "Exit").Call(Lit(1)),
							)
							g.Line()
							g.Comment(fmt.Sprintf("delete %s based on version", rootCmdStrHuman))
							g.Switch().Id(deleteDefInstVersionVar).BlockFunc(func(h *Group) {
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
										Id(rootObjectVar).Op(":=").Id(rootObjectConfigVar).Dot(rootObj),
										Add(setConfigPath),
										Id("_").Op(",").Id("_").Op(",").Err().Op("=").Id(rootObjectVar).Dot("Delete").Call(
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
										)), Id(rootObjectVar).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s instance %%s deleted",
											rootCmdStrHuman,
										)), Id(rootObjectVar).Dot("Name"))),
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Complete",
										).Call(Qual("fmt", "Sprintf").Call(Lit(fmt.Sprintf(
											"%s %%s deleted",
											rootCmdStrHuman,
										)), Id(rootObjectConfigVar).Dot(rootObj).Dot("Name"))),
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
							Line().Op("&").Id(deleteConfigPathVar),
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
							Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
							Line().Lit("control-plane-name"),
							Lit("i"),
							Lit(""),
							Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
							Line(),
						),
						Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
							Line().Op("&").Id(deleteDefInstVersionVar),
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

					// defined instance get command output function
					getOutputCode.Commentf(
						"%s produces the tabular output for the",
						getCmdOutputFunc,
					)
					getOutputCode.Commentf(
						"'tptctl get %s' command.",
						pluralize.Pluralize(rootCmdStr, 2, false),
					)
					objParams := &Statement{}
					objParams.CustomFunc(multiLineParams, func(g *Group) {
						for _, version := range apiObj.Versions {
							g.Id(fmt.Sprintf(
								"%s%s",
								version,
								pluralize.Pluralize(instanceVar, 2, false)),
							).Op("*").Index().Qual(
								fmt.Sprintf("%s%s", apiImportPath, version),
								instanceObj,
							)
						}
					})

					getOutputCode.Func().Id(getCmdOutputFunc).Params(
						objParams,
						Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
						Line().Id("apiEndpoint").String(),
						Line(),
					).Error().BlockFunc(func(g *Group) {
						g.Id("writer").Op(":=").Qual("text/tabwriter", "NewWriter").Call(
							Qual("os", "Stdout"), Lit(4), Lit(4), Lit(4), LitRune(' '), Lit(0),
						)
						g.Qual("fmt", "Fprintln").Call(Id("writer"), Lit("VERSION\t NAME\t AGE"))
						for _, version := range apiObj.Versions {
							g.For(
								List(Id("_"), Id(instanceVar)).Op(":=").Range().Op("*").Id(
									fmt.Sprintf("%s%s", version, pluralize.Pluralize(instanceVar, 2, false)),
								)).Block(
								Qual("fmt", "Fprintln").Call(
									Line().Id("writer"),
									Line().Lit(version).Op(",").Lit("\t"),
									Line().Op("*").Id(instanceVar).Dot("Name").Op(",").Lit("\t"),
									Line().Qual(
										"github.com/threeport/threeport/pkg/util/v0",
										"GetAge",
									).Call(Id(instanceVar).Dot("CreatedAt")),
									Line(),
								),
							)
						}
						g.Id("writer").Dot("Flush").Call()
						g.Line()
						g.Return(Nil())
					})
					Line()
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

				// get command
				getCmdVar := fmt.Sprintf("Get%sCmd", pluralize.Pluralize(apiObj.TypeName, 2, false))
				getClientFunc := fmt.Sprintf("Get%s", pluralize.Pluralize(apiObj.TypeName, 2, false))
				getObjectVersionVar := fmt.Sprintf("get%sVersion", apiObj.TypeName)

				commandCode.Var().Id(getObjectVersionVar).String()
				commandCode.Line()

				commandCode.Comment(fmt.Sprintf(
					"%s represents the %s command",
					getCmdVar,
					cmdStr,
				))
				commandCode.Var().Id(getCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(pluralize.Pluralize(cmdStr, 2, false)),
					Id("Example"): Lit(fmt.Sprintf(
						"  %s get %s",
						exampleCmdStr,
						pluralize.Pluralize(cmdStr, 2, false),
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Get %s from the system",
						pluralize.Pluralize(cmdStrHuman, 2, false),
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Get %s from the system.",
						pluralize.Pluralize(cmdStrHuman, 2, false),
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"): util.QualifiedOrLocal(
						gen.Extension,
						"github.com/threeport/threeport/cmd/tptctl/cmd",
						"CommandPreRunFunc",
					),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Extension {
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
						g.Switch().Id(getObjectVersionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Comment(fmt.Sprintf(
										"get %s",
										pluralize.Pluralize(cmdStrHuman, 2, false),
									)),
									List(Id(pluralize.Pluralize(objectVar, 2, false)), Err()).Op(":=").Qual(
										fmt.Sprintf("%s%s", clientImportPath, version),
										getClientFunc,
									).Call(Id("apiClient"), Id("apiEndpoint")),
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
									Comment("write the output"),
									If(Len(Op("*").Id(pluralize.Pluralize(objectVar, 2, false))).Op("==").Lit(0)).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Info",
										).Call(Qual("fmt", "Sprintf").Call(
											Line().Lit(fmt.Sprintf(
												"No %s currently managed by %%s threeport control plane",
												pluralize.Pluralize(cmdStrHuman, 2, false),
											)),
											Line().Id("requestedControlPlane").Op(",").Line(),
										)),
										Qual("os", "Exit").Call(Lit(0)),
									),

									If(
										Err().Op(":=").Id(
											fmt.Sprintf(
												"outputGet%s%sCmd",
												version,
												pluralize.Pluralize(apiObj.TypeName, 2, false),
											),
										).Call(
											Line().Id(pluralize.Pluralize(objectVar, 2, false)),
											Line().Id("apiClient"),
											Line().Id("apiEndpoint"),
											Line(),
										),
										Err().Op("!=").Nil(),
									).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit("failed to produce output"), Err()),
										Qual("os", "Exit").Call(Lit(0)),
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
						Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(getCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(getObjectVersionVar),
						Line().Lit("version"),
						Lit("v"),
						Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s object to retrieve. One of: %s",
							pluralize.Pluralize(cmdStrHuman, 2, false),
							apiObj.Versions,
						)),
						Line(),
					),
				)

				// create command
				createCmdVar := fmt.Sprintf("Create%sCmd", apiObj.TypeName)
				createConfigPathVar := fmt.Sprintf("create%sConfigPath", apiObj.TypeName)
				createdObjVar := fmt.Sprintf("created%s", apiObj.TypeName)
				createObjectVersionVar := fmt.Sprintf("create%sVersion", apiObj.TypeName)

				// for models that use configs that reference other files the config
				// path variable must be set on the config object
				setConfigPath := &Statement{}
				if apiObj.TptctlConfigPath {
					setConfigPath.Id(objectVar).Dot(configPathField).Op("=").Id(createConfigPathVar)
				}

				commandCode.Var().Defs(
					Id(createConfigPathVar).String(),
					Id(createObjectVersionVar).String(),
				)

				commandCode.Comment(fmt.Sprintf(
					"%s represents the %s command",
					createCmdVar,
					cmdStr,
				))
				commandCode.Var().Id(createCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  %s create %s --config path/to/config.yaml",
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
					Id("PreRun"): util.QualifiedOrLocal(
						gen.Extension,
						"github.com/threeport/threeport/cmd/tptctl/cmd",
						"CommandPreRunFunc",
					),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Extension {
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
							Id(createConfigPathVar),
						)
						g.If(Err().Op("!=").Nil()).Block(
							Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"Error",
							).Call(Lit("failed to read config file"), Err()),
							Qual("os", "Exit").Call(Lit(1)),
						)
						g.Comment(fmt.Sprintf("create %s based on version", cmdStrHuman))
						g.Switch().Id(createObjectVersionVar).BlockFunc(func(h *Group) {
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
									Id(objectVar).Op(":=").Id(objectConfigVar).Dot(apiObj.TypeName),
									Add(setConfigPath),
									Id(createdObjVar).Op(",").Err().Op(":=").Id(objectVar).Dot("Create").Call(
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
									)), Op("*").Id(createdObjVar).Dot("Name"))),
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
						Line().Op("&").Id(createConfigPathVar),
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
						Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(createCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(createObjectVersionVar),
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

				// delete command
				deleteCmdVar := fmt.Sprintf("Delete%sCmd", apiObj.TypeName)
				deleteConfigPathVar := fmt.Sprintf("delete%sConfigPath", apiObj.TypeName)
				deleteNameVar := fmt.Sprintf("delete%sName", apiObj.TypeName)
				deletedObjVar := fmt.Sprintf("deleted%s", apiObj.TypeName)
				deleteObjectVersionVar := fmt.Sprintf("delete%sVersion", apiObj.TypeName)

				// for models that use configs that reference other files the config
				// path variable must be set on the config object
				setConfigPath = &Statement{}
				if apiObj.TptctlConfigPath {
					setConfigPath.Id(objectVar).Dot(configPathField).Op("=").Id(deleteConfigPathVar)
				}

				commandCode.Var().Defs(
					Id(deleteConfigPathVar).String(),
					Id(deleteNameVar).String(),
					Id(deleteObjectVersionVar).String(),
				)

				commandCode.Comment(fmt.Sprintf(
					"%s represents the %s command",
					deleteCmdVar,
					cmdStr,
				))
				commandCode.Var().Id(deleteCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  # delete based on config file\n  %[1]s delete %[2]s --config path/to/config.yaml\n\n  # delete based on name\n  %[1]s delete %[2]s --name some-%[2]s",
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
					Id("PreRun"): util.QualifiedOrLocal(
						gen.Extension,
						"github.com/threeport/threeport/cmd/tptctl/cmd",
						"CommandPreRunFunc",
					),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Extension {
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
							Line().Id(deleteConfigPathVar),
							Line().Id(deleteNameVar),
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
						g.Switch().Id(deleteObjectVersionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Var().Id(objectConfigVar).Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									),
									If(Id(deleteConfigPathVar).Op("!=").Lit("")).Block(
										Comment(fmt.Sprintf(
											"load %s config",
											cmdStrHuman,
										)),
										List(
											Id("configContent"),
											Err(),
										).Op(":=").Qual("os", "ReadFile").Call(Id(deleteConfigPathVar)),
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
												Line().Id("Name"): Id(deleteNameVar).Op(",").Line(),
											}).Op(",").Line(),
										}),
									),
									Line(),
									Comment(fmt.Sprintf(
										"delete %s",
										cmdStrHuman,
									)),
									Id(objectVar).Op(":=").Id(objectConfigVar).Dot(apiObj.TypeName),
									Add(setConfigPath),
									Id(deletedObjVar).Op(",").Err().Op(":=").Id(objectVar).Dot("Delete").Call(
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
									)), Op("*").Id(deletedObjVar).Dot("Name"))),
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
						Line().Op("&").Id(deleteConfigPathVar),
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
						Line().Op("&").Id(deleteNameVar),
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
						Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(deleteCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(deleteObjectVersionVar),
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

				// describe command
				describeCmdVar := fmt.Sprintf("Describe%sCmd", apiObj.TypeName)
				describeConfigPathVar := fmt.Sprintf("describe%sConfigPath", apiObj.TypeName)
				describeNameVar := fmt.Sprintf("describe%sName", apiObj.TypeName)
				describeFieldVar := fmt.Sprintf("describe%sField", apiObj.TypeName)
				describeOutputVar := fmt.Sprintf("describe%sOutput", apiObj.TypeName)
				jsonObjectVar := fmt.Sprintf("%sJson", objectVar)
				yamlObjectVar := fmt.Sprintf("%sYaml", objectVar)
				redactedObjectVar := fmt.Sprintf("redacted%s", apiObj.TypeName)
				describeObjectVersionVar := fmt.Sprintf("describe%sVersion", apiObj.TypeName)

				commandCode.Var().Defs(
					Id(describeConfigPathVar).String(),
					Id(describeNameVar).String(),
					Id(describeFieldVar).String(),
					Id(describeOutputVar).String(),
					Id(describeObjectVersionVar).String(),
				)
				commandCode.Comment(fmt.Sprintf(
					"%s representes the %s command",
					describeCmdVar,
					cmdStr,
				))
				commandCode.Var().Id(describeCmdVar).Op("=").Op("&").Qual(
					"github.com/spf13/cobra",
					"Command",
				).Values(Dict{
					Id("Use"): Lit(cmdStr),
					Id("Example"): Lit(fmt.Sprintf(
						"  # Get the plain output description for a %[1]s\n  %[2]s describe %[3]s -n some-%[3]s\n\n  # Get JSON output for a %[1]s\n  %[2]s describe %[3]s -n some-%[3]s -o json\n\n  # Get the value of the Name field for a %[1]s\n  %[2]s describe %[3]s -n some-%[3]s -f Name ",
						cmdStrHuman,
						exampleCmdStr,
						cmdStr,
					)),
					Id("Short"): Lit(fmt.Sprintf(
						"Describe a %[1]s",
						cmdStrHuman,
					)),
					Id("Long"): Lit(fmt.Sprintf(
						"Describe a %s.  This command can give you a plain output description, output all fields in JSON or YAML format, or provide the value of any specific field.\n\nNote: any values that are encrypted in the database will be redacted unless the field is specifically requested with the --field flag.",
						cmdStrHuman,
					)),
					Id("SilenceUsage"): True(),
					Id("PreRun"): util.QualifiedOrLocal(
						gen.Extension,
						"github.com/threeport/threeport/cmd/tptctl/cmd",
						"CommandPreRunFunc",
					),
					Id("Run"): Func().Params(Id("cmd").Op("*").Qual(
						"github.com/spf13/cobra",
						"Command",
					), Id("args").Index().String()).BlockFunc(func(g *Group) {
						if gen.Extension {
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
							Line().Id(describeConfigPathVar),
							Line().Id(describeNameVar),
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
						g.If(
							List(Err().Op(":=")).Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"ValidateDescribeOutputFlag",
							).Call(
								Line().Id(describeOutputVar),
								Line().Lit(cmdStrHuman),
								Line(),
							),
							Err().Op("!=").Nil(),
						).Block(
							Qual(
								"github.com/threeport/threeport/pkg/cli/v0",
								"Error",
							).Call(Lit("flag validation failed"), Err()),
							Qual("os", "Exit").Call(Lit(1)),
						)
						g.Line()
						g.Comment(fmt.Sprintf(
							"get %s",
							cmdStrHuman,
						))
						g.Var().Id(objectVar).Interface()
						g.Switch().Id(describeObjectVersionVar).BlockFunc(func(h *Group) {
							for _, version := range apiObj.Versions {
								h.Case(Lit(version)).Block(
									Comment(fmt.Sprintf(
										"load %s config by name or config file",
										cmdStrHuman,
									)),
									Var().Id(objectConfigVar).Qual(
										fmt.Sprintf("%s%s", configImportPath, version),
										objectConfigObj,
									),
									If(Id(describeConfigPathVar).Op("!=").Lit("")).Block(
										List(Id("configContent"), Err()).Op(":=").Qual("os", "ReadFile").Call(
											Id(describeConfigPathVar),
										),
										If(Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit("failed to read config file"), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										),
										If(List(Err()).Op(":=").Qual(
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
												Line().Id("Name"): Id(describeNameVar).Op(",").Line(),
											}).Op(",").Line(),
										}),
									),
									Line(),
									Comment(fmt.Sprintf("get %s object by name", cmdStrHuman)),
									List(Id("obj"), Err()).Op(":=").Qual(
										fmt.Sprintf("%s%s", clientImportPath, version),
										fmt.Sprintf("Get%sByName", apiObj.TypeName),
									).Call(
										Line().Id("apiClient"),
										Line().Id("apiEndpoint"),
										Line().Id(objectConfigVar).Dot(apiObj.TypeName).Dot("Name"),
										Line(),
									),

									If(Err().Op("!=").Nil()).Block(
										Qual(
											"github.com/threeport/threeport/pkg/cli/v0",
											"Error",
										).Call(Lit(fmt.Sprintf(
											"failed to retrieve %s details",
											cmdStrHuman,
										)), Err()),
										Qual("os", "Exit").Call(Lit(1)),
									),
									Id(objectVar).Op("=").Id("obj"),
									Line(),
									Comment("return plain output if requested"),
									If(Id(describeOutputVar).Op("==").Lit("plain")).Block(
										If((Err().Op(":=").Id(
											fmt.Sprintf("outputDescribe%s%sCmd", version, apiObj.TypeName),
										).Params(
											Line().Id(objectVar).Assert(Op("*").Qual(
												fmt.Sprintf("%s%s", apiImportPath, version),
												apiObj.TypeName,
											)),
											Line().Op("&").Id(objectConfigVar),
											Line().Id("apiClient"),
											Line().Id("apiEndpoint"),
											Line(),
										).Op(";").Err().Op("!=").Nil()).Block(
											Qual(
												"github.com/threeport/threeport/pkg/cli/v0",
												"Error",
											).Call(Lit(fmt.Sprintf(
												"failed to describe %s",
												cmdStrHuman,
											)), Err()),
											Qual("os", "Exit").Call(Lit(1)),
										)),
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
						g.Line()

						g.Comment("return field value if specified")
						g.If(Id(describeFieldVar).Op("!=").Lit("")).Block(
							List(Id("fieldVal"), Err()).Op(":=").Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"GetObjectFieldValue",
							).
								Call(
									Line().Id(objectVar),
									Line().Id(describeFieldVar),
									Line(),
								),
							If(Err().Op("!=").Nil()).Block(
								Qual(
									"github.com/threeport/threeport/pkg/cli/v0",
									"Error",
								).Call(Lit(fmt.Sprintf(
									"failed to get field value from %s",
									cmdStrHuman,
								)), Err()),
								Qual("os", "Exit").Call(Lit(1)),
							),
							Line(),
							Comment("decrypt value as needed"),
							List(
								Id("encrypted"),
								Err(),
							).Op(":=").Id("encryption").Dot("IsEncryptedField").Call(
								Id(objectVar),
								Id(describeFieldVar),
							),
							If(Err().Op("!=").Nil()).Block(
								Id("cli").Dot("Error").Call(Lit(""), Err()),
							),
							If(Id("encrypted")).Block(
								Comment("get encryption key from threeport config"),
								List(
									Id("threeportConfig"),
									Id("requestedControlPlane"),
									Err(),
								).Op(":=").Qual(
									"github.com/threeport/threeport/pkg/config/v0",
									"GetThreeportConfig",
								).Call(
									Id(cliArgsVar).Dot("ControlPlaneName"),
								),
								If(Err().Op("!=").Nil()).Block(
									Id("cli").Dot("Error").Call(
										Lit("failed to get threeport config: %w"),
										Err(),
									),
									Qual("os", "Exit").Call(Lit(1)),
								),
								List(
									Id("encryptionKey"),
									Err(),
								).Op(":=").Id("threeportConfig").Dot("GetThreeportEncryptionKey").Call(
									Id("requestedControlPlane"),
								),
								If(Err().Op("!=").Nil()).Block(
									Id("cli").Dot("Error").Call(
										Lit("failed to get encryption key from threeport config: %w"),
										Err(),
									),
									Qual("os", "Exit").Call(Lit(1)),
								),
								Line(),
								Comment("decrypt value for output"),
								List(Id("decryptedVal"), Err()).Op(":=").Id("encryption").Dot("Decrypt").Call(
									Id("encryptionKey"), Id("fieldVal").Dot("String").Call(),
								),
								If(Err().Op("!=").Nil()).Block(
									Id("cli").Dot("Error").Call(Lit("failed to decrypt value: %w"), Err()),
								),
								Qual("fmt", "Println").Call(Id("decryptedVal")),
								Qual("os", "Exit").Call(Lit(0)),
							).Else().Block(
								Qual("fmt", "Println").Call(Id("fieldVal").Dot("Interface").Call()),
								Qual("os", "Exit").Call(Lit(0)),
							),
						)
						g.Line()
						g.Comment("produce json or yaml output if requested")
						g.Switch(Id(describeOutputVar)).Block(
							Case(Lit("json")).Block(
								Comment("redact encrypted values"),
								Id(redactedObjectVar).Op(":=").Qual(
									"github.com/threeport/threeport/pkg/encryption/v0",
									"RedactEncryptedValues",
								).Call(Id(objectVar)),
								Line(),
								Comment("marshal to JSON then print"),
								List(
									Id(jsonObjectVar),
									Err(),
								).Op(":=").Qual("encoding/json", "MarshalIndent").Call(
									Id(redactedObjectVar),
									Lit(""),
									Lit("  "),
								),
								If(Err().Op("!=").Nil()).Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(Lit(fmt.Sprintf(
										"failed to marshal %s into JSON",
										cmdStrHuman,
									)), Err()),
									Qual("os", "Exit").Call(Lit(1)),
								),
								Line(),
								Qual("fmt", "Println").Call(Id("string").Call(Id(jsonObjectVar))),
							),
							Case(Lit("yaml")).Block(
								Comment("redact encrypted values"),
								Id(redactedObjectVar).Op(":=").Qual(
									"github.com/threeport/threeport/pkg/encryption/v0",
									"RedactEncryptedValues",
								).Call(Id(objectVar)),
								Line(),
								Comment("marshal to JSON then convert to YAML - this results in field"),
								Comment("names with correct capitalization vs marshalling directly to YAML"),
								List(
									Id(jsonObjectVar),
									Err(),
								).Op(":=").Qual("encoding/json", "MarshalIndent").Call(
									Id(redactedObjectVar),
									Lit(""),
									Lit("  "),
								),
								If(Err().Op("!=").Nil()).Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(Lit(fmt.Sprintf(
										"failed to marshal %s into JSON",
										cmdStrHuman,
									)), Err()),
									Qual("os", "Exit").Call(Lit(1)),
								),
								List(Id(yamlObjectVar), Err()).Op(":=").Qual(
									"github.com/ghodss/yaml",
									"JSONToYAML",
								).Call(Id(jsonObjectVar)),
								If(Err().Op("!=").Nil()).Block(
									Qual(
										"github.com/threeport/threeport/pkg/cli/v0",
										"Error",
									).Call(Lit(fmt.Sprintf(
										"failed to convert %s JSON to YAML",
										cmdStrHuman,
									)), Err()),
									Qual("os", "Exit").Call(Lit(1)),
								),
								Line(),
								Qual("fmt", "Println").Call(Id("string").Call(Id(yamlObjectVar))),
							),
						)
					}),
				})

				commandCode.Func().Id("init").Params().Block(
					Id("DescribeCmd").Dot("AddCommand").Call(Id(describeCmdVar)),
					Line(),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(describeConfigPathVar),
						Line().Lit("config"),
						Lit("c"),
						Lit(""),
						Lit(fmt.Sprintf(
							"Path to file with %s config.",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(describeNameVar),
						Line().Lit("name"),
						Lit("n"),
						Lit(""),
						Lit(fmt.Sprintf(
							"Name of %s.",
							cmdStrHuman,
						)),
						Line(),
					),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(describeOutputVar),
						Line().Lit("output"),
						Lit("o"),
						Lit("plain"),
						Lit("Output format for object description. One of 'plain','json','yaml'.  Will be ignored if the --field flag is also used.  Plain output produces select details about the object.  JSON and YAML output formats include all direct attributes of the object"),
						Line(),
					),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(describeFieldVar),
						Line().Lit("field"),
						Lit("f"),
						Lit(""),
						Lit("Object field to get value for. If used, --output flag will be ignored.  *Only* the value of the desired field will be returned.  Will not return information on related objects, only direct attributes of the object itself."),
						Line(),
					),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(cliArgsVar).Dot("ControlPlaneName"),
						Line().Lit("control-plane-name"),
						Lit("i"),
						Lit(""),
						Lit("Optional. Name of control plane. Will default to current control plane if not provided."),
						Line(),
					),
					Id(describeCmdVar).Dot("Flags").Call().Dot("StringVarP").Call(
						Line().Op("&").Id(describeObjectVersionVar),
						Line().Lit("version"),
						Lit("v"),
						Lit(util.GetDefaultObjectVersion(apiObj.TypeName)),
						Lit(fmt.Sprintf(
							"Version of %s object to describe. One of: %s",
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
				objectVar := strcase.ToLowerCamel(apiObj.TypeName)
				cmdStr := strcase.ToKebab(apiObj.TypeName)
				objectConfigVar := fmt.Sprintf("%sConfig", objectVar)
				objectConfigObj := fmt.Sprintf("%sConfig", apiObj.TypeName)

				// get command output function
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
					"'tptctl get %s' command.",
					pluralize.Pluralize(cmdStr, 2, false),
				)
				getOutputCode.Func().Id(getCmdOutputFunc).Params(
					Line().Id(pluralize.Pluralize(objectVar, 2, false)).Op("*").Index().Qual(
						fmt.Sprintf("%s%s", apiImportPath, apiObj.Version),
						apiObj.TypeName,
					),
					Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
					Line().Id("apiEndpoint").String(),
					Line(),
				).Error().Block(
					Id("writer").Op(":=").Qual("text/tabwriter", "NewWriter").Call(
						Qual("os", "Stdout"), Lit(4), Lit(4), Lit(4), LitRune(' '), Lit(0),
					),
					Qual("fmt", "Fprintln").Call(Id("writer"), Lit("NAME\t AGE")),
					For(List(
						Id("_"),
						Id(objectVar),
					).Op(":=").Range().Op("*").Id(pluralize.Pluralize(objectVar, 2, false))).Block(
						Qual("fmt", "Fprintln").Call(
							Line().Id("writer"),
							Line().Op("*").Id(objectVar).Dot("Name").Op(",").Lit("\t"),
							Line().Qual(
								"github.com/threeport/threeport/pkg/util/v0",
								"GetAge",
							).Call(Id(objectVar).Dot("CreatedAt")),
							Line(),
						),
					),
					Id("writer").Dot("Flush").Call(),
					Line(),
					Return(Nil()),
				)
				Line()

				// describe command output function
				describeCmdOutputFunc := fmt.Sprintf(
					"outputDescribe%s%sCmd",
					apiObj.Version,
					apiObj.TypeName,
				)
				describeOutputCode.Comment(fmt.Sprintf(
					"%s produces the plain description",
					describeCmdOutputFunc,
				))
				describeOutputCode.Comment(fmt.Sprintf(
					"output for the 'tptctl describe %s' command",
					cmdStr,
				))
				describeOutputCode.Func().Id(describeCmdOutputFunc).Params(
					Line().Id(objectVar).Op("*").Qual(
						fmt.Sprintf("%s%s", apiImportPath, apiObj.Version),
						apiObj.TypeName,
					),
					Line().Id(objectConfigVar).Op("*").Qual(
						fmt.Sprintf("%s%s", configImportPath, apiObj.Version),
						objectConfigObj,
					),
					Line().Id("apiClient").Op("*").Qual("net/http", "Client"),
					Line().Id("apiEndpoint").String(),
					Line(),
				).Error().Block(
					Comment("output describe details"),
					Qual("fmt", "Printf").Call(
						Line().Lit(fmt.Sprintf(
							"* %s Name: %%s\n",
							apiObj.TypeName,
						)),
						Line().Id(objectConfigVar).Dot(apiObj.TypeName).Dot("Name"),
						Line(),
					),
					Qual("fmt", "Printf").Call(
						Line().Lit("* Created: %s\n"),
						Line().Op("*").Id(objectVar).Dot("CreatedAt"),
						Line(),
					),
					Qual("fmt", "Printf").Call(
						Line().Lit("* Last Modified: %s\n"),
						Line().Op("*").Id(objectVar).Dot("UpdatedAt"),
						Line(),
					),
					Line(),
					Return(Nil()),
				)
			}
		}

		if commandsGenerated {
			commandsDir := filepath.Join("cmd", "tptctl", "cmd")
			if gen.Extension {
				commandsDir = filepath.Join("cmd", strcase.ToSnake(sdkConfig.ExtensionName), "cmd")
			}
			// write commands code to file
			genFilepath := filepath.Join(
				commandsDir,
				fmt.Sprintf("%s_gen.go", util.FilenameSansExt(apiObjGroup.ModelFilename)),
			)
			_, err := util.WriteCodeToFile(commandCode, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf(
				"source code for %s tptctl commands written to %s",
				apiObjGroup.ControllerDomainLower,
				genFilepath,
			))

			// write get output code to file if it doesn't already exist
			genFilepath = filepath.Join(
				commandsDir,
				fmt.Sprintf("%s_get_output.go", util.FilenameSansExt(apiObjGroup.ModelFilename)),
			)
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

			// write describe output code to file if it doesn't already exist
			genFilepath = filepath.Join(
				commandsDir,
				fmt.Sprintf("%s_describe_output.go", util.FilenameSansExt(apiObjGroup.ModelFilename)),
			)
			fileWritten, err = util.WriteCodeToFile(describeOutputCode, genFilepath, false)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			if fileWritten {
				cli.Info(fmt.Sprintf(
					"source code for %s tptctl describe command output written to %s",
					apiObjGroup.ControllerDomainLower,
					genFilepath,
				))
			} else {
				cli.Info(fmt.Sprintf(
					"source code for %s tptctl describe command output already exists at %s - not overwritten",
					apiObjGroup.ControllerDomainLower,
					genFilepath,
				))
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
