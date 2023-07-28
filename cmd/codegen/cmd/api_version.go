/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/codegen/versions"
)

// const versionExcludeMarker = "+threeport-codegen version-exclude"
const (
	routeExcludeMarker    = "+threeport-codegen route-exclude"
	databaseExcludeMarker = "+threeport-codegen database-exclude"
)

// apiVersionCmd represents the apiVersion command
var apiVersionCmd = &cobra.Command{
	Use:   "api-version",
	Short: "Generate code for all models in an API version",
	Long: `The api-version command accepts versions arguments and generates code
for all the models in the supplied version/s.  The generated code includes:
* the AddRoutes function in 'internal/api/routes/routes.go' that add the REST routes
  to the server.
* the AutoMigrate calls to add the database tables for the API models in
  'internal/api/database/database_gen.go'.
* the tagged field maps that contain the field validation information for all
  API Models in 'internal/api/tagged_fields_gen.go'
`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var globalVersionConf versions.GlobalVersionConfig
		// assemble all route mapping and DB init function names
		for _, version := range args {
			versionConf := versions.VersionConfig{VersionName: version}
			var routeNames []string
			var dbInitNames []string

			modelFiles, err := os.ReadDir(filepath.Join("..", "..", "pkg", "api", version))
			if err != nil {
				fmt.Errorf("failed to read source code files: %w", err)
			}
			for _, mf := range modelFiles {
				if strings.Contains(mf.Name(), "_gen") {
					// exclude generated code files
					continue
				}
				// parse source code
				modelFilepath := filepath.Join("..", "..", "pkg", "api", version, mf.Name())
				fset := token.NewFileSet()
				pf, err := parser.ParseFile(fset, modelFilepath, nil, parser.ParseComments)
				if err != nil {
					return fmt.Errorf("failed to parse source code file: %w", err)
				}

				includeRoutes := true
				includeDBInit := true

				comments := pf.Doc
				if comments != nil {
					//exclude := false
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
						genDecl := node.(*ast.GenDecl)
						for _, spec := range genDecl.Specs {
							switch spec.(type) {
							case *ast.TypeSpec:
								typeSpec := spec.(*ast.TypeSpec)
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
					}
				}
			}
			versionConf.RouteNames = routeNames
			versionConf.DatabaseInitNames = dbInitNames
			globalVersionConf.Versions = append(globalVersionConf.Versions, versionConf)
		}

		// generate all the APIs REST route mappings
		if err := globalVersionConf.AllRoutes(); err != nil {
			return fmt.Errorf("failed to write all routes source code: %w", err)
		}

		// generate the database init code incl the automigrate calls
		if err := globalVersionConf.DatabaseInit(); err != nil {
			return fmt.Errorf("failed to write database init source code: %w", err)
		}

		// generate the tagged fields code
		if err := globalVersionConf.TaggedFields(); err != nil {
			return fmt.Errorf("failed to write tagged field source code: %w", err)
		}

		// generate the version maps
		if err := globalVersionConf.AddVersions(); err != nil {
			return fmt.Errorf("failed to write add versions source code: %w", err)
		}

		// generate response object type conversions
		if err := globalVersionConf.ResponseObjects(); err != nil {
			return fmt.Errorf("failed to write response object source code: %w", err)
		}

		// generate client type switch functions
		if err := globalVersionConf.DeleteObjects(); err != nil {
			return fmt.Errorf("failed to generate model type switch functions: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(apiVersionCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// apiVersionCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// apiVersionCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
