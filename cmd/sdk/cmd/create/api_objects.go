/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package api

import (
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/sdk"
	manager "github.com/threeport/threeport/internal/sdk/codegen-manager"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var createAPIObjectConfig string

// createAPICmd represents the cmd to create an api-object with the sdk in threeport
var createAPICmd = &cobra.Command{
	Use:   "api-objects",
	Short: "Create initial source code scaffolding for api objects.",
	Long:  `Create initial source code scaffolding for api objects.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// get sdk config
		sdkConfig, err := sdk.GetSDKConfig()
		if err != nil {
			cli.Error("failed to get sdk config", err)
			os.Exit(1)
		}

		// create api-object config
		configContent, err := os.ReadFile(createAPIObjectConfig)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var apiObjectConfig sdk.SdkConfig
		if err := yaml.UnmarshalStrict(configContent, &apiObjectConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		objManager, err := manager.CreateManager(sdkConfig)
		if err != nil {
			cli.Error("failed to get api object manager", err)
			os.Exit(1)
		}

		err = objManager.CreateAPIObject(apiObjectConfig)
		if err != nil {
			cli.Error("failed to get create api object with sdk", err)
			os.Exit(1)
		}

		return nil
	},
}

func init() {
	createCmd.AddCommand(createAPICmd)

	createAPICmd.Flags().StringVarP(
		&createAPIObjectConfig,
		"config", "c", "", "Path to file with api object config.",
	)
	createAPICmd.MarkFlagRequired("config")
}
