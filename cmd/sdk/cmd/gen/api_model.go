/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iancoleman/strcase"

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

func ApiModelGen(controllerDomain string, apiObjects []*sdk.ApiObject) error {
	filename := fmt.Sprintf("%s.go", controllerDomain)
	// Assemble all api objects in this controller domain according to there version
	versionObjMap := make(map[string][]*sdk.ApiObject, 0)
	for _, obj := range apiObjects {
		if obj.ExcludeRoute != nil && *obj.ExcludeRoute {
			continue
		}

		for _, v := range obj.Versions {
			if _, exists := versionObjMap[*v]; exists {
				versionObjMap[*v] = append(versionObjMap[*v], obj)
			} else {
				versionObjMap[*v] = []*sdk.ApiObject{obj}
			}
		}
	}

	extension, modulePath, err := isExtension()
	if err != nil {
		return fmt.Errorf("could not determine if running for an extension: %w", err)
	}

	for version, objects := range versionObjMap {
		var modelConfigs []*models.ModelConfig
		var reconcilerModels []string
		var tptctlModels []string
		var tptctlModelsConfigPath []string
		var allowDuplicateNameModels []string
		var allowCustomMiddleware []string
		var dbLoadAssociations []string

		for _, obj := range objects {

			mc := &models.ModelConfig{
				TypeName: *obj.Name,
			}

			if obj.Reconcilable != nil && *obj.Reconcilable {
				reconcilerModels = append(reconcilerModels, *obj.Name)
				mc.ReconciledField = true
			}

			if obj.AllowCustomMiddleware != nil && *obj.AllowCustomMiddleware {
				allowCustomMiddleware = append(allowCustomMiddleware, *obj.Name)
			}

			if obj.AllowDuplicateModelNames != nil && *obj.AllowDuplicateModelNames {
				allowDuplicateNameModels = append(allowDuplicateNameModels, *obj.Name)
			}

			if obj.LoadAssociationsFromDb != nil && *obj.LoadAssociationsFromDb {
				dbLoadAssociations = append(dbLoadAssociations, *obj.Name)
			}

			if obj.Tptctl != nil {
				if obj.Tptctl.Enabled != nil && *obj.Tptctl.Enabled {
					tptctlModels = append(tptctlModels, *obj.Name)
				}

				if obj.Tptctl.ConfigPath != nil && *obj.Tptctl.ConfigPath {
					tptctlModelsConfigPath = append(tptctlModelsConfigPath, *obj.Name)
				}
			}

			modelConfigs = append(modelConfigs, mc)

		}

		sort.Slice(modelConfigs, func(i, j int) bool {
			return modelConfigs[i].TypeName < modelConfigs[j].TypeName
		})

		filepath := filepath.Join("pkg", "api", version, filename)
		// inspect source code
		fset := token.NewFileSet()
		pf, err := parser.ParseFile(fset, filepath, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			return fmt.Errorf("failed to parse source code file: %w", err)
		}

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
						// check if this is a struct type
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							var mc *models.ModelConfig
							for _, c := range modelConfigs {
								if c.TypeName == objectName {
									mc = c
								}
							}

							// if so, iterate over the fields
							for _, field := range structType.Fields.List {
								// fields will be of type *ast.Ident
								if identType, ok := field.Type.(*ast.Ident); ok {
									if util.StringSliceContains(nameFields(), identType.Name, true) {
										mc.NameField = true
									}
								}
								// structs will be of type *ast.SelectorExpr
								if identType, ok := field.Type.(*ast.SelectorExpr); ok {
									if util.StringSliceContains(nameFields(), identType.Sel.Name, true) {
										mc.NameField = true
									}
								}
								// each field is an *ast.Field, which has a Names field that
								// is a []*ast.Ident - iterate over those names to find the
								// one we're looking for
								for _, name := range field.Names {
									if util.StringSliceContains(nameFields(), name.Name, true) {
										mc.NameField = true
									}
								}
							}
						}
					}
				}
			}
		}

		// construct the controller config object
		controllerConfig := models.ControllerConfig{
			Version:                version,
			ModelFilename:          filename,
			PackageName:            version,
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
				return fmt.Errorf("naming conflict encountered: %s", err)
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
		if extension {

		} else {
			if err := controllerConfig.TptctlCommands(); err != nil {
				return fmt.Errorf("failed to generate tptctl commands: %w", err)
			}
		}
	}

	return nil
}
