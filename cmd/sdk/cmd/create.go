/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/create"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var createConfig string

// createCmd represents the createCmd command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create source code scaffolding for Threeport API objects.",
	Long: `Create source code scaffolding for Threeport API objects.

Run this command when you have added new API objects and/or versions to the SDK
config.  This command will add the scaffolding for you to define those objects
with their field attributes.

After you have added those field attributes, run 'threeport-sdk gen' to generate
the source code scaffolding and boilerplate for the project.

See the Threeport SDK docs for more information: https://threeport.io/sdk/sdk-intro/
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// get sdk config
		sdkConfig, err := sdk.GetSdkConfig(createConfig)
		if err != nil {
			cli.Error("failed to get valid Threeport SDK config", err)
			os.Exit(1)
		}

		// determine if project is threeport/threeport or an extension
		extension, _, err := util.IsExtension()
		if err != nil {
			return fmt.Errorf("could not determine if creating scaffolding for an extension: %w", err)
		}

		// create API object source code scaffolding
		if err = create.CreateApiObjects(sdkConfig, extension); err != nil {
			cli.Error("failed to create API objects", err)
			os.Exit(1)
		}
		cli.Info("API object scaffolding generation complete")

		cli.Complete("source code scaffolding complete")

		cli.Info(`next add the fields to your API objects in 'pkg/api'.  Then run
'threeport-sdk gen'.`)

		return nil
	},
}

func init() {
	RootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(
		&createConfig,
		"config", "c", "", "Path to file with Threeport SDK config.",
	)
	createCmd.MarkFlagRequired("config")
}
