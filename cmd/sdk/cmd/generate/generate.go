/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/cmd/sdk/cmd"
	"github.com/threeport/threeport/internal/sdk"
)

// generateCmd represents the parent command for all codegen related operations
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate code for Threeport or its extensions.",
	Long:  `Generate code for Threeport or its extensions.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// get sdk config
		sdkConfig, err := sdk.GetSDKConfig()
		if err != nil {
			return fmt.Errorf("failed to get sdk config: %w", err)
		}

		if err := ApiVersionGen(args); err != nil {
			return fmt.Errorf("could not generate code for api-version: %w", err)
		}

		for _, version := range args {
			for controllerDomain, apiObjects := range sdkConfig.APIObjects {
				if err := ApiModelGen(controllerDomain, version); err != nil {
					return fmt.Errorf("could not generate code for api-model: %w", err)
				}

				// Determine if any objects within this controller domain need reconcilliation
				needReconcilers := false
				for _, obj := range apiObjects {
					if obj.Reconcilable != nil && *obj.Reconcilable {
						needReconcilers = true
						break
					}
				}

				if needReconcilers {
					if err := ControllerGen(controllerDomain, version); err != nil {
						return fmt.Errorf("could not generate code for controller: %w", err)
					}
				}
			}
		}

		return nil
	},
}

func init() {
	cmd.RootCmd.AddCommand(generateCmd)
}
