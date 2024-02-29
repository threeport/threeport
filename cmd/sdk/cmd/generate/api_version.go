/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/versions"
)

const (
	routeExcludeMarker    = "+threeport-sdk route-exclude"
	databaseExcludeMarker = "+threeport-sdk database-exclude"
)

func ApiVersionGen(apiVersions []string) error {
	var globalVersionConf versions.GlobalVersionConfig
	// assemble all route mapping and DB init function names
	for _, version := range apiVersions {
		versionConf := versions.VersionConfig{VersionName: version}
		var routeNames []string
		var dbInitNames []string
		var reconciledNames []string

		modelFiles, err := os.ReadDir(filepath.Join("pkg", "api", version))
		if err != nil {
			fmt.Errorf("failed to read source code files: %w", err)
		}
		for _, mf := range modelFiles {
			if strings.Contains(mf.Name(), "_gen") {
				// exclude generated code files
				continue
			}
			// parse source code
			modelFilepath := filepath.Join("pkg", "api", version, mf.Name())
			fset := token.NewFileSet()
			pf, err := parser.ParseFile(fset, modelFilepath, nil, parser.ParseComments|parser.AllErrors)
			if err != nil {
				return fmt.Errorf("failed to parse source code file: %w", err)
			}

			includeRoutes := true
			includeDBInit := true

			comments := pf.Doc
			if comments != nil {
				for _, c := range comments.List {
					if strings.Contains(c.Text, routeExcludeMarker) {
						// exclude files with route exclude marker
						includeRoutes = false
					}
					if strings.Contains(c.Text, databaseExcludeMarker) {
						// exclude files with database exclude marker
						includeDBInit = false
					}
				}
			}
			for _, node := range pf.Decls {
				switch node.(type) {
				case *ast.GenDecl:
					var objectName string
					genDecl := node.(*ast.GenDecl)
					for _, spec := range genDecl.Specs {
						switch spec.(type) {
						case *ast.TypeSpec:
							typeSpec := spec.(*ast.TypeSpec)
							objectName = typeSpec.Name.Name
							if includeRoutes {
								routeNames = append(
									routeNames,
									typeSpec.Name.Name,
								)
							}
							if includeDBInit {
								dbInitNames = append(
									dbInitNames,
									typeSpec.Name.Name,
								)
							}
						}
					}
					if genDecl.Doc != nil {
						for _, comment := range genDecl.Doc.List {
							if strings.Contains(comment.Text, sdk.ReconclierMarkerText) {
								reconciledNames = append(reconciledNames, objectName)
							}
						}
					}
				}
			}
		}
		versionConf.RouteNames = routeNames

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

		versionConf.DatabaseInitNames = dbInitNames
		versionConf.ReconciledNames = reconciledNames
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

	if err := globalVersionConf.InitJetStreamContext(sdkConfig); err != nil {
		return fmt.Errorf("failed to generate jetstream function for rest-api: %w", err)
	}

	return nil
}
