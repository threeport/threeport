/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package api

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/cmd/sdk/cmd"
	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/mod"
	"github.com/threeport/threeport/internal/sdk/scaffold"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var createConfig string

// createCmd represents the createCmd command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create initial source code scaffolding for Threeport API and its extensions.",
	Long:  `Create initial source code scaffolding for Threeport API and its extensions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// get sdk config
		sdkConfig, err := sdk.GetSdkConfig(createConfig)
		if err != nil {
			cli.Error("failed to get Threeport SDK config", err)
			os.Exit(1)
		}

		// determine if project is threeport/threeport or an extension
		extension, _, err := mod.IsExtension()
		if err != nil {
			return fmt.Errorf("could not determine if creating scaffolding for an extension: %w", err)
		}

		// create internal version package
		if err = scaffold.CreateVersionPackage(); err != nil {
			cli.Error("failed to version package", err)
			os.Exit(1)
		}
		cli.Info("internal version package generated")

		// create API object source code scaffolding
		if err = scaffold.CreateAPIObjects(sdkConfig, extension); err != nil {
			cli.Error("failed to create API objects", err)
			os.Exit(1)
		}
		cli.Info("API object scaffolding generation complete")

		// create component cmd scaffolding
		if err = scaffold.CreateComponentCmd(sdkConfig); err != nil {
			cli.Error("failed to create component cmd scaffolding", err)
			os.Exit(1)
		}
		cli.Info("cmd package for components created")

		// create mage file
		if err = scaffold.CreateMageFile(sdkConfig); err != nil {
			cli.Error("failed to create project mage file", err)
			os.Exit(1)
		}
		cli.Info("project Makefile created")

		cli.Complete("source code scaffolding complete")

		return nil
	},
}

func init() {
	cmd.RootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(
		&createConfig,
		"config", "c", "", "Path to file with Threeport SDK config.",
	)
	createCmd.MarkFlagRequired("config")
}
