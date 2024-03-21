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

// genCmd represents the parent command for all codegen related operations
var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate code for Threeport or its extensions.",
	Long:  `Generate code for Threeport or its extensions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get SDK config
		sdkConfig, err := sdk.GetSDKConfig()
		if err != nil {
			return fmt.Errorf("failed to get sdk config: %w", err)
		}

		// Determine whether an object is being created first time
		// In that case we ensure the necessary api files exists for the user

		// group objects according to version for version gen logic
		versionObjMap := make(map[string][]*sdk.APIObject, 0)

		for _, apiObjects := range sdkConfig.APIObjects {
			for _, obj := range apiObjects {
				for _, v := range obj.Versions {
					if _, exists := versionObjMap[*v]; exists {
						versionObjMap[*v] = append(versionObjMap[*v], obj)
					} else {
						versionObjMap[*v] = []*sdk.APIObject{obj}
					}
				}
			}
		}

		if err := ApiVersionGen(versionObjMap); err != nil {
			return fmt.Errorf("could not generate code for api-version: %w", err)
		}

		for controllerDomain, apiObjects := range sdkConfig.APIObjects {
			if err := ApiModelGen(controllerDomain, apiObjects); err != nil {
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
				if err := ControllerGen(controllerDomain, apiObjects); err != nil {
					return fmt.Errorf("could not generate code for controller: %w", err)
				}
			}
		}

		return nil
	},
}

func init() {
	cmd.RootCmd.AddCommand(genCmd)
}
