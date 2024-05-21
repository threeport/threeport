/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	sdkcmd "github.com/threeport/threeport/internal/sdk/gen/cmd"
	"github.com/threeport/threeport/internal/sdk/gen/internalpkg"
	sdkpkg "github.com/threeport/threeport/internal/sdk/gen/pkg"
	"github.com/threeport/threeport/internal/sdk/gen/root"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

var genConfig string

// genCmd represents the parent command for all codegen related operations
var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate source code for Threeport or its extensions.",
	Long: `Generate source code for Threeport or its extensions.

Once you have defined your data model for the API objects in 'pkg/api/', run this
command to generate the source code scaffolding and boilerplated for the project.

This command uses the SDK config and the source code you have defined for your API
objects to generate source code to produce components that can be compiled and
deployed.

After running this command add the functionality to your controllers in
'internal/[API object group name]/'.

See the Threeport SDK docs for more information: https://docs.threeport.io/sdk/sdk-intro/
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// get SDK config
		sdkConfig, err := sdk.GetSdkConfig(genConfig)
		if err != nil {
			//return fmt.Errorf("failed to get sdk config: %w", err)
			cli.Error("failed to get valid Threeport SDK config", err)
			os.Exit(1)
		}

		// create gen config that will inform code generation based
		var generator gen.Generator
		if err := generator.New(sdkConfig); err != nil {
			cli.Error("failed to create new generator from SDK config for code generation", err)
			os.Exit(1)
		}

		fmt.Println("----------------------------------------------------------")
		genJson, err := json.MarshalIndent(generator, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal generator JSON: %w", err)
		}
		fmt.Println(string(genJson))
		fmt.Println("----------------------------------------------------------")
		for _, objGroup := range generator.ApiObjectGroups {
			for _, obj := range objGroup.ApiObjects {
				fmt.Println("&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&")
				fmt.Printf("%+v\n", *obj)
				fmt.Println("&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&")
			}
		}

		////////////////////////////////////////////////////////////////////////
		// build source code at root of project
		if err := root.GenRoot(&generator); err != nil {
			cli.Error("failed to generate source code at prject root", err)
			os.Exit(1)
		}

		// build source code for internal packages
		if err := internalpkg.GenInternalPkg(&generator, sdkConfig); err != nil {
			cli.Error("failed to generate source code for internal package", err)
			os.Exit(1)
		}

		// build source code for cmd packages
		if err := sdkcmd.GenCmd(&generator, sdkConfig); err != nil {
			cli.Error("failed to generate source code for cmd package", err)
			os.Exit(1)
		}

		// build source code for pkg packages
		if err := sdkpkg.GenPkg(&generator, sdkConfig); err != nil {
			cli.Error("failed to generate source code for pkg package", err)
			//os.Exit(1)
		}

		////////////////////////////////////////////////////////////////////////

		//if err := ApiVersionGen(sdkConfig); err != nil {
		//	return fmt.Errorf("could not generate code for api-version: %w", err)
		//}

		//for _, og := range sdkConfig.ApiObjectConfig.ApiObjectGroups {
		//	if err := ApiModelGen(*og.Name, og.Objects); err != nil {
		//		return fmt.Errorf("could not generate code for api-model: %w", err)
		//	}

		//	// determine if any objects within this controller domain need reconcilliation
		//	needReconcilers := false
		//	for _, obj := range og.Objects {
		//		if obj.Reconcilable != nil && *obj.Reconcilable {
		//			needReconcilers = true
		//			break
		//		}
		//	}

		//	if needReconcilers {
		//		if err := ControllerGen(*og.Name, og); err != nil {
		//			return fmt.Errorf("could not generate code for controller: %w", err)
		//		}
		//	}
		//}

		cli.Complete("source code generation complete")

		return nil
	},
}

func init() {
	RootCmd.AddCommand(genCmd)

	genCmd.Flags().StringVarP(
		&genConfig,
		"config", "c", "", "Path to file with Threeport SDK config.",
	)
	genCmd.MarkFlagRequired("config")
}
