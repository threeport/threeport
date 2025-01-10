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

// GenPluginCrudCmds generates the create, delete, describe, get and update
// commands for an extension's tptctl plugin.
func GenPluginCrudCmds(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	crudCmds := []string{
		"create",
		"delete",
		"describe",
		"get",
		"update",
	}
	for _, crudCmd := range crudCmds {
		crudCmdUpper := strcase.ToCamel(crudCmd)

		f := NewFile("cmd")
		f.HeaderComment(util.HeaderCommentGenMod)

		f.Comment(fmt.Sprintf("%sCmd represents the %s command", crudCmdUpper, crudCmd))
		f.Var().Id(
			fmt.Sprintf("%sCmd", crudCmdUpper),
		).Op("=").Op("&").Qual("github.com/spf13/cobra", "Command").Values(Dict{
			Id("Use"): Lit(crudCmd),
			Id("Short"): Lit(fmt.Sprintf(
				"%s a Threeport %s object",
				crudCmdUpper,
				sdkConfig.ExtensionName,
			)),
			Id("Long"): Lit(fmt.Sprintf(`%s a Threeport %s object.

	The %s command does nothing by itself.  Use one of the available subcommands
	to %[3]s different objects from the system.`, crudCmdUpper, sdkConfig.ExtensionName, crudCmd)),
		})

		f.Line()

		f.Func().Id("init").Params().Block(
			Id("rootCmd").Dot("AddCommand").Call(Id(fmt.Sprintf("%sCmd", crudCmdUpper))),
		)

		// write code to file if it doesn't already exist
		genFilepath := filepath.Join(
			"cmd",
			strcase.ToSnake(sdkConfig.ExtensionName),
			"cmd",
			fmt.Sprintf("%s.go", crudCmd),
		)
		fileWritten, err := util.WriteCodeToFile(f, genFilepath, false)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		if fileWritten {
			cli.Info(fmt.Sprintf(
				"source code for %s command written to %s",
				crudCmd,
				genFilepath,
			))
		} else {
			cli.Info(fmt.Sprintf(
				"source code for %s command already exists at %s - not overwritten",
				crudCmd,
				genFilepath,
			))
		}
	}

	return nil
}
