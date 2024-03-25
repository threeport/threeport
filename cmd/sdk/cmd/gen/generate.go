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
	Long: `The SDK will generate code for the api model and all necessary reconcilation logic.
	Generate code for Threeport or its extensions. Code generation behaviour can be controlled
	via different settings in the sdk config.
	Suppose you have an APIObjectGroup with the name foo.
	The following code is generated:
	* 'pkg/api/v0/foo_gen.go:
		* all model methods that satisfy the APIObject interface
		* NATS subject constants that are used for controller notifications about
	  	  the Foo objects
	* 'internal/api/routes/foo.go':
		* the routes used by clients to manage Foo objects
	* 'internal/api/handlers/foo.go':
		* the handlers that update database state for Foo objects
	* 'internal/api/database/database.go':
		* the auto migrate calls
	* 'pkg/client/v0/foo_gen.go':
		* go client library functions for Foo objects
	* 'cmd/tptctl/cmd/':
		* the tptctl commands to create, describe and delete foo-definition and
	  	  foo-instance objects in the API
	* the AddRoutes function in 'internal/api/routes/routes.go' that add the REST routes
	  to the api-server.
	* the tagged field maps that contain the field validation information for all
	  API Models in 'internal/api/tagged_fields_gen.go'
	* main package and reconcilers for API objects in foo.
	`,
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

		for _, apiObjectGroups := range sdkConfig.APIObjectGroups {
			for _, obj := range apiObjectGroups.Objects {
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

		for _, og := range sdkConfig.APIObjectGroups {
			if err := ApiModelGen(*og.Name, og.Objects); err != nil {
				return fmt.Errorf("could not generate code for api-model: %w", err)
			}

			// Determine if any objects within this controller domain need reconcilliation
			needReconcilers := false
			for _, obj := range og.Objects {
				if obj.Reconcilable != nil && *obj.Reconcilable {
					needReconcilers = true
					break
				}
			}

			if needReconcilers {
				if err := ControllerGen(*og.Name, og.Objects); err != nil {
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
