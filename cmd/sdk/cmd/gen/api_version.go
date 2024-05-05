/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"sort"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/mod"
	"github.com/threeport/threeport/internal/sdk/versions"
)

const (
	routeExcludeMarker    = "+threeport-sdk route-exclude"
	databaseExcludeMarker = "+threeport-sdk database-exclude"
)

// ApiVersionGen generates the source code across API versions.
func ApiVersionGen(
	sdkConfig *sdk.SdkConfig,
) error {
	var globalVersionConf versions.GlobalVersionConfig

	extension, modulePath, err := mod.IsExtension()
	if err != nil {
		return fmt.Errorf("could not determine if generating code for an extension: %w", err)
	}

	// group objects according to version for version gen logic
	versionObjMap := make(map[string][]*sdk.ApiObject, 0)

	for _, apiObjectGroups := range sdkConfig.ApiObjectConfig.ApiObjectGroups {
		for _, obj := range apiObjectGroups.Objects {
			for _, v := range obj.Versions {
				if _, exists := versionObjMap[*v]; exists {
					versionObjMap[*v] = append(versionObjMap[*v], obj)
				} else {
					versionObjMap[*v] = []*sdk.ApiObject{obj}
				}
			}
		}
	}

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

		if version == "v0" && !extension {
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

	// generate the APIs main package
	if extension {
		if err := globalVersionConf.ExtensionApiMain(sdkConfig); err != nil {
			return fmt.Errorf("failed to generate API main package: %w", err)
		}
	} else {
		if err := globalVersionConf.ApiMain(sdkConfig); err != nil {
			return fmt.Errorf("failed to generate API main package: %w", err)
		}
	}

	// generate API server handler boilerplate
	if extension {
		if err := globalVersionConf.ExtensionApiHandler(); err != nil {
			return fmt.Errorf("failed to create API handler boilerplate: %w", err)
		}
	} else {
		if err := globalVersionConf.ApiHandler(); err != nil {
			return fmt.Errorf("failed to create API handler boilerplate: %w", err)
		}
	}

	// generate all the APIs REST route mappings
	if extension {
		if err := globalVersionConf.ExtensionAllRoutes(modulePath); err != nil {
			return fmt.Errorf("failed to write all routes source code: %w", err)
		}
	} else {
		if err := globalVersionConf.AllRoutes(); err != nil {
			return fmt.Errorf("failed to write all routes source code: %w", err)
		}
	}

	// generate the database init code incl the automigrate calls
	if extension {
		if err := globalVersionConf.ExtensionDatabaseInit(modulePath); err != nil {
			return fmt.Errorf("failed to write database init source code: %w", err)
		}
	} else {
		if err := globalVersionConf.DatabaseInit(); err != nil {
			return fmt.Errorf("failed to write database init source code: %w", err)
		}
	}

	// generate the tagged fields code
	if extension {
		if err := globalVersionConf.ExtensionTaggedFields(); err != nil {
			return fmt.Errorf("failed to write tagged field source code: %w", err)
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
			return fmt.Errorf("failed to write response object source code: %w", err)
		}
	}

	// generate client type switch functions
	if extension {

	} else {
		if err := globalVersionConf.DeleteObjects(); err != nil {
			return fmt.Errorf("failed to generate model type switch functions: %w", err)
		}
	}

	// generate the notifications helper
	if extension {

	} else {
		if err := globalVersionConf.NotificationHelper(); err != nil {
			return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
		}
	}

	// generate the jetstream context for the rest-api to interact with reconcilers
	if err := globalVersionConf.InitJetStreamContext(&sdkConfig.ApiObjectConfig, modulePath); err != nil {
		return fmt.Errorf("failed to generate jetstream function for rest-api: %w", err)
	}

	// generate the version route function for serving the API version at
	// /version
	if extension {
		if err := globalVersionConf.ExtensionApiVersion(modulePath); err != nil {
			return fmt.Errorf("failed to generate the API version route function: %w", err)
		}
	} else {
		if err := globalVersionConf.ApiVersion(); err != nil {
			return fmt.Errorf("failed to generate the API version route function: %w", err)
		}
	}

	return nil
}
