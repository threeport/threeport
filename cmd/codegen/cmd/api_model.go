/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/codegen"
	"github.com/threeport/threeport/internal/codegen/models"
	"github.com/threeport/threeport/internal/util"
)

// nameFields returns a list of struct type fields that indicate a struct
// requires a unique name for the object.
func nameFields() []string {
	return []string{
		"Name",
		"Definition",
		"Instance",
	}
}

var (
	filename    string
	packageName string
)

// apiModelCmd represents the apiModel command
var apiModelCmd = &cobra.Command{
	Use:   "api-model",
	Short: "Generate code for a REST API model",
	Long: `The api-model command parses the object definitions for a threeport
RESTful API model and produces all the boilerplate code for that model.  It is
generally used with go generate.

For example, let's suppose we create the models FooDefinition and FooInstance
in the file 'pkg/api/v0/foo.go'.
This creates a controller domain.  A controller domain is the set of objects
that a controller is responsible for reconciling, in this case FooDefinition and
FooInstance are the objects the foo-controller will be responsible for.
We put the go:generate declaration at the top of that file:
////go:generate threeport-codegen api-model --filename $GOFILE

Note: The controller domain and model objects must have unique names.  You
cannot have a Foo model in the Foo controller domain.  This will create ambiguous
names and is therefore not allowed.

When 'make generate' is run, the following code is generated for API:
* 'pkg/api/v0/foo_gen.go:
	* all model methods that satisfy the APIObject interface
	* NATS subjects that are used for controller notifications about the Foo
	  object
* 'internal/api/routes/foo.go':
	* the routes used by clients to manage Foo objects
* 'internal/api/handlers/foo.go':
	* the handlers that updated database state for Foo objects
* 'internal/api/database/database.go':
	* the auto migrate calls
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// inspect source code
		fset := token.NewFileSet()
		pf, err := parser.ParseFile(fset, filename, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return fmt.Errorf("failed to parse source code file: %w", err)
		}
		////////////////////////////////////////////////////////////////////////////
		//// print the syntax tree for dev purposes
		//if err = ast.Print(fset, pf); err != nil {
		//	return err
		//}
		////////////////////////////////////////////////////////////////////////////
		var modelConfigs []models.ModelConfig
		var reconcilerModels []string
		for _, node := range pf.Decls {
			switch node.(type) {
			case *ast.GenDecl:
				var objectName string
				genDecl := node.(*ast.GenDecl)
				for _, spec := range genDecl.Specs {
					switch spec.(type) {
					// in the case we're looking at a struct type definition, inspect
					case *ast.TypeSpec:
						typeSpec := spec.(*ast.TypeSpec)
						objectName = typeSpec.Name.Name
						// capture the name of the struct - the model name
						mc := models.ModelConfig{
							TypeName:  typeSpec.Name.Name,
							NameField: false, // will be set to true as needed below
						}
						// check if this is a struct type
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							// if so, iterate over the fields
							for _, field := range structType.Fields.List {
								//check if this is an ident type
								if identType, ok := field.Type.(*ast.Ident); ok {
									// if so, it may be an anonymous field - check
									// the name
									if util.StringSliceContains(nameFields(), identType.Name, true) {
										mc.NameField = true
									}
									if identType.Name == "Reconciliation" {
										mc.ReconciledField = true
									}
								}
								// each field is an *ast.Field, which has a Names field that
								// is a []*ast.Ident - iterate over those names to find the
								// one we're looking for
								for _, name := range field.Names {
									if util.StringSliceContains(nameFields(), name.Name, true) {
										mc.NameField = true
									}
									if name.Name == "Reconciled" {
										mc.ReconciledField = true
									}
								}
							}
						}
						modelConfigs = append(modelConfigs, mc)
					}
				}
				if genDecl.Doc != nil {
					for _, comment := range genDecl.Doc.List {
						if strings.Contains(comment.Text, codegen.ReconclierMarkerText) {
							reconcilerModels = append(reconcilerModels, objectName)
						}
					}
				}
			}
		}

		// construct the controller config object
		controllerConfig := models.ControllerConfig{
			ModelFilename:         filename,
			PackageName:           packageName,
			ParsedModelFile:       *pf,
			ControllerDomain:      strcase.ToCamel(codegen.FilenameSansExt(filename)),
			ControllerDomainLower: strcase.ToLowerCamel(codegen.FilenameSansExt(filename)),
			ModelConfigs:          modelConfigs,
			ReconcilerModels:      reconcilerModels,
		}

		// validate model configs
		for _, mc := range controllerConfig.ModelConfigs {
			// ensure no naming conflicts between controller domain and models
			if mc.TypeName == controllerConfig.ControllerDomain {
				err := errors.New(fmt.Sprintf(
					"controller domain %s has naming conflict with model %s",
					controllerConfig.ControllerDomain,
					mc.TypeName,
				))
				return fmt.Errorf("naming conflict encountered: %w", err)
			}
		}

		// for all objects with a reconciler:
		// * validate the model includes the Reconciled field
		// * set Reconciler field in model config to true
		for _, rm := range controllerConfig.ReconcilerModels {
			for i, mc := range controllerConfig.ModelConfigs {
				if rm == mc.TypeName {
					if !mc.ReconciledField {
						return errors.New(fmt.Sprintf(
							"%s object does not include a Reconciled field - all objects with reconcilers must include this field", rm,
						))
					} else {
						controllerConfig.ModelConfigs[i].Reconciler = true
					}
				}
			}
		}

		// generate the model's constants and methods
		if err := controllerConfig.ModelConstantsMethods(); err != nil {
			return fmt.Errorf("failed to generate model constants and methods: %w", err)
		}

		// generate the model's routes
		if err := controllerConfig.ModelRoutes(); err != nil {
			return fmt.Errorf("failed to generate model routes: %w", err)
		}

		// generate the model's handlers
		if err := controllerConfig.ModelHandlers(); err != nil {
			return fmt.Errorf("failed to generate model handlers: %w", err)
		}

		// generate functions to add API versions, validation
		if err := controllerConfig.ModelVersions(); err != nil {
			return fmt.Errorf("failed to generate model versions: %w", err)
		}

		// generate client library functions
		if err := controllerConfig.ClientLib(); err != nil {
			return fmt.Errorf("failed to generate model client library: %w", err)
		}

		return nil
	},
}

// init initializes the api-model subcommand
func init() {
	rootCmd.AddCommand(apiModelCmd)

	apiModelCmd.Flags().StringVarP(&filename, "filename", "f", "", "The filename for the file containing the API model")
	apiModelCmd.MarkFlagRequired("filename")
	apiModelCmd.Flags().StringVarP(&packageName, "package", "p", "", "The package name of the the API model")
	apiModelCmd.MarkFlagRequired("package")
}
