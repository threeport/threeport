/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"sort"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/versions"
)

const (
	routeExcludeMarker    = "+threeport-sdk route-exclude"
	databaseExcludeMarker = "+threeport-sdk database-exclude"
)

// apiVersionCmd represents the apiVersion command
func ApiVersionGen(versionObjMap map[string][]*sdk.ApiObject) error {
	var globalVersionConf versions.GlobalVersionConfig

	// assemble all objects to process further
	for version, apiObjects := range versionObjMap {
		sort.Slice(apiObjects, func(i, j int) bool {
			return *apiObjects[i].Name < *apiObjects[j].Name
		})

		versionConf := versions.VersionConfig{VersionName: version}
		var routeNames []string = make([]string, 0)
		var dbInitNames []string = make([]string, 0)
		var reconciledNames []string = make([]string, 0)

		for _, obj := range apiObjects {
			if (obj.ExcludeFromDb != nil && !*obj.ExcludeFromDb) || obj.ExcludeFromDb == nil {
				dbInitNames = append(dbInitNames, *obj.Name)
			}

			if (obj.ExcludeRoute != nil && !*obj.ExcludeRoute) || obj.ExcludeRoute == nil {
				routeNames = append(routeNames, *obj.Name)
			}

			if obj.Reconcilable != nil && *obj.Reconcilable {
				reconciledNames = append(reconciledNames, *obj.Name)
			}

		}

		if version == "v0" {
			// this is a hack to ensure that there are order constraints satisfied for
			// the db automigrate function to properly execute
			swaps := map[string]string{
				"ControlPlaneDefinition": "KubernetesRuntimeDefinition",
				"ControlPlaneInstance":   "KubernetesRuntimeInstance",
			}

			for key, value := range swaps {
				var keyIndex int = -1
				var valueIndex int = -1
				for i, name := range dbInitNames {
					if name == key {
						keyIndex = i
					} else if name == value {
						valueIndex = i
					}
				}

				if keyIndex == -1 && valueIndex == -1 && !extension {
					return fmt.Errorf("could not find items to swap in db automigrate: %s and %s", key, value)
				}

				if keyIndex != -1 && valueIndex != -1 {
					dbInitNames[keyIndex] = value
					dbInitNames[valueIndex] = key
				}
			}
		}

		versionConf.DatabaseInitNames = dbInitNames
		versionConf.ReconciledNames = reconciledNames
		versionConf.RouteNames = routeNames
		globalVersionConf.Versions = append(globalVersionConf.Versions, versionConf)
	}

	// Get module path if its an extension
	var modulePath string
	if extension {
		var modError error
		modulePath, modError = GetPathFromGoModule()
		if modError != nil {
			return fmt.Errorf("could not get go module path for extension: %w", modError)
		}
	}

	// generate all the APIs REST route mappings
	if extension {
		if err := globalVersionConf.ExtensionAllRoutes(modulePath); err != nil {
			return fmt.Errorf("failed to write all routes source code for extension: %w", err)
		}
	} else {
		if err := globalVersionConf.AllRoutes(); err != nil {
			return fmt.Errorf("failed to write all routes source code: %w", err)
		}
	}

	// generate the database init code incl the automigrate calls
	if extension {
		if err := globalVersionConf.ExtensionDatabaseInit(modulePath); err != nil {
			return fmt.Errorf("failed to write database init source code for extension: %w", err)
		}
	} else {
		if err := globalVersionConf.DatabaseInit(); err != nil {
			return fmt.Errorf("failed to write database init source code: %w", err)
		}
	}

	// generate the tagged fields code
	if extension {
		if err := globalVersionConf.ExtensionTaggedFields(); err != nil {
			return fmt.Errorf("failed to write tagged field source code for extension: %w", err)
		}
	} else {
		if err := globalVersionConf.TaggedFields(); err != nil {
			return fmt.Errorf("failed to write tagged field source code: %w", err)
		}
	}

	// generate the version maps
	if err := globalVersionConf.AddVersions(); err != nil {
		return fmt.Errorf("failed to write add versions source code: %w", err)
	}

	// generate response object type conversions
	if extension {
		if err := globalVersionConf.ExtensionResponseObjects(); err != nil {
			return fmt.Errorf("failed to write response object source code for extension: %w", err)
		}
	}

	// generate client type switch functions
	if err := globalVersionConf.DeleteObjects(); err != nil {
		return fmt.Errorf("failed to generate model type switch functions: %w", err)
	}

	// generate the notifications helper
	if err := globalVersionConf.NotificationHelper(); err != nil {
		return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
	}

	// generate the controller streams
	sdkConfig, err := sdk.GetSDKConfig()
	if err != nil {
		return fmt.Errorf("could not get sdk config: %w", err)
	}

	// Generate the jetstream context for the rest-api to interact with reconcilers
	if err := globalVersionConf.InitJetStreamContext(sdkConfig); err != nil {
		return fmt.Errorf("failed to generate jetstream function for rest-api: %w", err)
	}

	return nil
}
