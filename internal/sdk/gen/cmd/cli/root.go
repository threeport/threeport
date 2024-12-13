package cli

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenPluginRootCmd generates the root command for an extension's tptctl plugin.
func GenPluginRootCmd(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("cmd")
	f.HeaderComment("generated by 'threeport-sdk gen' but will not be regenerated - intended for modification")

	f.ImportAlias("github.com/threeport/threeport/pkg/cli/v0", "cli")

	packageDir := strcase.ToSnake(sdkConfig.ExtensionName)
	commandVar := fmt.Sprintf("%sCmd", strcase.ToCamel(sdkConfig.ExtensionName))

	f.Var().Id("CliArgs").Op("=").Op("&").Qual(
		"github.com/threeport/threeport/pkg/cli/v0",
		"GenesisControlPlaneCLIArgs",
	).Values()
	f.Line()

	f.Comment(fmt.Sprintf(
		"%s represents the wordpress command which is the root command for",
		commandVar,
	))
	f.Comment("the wordpress plugin.")
	f.Var().Id(commandVar).Op("=").Op("&").Qual(
		"github.com/spf13/cobra",
		"Command",
	).Values(Dict{
		Id("Use"): Lit(strcase.ToKebab(sdkConfig.ExtensionName)),
		Id("Short"): Lit(fmt.Sprintf(
			"Manage the %s Threeport extension",
			sdkConfig.ExtensionName,
		)),
		Id("Long"): Lit(fmt.Sprintf(
			"Manage the %s Threeport extension",
			sdkConfig.ExtensionName,
		)),
	})
	f.Line()

	// write code to file
	genFilepath := filepath.Join(
		"cmd",
		packageDir,
		"cmd",
		"root.go",
	)
	fileWritten, err := util.WriteCodeToFile(f, genFilepath, false)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	if fileWritten {
		cli.Info(fmt.Sprintf("source code for plugin root command written to %s", genFilepath))
	} else {
		cli.Info(fmt.Sprintf("source code for plugin root command already exists at %s - not overwritten", genFilepath))
	}

	return nil
}
