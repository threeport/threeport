/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/controller"
)

var (
	modelFilenameForController string
	//packageName string
)

// parseStructTag parses the struct tag string into a map[string]string
func parseStructTag(tagString string) map[string]string {
	tag := reflect.StructTag(strings.Trim(tagString, "`"))
	tagMap := make(map[string]string)
	for _, key := range tagList(tag) {
		tagMap[key] = tag.Get(key)
	}
	return tagMap
}

// tagList extracts keys from a struct tag
func tagList(tag reflect.StructTag) []string {
	raw := string(tag)
	var list []string
	for raw != "" {
		var pair string
		pair, raw = next(raw)
		key, _ := split(pair)
		list = append(list, key)
	}
	return list
}

// next gets the next key-value pair from a struct tag
func next(raw string) (pair, rest string) {
	i := strings.Index(raw, " ")
	if i < 0 {
		return raw, ""
	}
	return raw[:i], raw[i+1:]
}

// split splits a key-value pair from a struct tag
func split(pair string) (key, value string) {
	i := strings.Index(pair, ":")
	if i < 0 {
		return pair, ""
	}
	key = strings.TrimSpace(pair[:i])
	value = strings.TrimSpace(pair[i+1:])
	return key, value
}

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

		var structType *ast.StructType
		controllerConfig.StructTags = make(map[string]map[string]map[string]string)
		for _, node := range pf.Decls {
			switch node.(type) {
			case *ast.GenDecl:
				var objectName string
				genDecl := node.(*ast.GenDecl)
				for _, spec := range genDecl.Specs {

					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, _ = typeSpec.Type.(*ast.StructType)

					switch spec.(type) {
					case *ast.TypeSpec:
						typeSpec := spec.(*ast.TypeSpec)
						objectName = typeSpec.Name.Name
						controllerConfig.StructTags[objectName] = make(map[string]map[string]string)
						for _, field := range structType.Fields.List {
							if len(field.Names) > 0 {
								// for _, fieldName := range field.Names {
								// }
								fieldName := field.Names[0].Name
								// fmt.Printf("Object Name: %s\n", objectName)
								// fmt.Printf("Field Name: %s\n", field.Names[0].Name)
								// fmt.Printf("Field Tag: %s\n", field.Tag.Value)
								tagMap := parseStructTag(field.Tag.Value)
								// fmt.Printf("Map: %s\n", tagMap)
								controllerConfig.StructTags[objectName][fieldName] = tagMap
							}
						}
					}

				}
				if genDecl.Doc != nil {
					for _, comment := range genDecl.Doc.List {
						if strings.Contains(comment.Text, sdk.ReconclierMarkerText) {
							controllerConfig.ReconciledObjects = append(controllerConfig.ReconciledObjects, objectName)
						}
					}
				}
			}
		}
		fmt.Printf("Reconciled Objects: %s\n", controllerConfig.ReconciledObjects)
		fmt.Printf("Map: %s\n", controllerConfig.StructTags)
		// time.Sleep(time.Second * 10)
		// os.Exit(1)

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
	controllerCmd.Flags().BoolVarP(&extension, "extension", "e", false, "Indicate whether code being generated is for an extension")
	controllerCmd.MarkFlagRequired("filename")
}
