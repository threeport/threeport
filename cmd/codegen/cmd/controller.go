/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/codegen"
	"github.com/threeport/threeport/internal/codegen/controller"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	modelFilenameForController string
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
		// inspect source code
		fset := token.NewFileSet()
		pf, err := parser.ParseFile(fset, modelFilenameForController, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return fmt.Errorf("failed to parse source code file: %w", err)
		}
		//////////////////////////////////////////////////////////////////////////
		//// print the syntax tree for dev purposes
		//if err = ast.Print(fset, pf); err != nil {
		//	return err
		//}
		//////////////////////////////////////////////////////////////////////////
		baseName := codegen.FilenameSansExt(modelFilenameForController)
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
						if strings.Contains(comment.Text, codegen.ReconclierMarkerText) {
							controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, objectName)
						}
					}
				}
			}
		}

		// generate the controller's main package
		if extension {
			if err := controllerConfig.ExtensionMainPackage(); err != nil {
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
			if err := controllerConfig.ExtensionReconcilers(); err != nil {
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
	rootCmd.AddCommand(controllerCmd)

	controllerCmd.Flags().StringVarP(&modelFilenameForController, "filename", "f", "", "The filename for the file containing the API models")
	controllerCmd.Flags().BoolVarP(&extension, "extension", "e", false, "Indicate whether code being generated is for an extension")
	controllerCmd.MarkFlagRequired("filename")
}
