/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/controller"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	modelFilenameForController string
	apiVersions                string
	//packageName string
)

// controllerCmd represents the apiModel command
var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Generate controller code for objects",
	Long: `The controller command generates the main package and reconcilers for API
objects defined in a source code file.

By convention, the API objects (database tables) defined in a single source code
file in 'pkg/api/<version>/' correspond to a single controller in the threeport
control plane.  All of the objects defined in that source code file that require
reconciliation get their own reconciler within that controller.

This command will generally be called by ////go:generate declared in each
relevant object definition file.  As such code will be regenerated for all
controllers each time 'make generate' is called.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		baseName := sdk.FilenameSansExt(modelFilenameForController)
		controllerConfig := controller.ControllerConfig{
			Name: strings.ReplaceAll(
				fmt.Sprintf("%s-controller", baseName),
				"_",
				"-",
			),
			ShortName:   strings.ReplaceAll(baseName, "_", "-"),
			PackageName: strings.ReplaceAll(baseName, "_", ""),
			StreamName: fmt.Sprintf(
				"%sStreamName", strcase.ToCamel(baseName),
			),
		}

		additionalVersions := []string{}
		if apiVersions != "" {
			additionalVersions = strings.Split(apiVersions, ",")
		}

		paths := getAllVersionPaths(
			additionalVersions,
			modelFilenameForController,
		)

		for _, path := range paths {
			// inspect source code
			fset := token.NewFileSet()
			pf, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.AllErrors)
			if err != nil {
				return fmt.Errorf("failed to parse source code file: %w", err)
			}
			//////////////////////////////////////////////////////////////////////////
			//// print the syntax tree for dev purposes
			//if err = ast.Print(fset, pf); err != nil {
			//	return err
			//}
			//////////////////////////////////////////////////////////////////////////

			// determine which objects must be reconciled and build a map
			// of struct tags for each object

			var structType *ast.StructType
			controllerConfig.StructTags = make(map[string]map[string]map[string]string)

			// iterate over the declarations in the source code file
			for _, node := range pf.Decls {
				// if the declaration is a type declaration, iterate over the
				// specs to find the struct type and its fields
				switch node.(type) {
				case *ast.GenDecl:
					var objectName string

					// get the type declaration
					genDecl := node.(*ast.GenDecl)

					// iterate over the specs to find the struct type and its fields
					for _, spec := range genDecl.Specs {

						switch spec.(type) {
						case *ast.TypeSpec:
							// if the spec is a type spec, get the type spec and its name
							typeSpec := spec.(*ast.TypeSpec)
							objectName = typeSpec.Name.Name

							// populate the struct tags map
							structType, _ = typeSpec.Type.(*ast.StructType)
							controllerConfig.StructTags[objectName] = make(map[string]map[string]string)
							for _, field := range structType.Fields.List {
								if len(field.Names) == 0 {
									continue
								}
								fieldName := field.Names[0].Name
								tagMap := util.ParseStructTag(field.Tag.Value)
								controllerConfig.StructTags[objectName][fieldName] = tagMap
							}
						}

					}
					// check for the presence of a reconciler marker comment
					if genDecl.Doc != nil {
						for _, comment := range genDecl.Doc.List {
							if strings.Contains(comment.Text, sdk.ReconclierMarkerText) {
								controllerConfig.ReconciledObjects = append(
									controllerConfig.ReconciledObjects,
									controller.ReconciledObject{
										Name:    objectName,
										Version: pf.Name.Name,
									},
								)
							}
						}
					}
				}
			}
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

		// generate the controller's main package
		if extension {
			if err := controllerConfig.ExtensionMainPackage(modulePath); err != nil {
				return fmt.Errorf("failed to generate code for controller's main package for extension: %w", err)
			}
		} else {
			if err := controllerConfig.MainPackage(); err != nil {
				return fmt.Errorf("failed to generate code for controller's main package: %w", err)
			}
		}

		// generate the controller's internal package general source code
		if err := controllerConfig.InternalPackage(); err != nil {
			return fmt.Errorf("failed to generate code for controller's internal package source file: %w", err)
		}

		// generate the controller's reconcile functions
		if extension {
			if err := controllerConfig.ExtensionReconcilers(modulePath); err != nil {
				return fmt.Errorf("failed to generate code for controller's reconcilers for extension: %w", err)
			}
		} else {
			if err := controllerConfig.Reconcilers(); err != nil {
				return fmt.Errorf("failed to generate code for controller's reconcilers: %w", err)
			}
		}

		//// generate the controller's reconcile functions
		//if err := controllerConfig.NotificationHelper(); err != nil {
		//	return fmt.Errorf("failed to generate notification helper for controller's reconcilers: %w", err)
		//}

		return nil

	},
}

// init initializes the api-model subcommand
func init() {
	codegenCmd.AddCommand(controllerCmd)

	controllerCmd.Flags().StringVarP(&modelFilenameForController, "filename", "f", "", "The filename for the file containing the API models")
	controllerCmd.Flags().StringVarP(&apiVersions, "api-versions", "v", "", "The api-versions to generate reconcilers for. Defaults to current package version")
	controllerCmd.Flags().BoolVarP(&extension, "extension", "e", false, "Indicate whether code being generated is for an extension")
	controllerCmd.MarkFlagRequired("filename")
}

// getAllVersionPaths returns a list of paths to the source code files for
// all versions of a given object
func getAllVersionPaths(
	additionalVersions []string,
	modelFilenameForController string,
) []string {
	paths := []string{}
	name := sdk.FilenameSansExt(modelFilenameForController)
	for _, version := range additionalVersions {
		paths = append(paths, fmt.Sprintf("../%s/%s.go", version, name))
	}
	paths = append(paths, modelFilenameForController)
	return paths
}
