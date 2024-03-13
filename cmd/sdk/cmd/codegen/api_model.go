/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package codegen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/models"
	util "github.com/threeport/threeport/pkg/util/v0"
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
////go:generate threeport-sdk codegen api-model --filename $GOFILE --package $GOPACKAGE

Note: The controller domain and model objects must have unique names.  You
cannot have a Foo model in the Foo controller domain.  This will create ambiguous
names and is therefore not allowed.

When 'make generate' is run, the following code is generated for API:
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
		var tptctlModels []string
		var tptctlModelsConfigPath []string
		var allowDuplicateNameModels []string
		var allowCustomMiddleware []string
		var dbLoadAssociations []string
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
						fmt.Println(objectName)
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
								checkNameField := func(name string) {
									// if so, it may be an anonymous field - check
									// the name
									if util.StringSliceContains(nameFields(), name, true) {
										mc.NameField = true
									}
									if name == "Reconciliation" {
										mc.ReconciledField = true
									}
								}
								if identType, ok := field.Type.(*ast.Ident); ok {
									checkNameField(identType.Name)
								}
								if identType, ok := field.Type.(*ast.SelectorExpr); ok {
									checkNameField(identType.Sel.Name)
								}
								// each field is an *ast.Field, which has a Names field that
								// is a []*ast.Ident - iterate over those names to find the
								// one we're looking for
								for _, name := range field.Names {
									fmt.Println(name.Name)
									if util.StringSliceContains(nameFields(), name.Name, true) {
										mc.NameField = true
									}
									if name.Name == "Reconciled" || name.Name == "v0.Reconciled" {
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
						if strings.Contains(comment.Text, sdk.ReconclierMarkerText) {
							reconcilerModels = append(reconcilerModels, objectName)
						} else if strings.Contains(comment.Text, sdk.AllowDuplicateNamesMarkerText) {
							allowDuplicateNameModels = append(allowDuplicateNameModels, objectName)
						} else if strings.Contains(comment.Text, sdk.AddCustomMiddleware) {
							allowCustomMiddleware = append(allowCustomMiddleware, objectName)
						} else if strings.Contains(comment.Text, sdk.DbLoadAssociations) {
							dbLoadAssociations = append(dbLoadAssociations, objectName)
						}
						if strings.Contains(comment.Text, sdk.TptctlMarkerText) {
							tptctlModels = append(tptctlModels, objectName)
						}
						if strings.Contains(comment.Text, sdk.TptctlMarkerConfigPathText) {
							tptctlModelsConfigPath = append(tptctlModelsConfigPath, objectName)
						}
					}
				}
			}
		}

		// construct the controller config object
		controllerConfig := models.ControllerConfig{
			ModelFilename:          filename,
			PackageName:            packageName,
			ParsedModelFile:        *pf,
			ControllerDomain:       strcase.ToCamel(sdk.FilenameSansExt(filename)),
			ControllerDomainLower:  strcase.ToLowerCamel(sdk.FilenameSansExt(filename)),
			ModelConfigs:           modelConfigs,
			ReconcilerModels:       reconcilerModels,
			TptctlModels:           tptctlModels,
			TptctlConfigPathModels: tptctlModelsConfigPath,
			ApiVersion:             pf.Name.Name,
		}

		// validate model configs
		for _, mc := range controllerConfig.ModelConfigs {
			// ensure no naming conflicts between controller domain and models
			if mc.TypeName == controllerConfig.ControllerDomain {
				err := fmt.Sprintf(
					"controller domain %s has naming conflict with model %s",
					controllerConfig.ControllerDomain,
					mc.TypeName,
				)
				return fmt.Errorf("naming conflict encountered: %w", err)
			}
		}

		// for all definition objects that have a corresponding instance object:
		// * set DefinedInstance to true on the definition model config
		for i, mc := range controllerConfig.ModelConfigs {
			if strings.HasSuffix(mc.TypeName, "Definition") {
				// have found a definition object, see if there's a
				// corresponding instance
				rootDefObj := strings.TrimSuffix(mc.TypeName, "Definition")
				for _, imc := range controllerConfig.ModelConfigs {
					if strings.HasSuffix(imc.TypeName, "Instance") {
						rootInstObj := strings.TrimSuffix(imc.TypeName, "Instance")
						if rootDefObj == rootInstObj {
							controllerConfig.ModelConfigs[i].DefinedInstance = true
						}
					}
				}
			}
		}

		// for all objects with a reconciler:
		// * validate the model includes the Reconciled field
		// * set Reconciler field in model config to true
		for _, rm := range controllerConfig.ReconcilerModels {
			for i, mc := range controllerConfig.ModelConfigs {
				if rm == mc.TypeName {
					if !mc.ReconciledField && !extension {
						return errors.New(fmt.Sprintf(
							"%s object does not include a Reconciled field - all objects with reconcilers must include this field", rm,
						))
					} else {
						controllerConfig.ModelConfigs[i].Reconciler = true
					}
				}
			}
		}

		// for all objects getting tptctl commands:
		// * set TptctlCommands field in model config to true
		for _, tc := range controllerConfig.TptctlModels {
			for i, mc := range controllerConfig.ModelConfigs {
				if tc == mc.TypeName {
					controllerConfig.ModelConfigs[i].TptctlCommands = true
				}
			}
		}

		// for all objects getting tptctl command with config packages that have
		// a config path for external files:
		// * set TptctlConfigPath field in model config to true
		for _, tc := range controllerConfig.TptctlConfigPathModels {
			for i, mc := range controllerConfig.ModelConfigs {
				if tc == mc.TypeName {
					controllerConfig.ModelConfigs[i].TptctlConfigPath = true
				}
			}
		}

		// for all objects with we allow duplicate names for:
		// * set AllowDuplicateNames field in model config to true
		for _, nm := range allowDuplicateNameModels {
			for i, mc := range controllerConfig.ModelConfigs {
				if nm == mc.TypeName {
					controllerConfig.ModelConfigs[i].AllowDuplicateNames = true
				}
			}
		}

		// for all objects with we allow custom middleware for:
		// * set AllowCustomMiddleware field in model config to true
		for _, nm := range allowCustomMiddleware {
			for i, mc := range controllerConfig.ModelConfigs {
				if nm == mc.TypeName {
					controllerConfig.ModelConfigs[i].AllowCustomMiddleware = true
				}
			}
		}

		// for all objects that load associated data from db in handlers:
		// * set DbLoadAssociations field in model config to true
		for _, nm := range dbLoadAssociations {
			for i, mc := range controllerConfig.ModelConfigs {
				if nm == mc.TypeName {
					controllerConfig.ModelConfigs[i].DbLoadAssociations = true
				}
			}
		}

		// get module path if its an extension
		var modulePath string
		if extension {
			var modError error
			modulePath, modError = GetPathFromGoModule()
			if modError != nil {
				return fmt.Errorf("could not get go module path for extension: %w", modError)
			}
		}

		// generate the model's constants and methods
		if extension {
			if err := controllerConfig.ExtensionModelConstantsMethods(); err != nil {
				return fmt.Errorf("failed to generate model constants and methods for extension: %w", err)
			}
		} else {
			if err := controllerConfig.ModelConstantsMethods(); err != nil {
				return fmt.Errorf("failed to generate model constants and methods: %w", err)
			}
		}

		// generate the model's routes
		if extension {
			if err := controllerConfig.ExtensionModelRoutes(modulePath); err != nil {
				return fmt.Errorf("failed to generate model routes for extension: %w", err)
			}
		} else {
			if err := controllerConfig.ModelRoutes(); err != nil {
				return fmt.Errorf("failed to generate model routes: %w", err)
			}
		}

		// generate the model's handlers
		if extension {
			if err := controllerConfig.ExtensionModelHandlers(modulePath); err != nil {
				return fmt.Errorf("failed to generate model handlers for extension: %w", err)
			}
		} else {
			if err := controllerConfig.ModelHandlers(); err != nil {
				return fmt.Errorf("failed to generate model handlers: %w", err)
			}
		}

		if extension {
			// generate functions to add API versions, validation
			if err := controllerConfig.ExtensionModelVersions(modulePath); err != nil {
				return fmt.Errorf("failed to generate model versions for extension: %w", err)
			}
		} else {
			// generate functions to add API versions, validation
			if err := controllerConfig.ModelVersions(); err != nil {
				return fmt.Errorf("failed to generate model versions: %w", err)
			}
		}

		// generate client library functions
		if extension {
			if err := controllerConfig.ExtensionClientLib(modulePath); err != nil {
				return fmt.Errorf("failed to generate model client library for extension: %w", err)
			}
		} else {
			if err := controllerConfig.ClientLib(); err != nil {
				return fmt.Errorf("failed to generate model client library: %w", err)
			}
		}

		// generate tptctl commands
		if err := controllerConfig.TptctlCommands(); err != nil {
			return fmt.Errorf("failed to generate tptctl commands: %w", err)
		}

		return nil
	},
}

// init initializes the api-model subcommand
func init() {
	codegenCmd.AddCommand(apiModelCmd)

	apiModelCmd.Flags().StringVarP(
		&filename,
		"filename", "f", "", "The filename for the file containing the API model/s.",
	)
	apiModelCmd.MarkFlagRequired("filename")
	apiModelCmd.Flags().StringVarP(
		&packageName,
		"package", "p", "", "The package name of the the API model.",
	)
	apiModelCmd.MarkFlagRequired("package")
	apiModelCmd.Flags().BoolVarP(
		&extension,
		"extension", "e", false, "Indicates whether code being generated is for a Threeport extension.",
	)
}
