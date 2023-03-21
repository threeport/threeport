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

	"github.com/iancoleman/strcase"
	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/codegen/models"
	"github.com/threeport/threeport/internal/codegen/name"
)

var filename string

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
		pf, err := parser.ParseFile(fset, filename, nil, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("failed to parse source code file: %w", err)
		}
		////////////////////////////////////////////////////////////////////////////
		// print the syntax tree for dev purposes
		//if err = ast.Print(fset, pf); err != nil {
		//	return err
		//}
		////////////////////////////////////////////////////////////////////////////
		var structTypeNames []string
		for _, node := range pf.Decls {
			switch node.(type) {
			case *ast.GenDecl:
				genDecl := node.(*ast.GenDecl)
				for _, spec := range genDecl.Specs {
					switch spec.(type) {
					case *ast.TypeSpec:
						typeSpec := spec.(*ast.TypeSpec)
						structTypeNames = append(structTypeNames, typeSpec.Name.Name)
					}
				}
			}
		}

		// create the model config
		var modelConfigs []models.ModelConfig
		for _, stn := range structTypeNames {
			mc := models.ModelConfig{
				TypeName: stn,
			}
			modelConfigs = append(modelConfigs, mc)
		}
		controllerConfig := models.ControllerConfig{
			ModelFilename:         filename,
			ParsedModelFile:       *pf,
			ControllerDomain:      strcase.ToCamel(name.FilenameSansExt(filename)),
			ControllerDomainLower: strcase.ToLowerCamel(name.FilenameSansExt(filename)),
			ModelConfigs:          modelConfigs,
		}

		// validate names
		// ensure no naming conflicts between controller domain and models
		for _, mc := range controllerConfig.ModelConfigs {
			if mc.TypeName == controllerConfig.ControllerDomain {
				err := errors.New(fmt.Sprintf(
					"controller domain %s has naming conflict with model %s",
					controllerConfig.ControllerDomain,
					mc.TypeName,
				))
				return fmt.Errorf("naming conflict encountered: %w", err)
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

		return nil
	},
}

// init initializes the api-model subcommand
func init() {
	rootCmd.AddCommand(apiModelCmd)

	apiModelCmd.Flags().StringVarP(&filename, "filename", "f", "", "The filename for the file containing the API models")
	apiModelCmd.MarkFlagRequired("filename")
}
