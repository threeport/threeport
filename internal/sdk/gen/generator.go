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

	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	sdkutil "github.com/threeport/threeport/internal/sdk/util"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Generator contains the values for generating the source code for Threeport
// and its extensions when the 'threeport-sdk gen' command is run.
type Generator struct {
	// If true, is an extension of Threeport.  If false, is the
	// threeeport/threeport project.
	Extension bool

	// The project path as provided by the module statement in go.mod.
	ModulePath string

	// The version of golang in use on the project.
	GoVersion string

	// Contains values for generating source code that is version oriented,
	// i.e. requires looping over versions to generate the code that is organized
	// organized by API version.
	GlobalVersionConfig GlobalVersionConfig

	// A collection of all API object groups for generating source code that is
	// API object oriented, i.e. requires looping over API objects to generate
	// code for each.
	ApiObjectGroups []ApiObjectGroup

	// All API objects collected together by version in the way the API is
	// organized in the codebase.
	VersionedApiObjectCollections []VersionedApiObjectCollection
}

// GlobalVersionConfig contains all API versions for which code is being
// generated.
type GlobalVersionConfig struct {
	// A slice of API versions.
	Versions []VersionConfig
}

// VersionConfig is the configuration for a given version.
type VersionConfig struct {
	// The name of the version, e.g "v1".
	VersionName string

	// All the API routes (REST paths) that are organized by version.
	RouteNames []string

	// The names of database init objects.
	DatabaseInitNames []string

	// The names of reconciled objects.
	ReconciledNames []string
}

// ApiObjectGroup is a group of API objects that are defined together in a file
// in pkg/api.  Reconciliation for API objects in a group is performed by one
// controller for each group - with each object using its own reconciler within
// that controller.  Therefore, an API object group is also a controller domain.
type ApiObjectGroup struct {
	// The name of the source code file where the API objects' data models are
	// defined.
	ModelFilename string

	// The controller domain for an object group.
	ControllerDomain string

	// The controller domain in all lowercase.
	ControllerDomainLower string

	// List of API object names that are reconciled by a controller.
	ReconciledApiObjectNames []string

	// The API objects that get CLI commands generated.
	TptctlModels []string

	// The API objects that have a CLI configuration that references a file on a
	// path on the filesystem.  Used to generate code to resolve that path
	// properly.
	TptctlConfigPathModels []string

	// The details for each API object in the group.
	ApiObjects []*ApiObject

	// The name of the object group's controller in kebab case, e.g.
	// kubernetes-runtime-controller
	ControllerName string

	// The name of the controller in kebab case sans "-controler", e.g
	// kubernetes-runtime
	ControllerShortName string

	// The name of the controller in lower case, no spaces, e.g.
	// kubernetesruntime
	ControllerPackageName string

	// The name of a NATS Jetstream stream for a controller, e.g.
	// KubernetesRuntimeStreamName
	StreamName string

	// The objects for which reconcilers should be generated.
	ReconciledObjects []ReconciledObject

	// The struct values parsed from the object group's model file.
	// The data model can be interpreted as:
	// map[objectName]map[fieldName]map[tagKey]tagValue
	// An example of this data model with a WorkloadDefinition is:
	// map["WorkloadDefinition"]map["YAMLDocument"]map["validate"]"required"
	StructTags map[string]map[string]map[string]string
}

// VersionedApiObjectCollection contains all API objects grouped by version and
// then group in the way the API is organized.
type VersionedApiObjectCollection struct {
	// The version for all API objects.
	Version string

	// The object groups organized by version.
	VersionedApiObjectGroups []VersionedApiObjectGroup
}

type VersionedApiObjectGroup struct {
	// Object group name in short kebab case, e.g. kubernetes-runtime
	Name string

	// The API objects for a particular version.
	ApiObjects []*ApiObject
}

// ApiObject contains the values for a particular model.
type ApiObject struct { // was: ModelConfig
	// The name of the go package where the API object's data models is defined.
	PackageName string

	// The version of the API object.
	Version string

	TypeName              string
	AllowDuplicateNames   bool
	AllowCustomMiddleware bool
	DbLoadAssociations    bool
	NameField             bool
	Reconciler            bool
	ReconciledField       bool

	// If true, generate tptctl commands for the model.
	TptctlCommands bool

	// If true, the config for the object, references another file and should
	// have code that includes passing the config path to config package object.
	TptctlConfigPath bool

	// Only applied to definition objects - if true, there is a corresponding
	// instance object.

	// Only applied to definition objects - if true, there is a corresponding
	// instance object that a DefinedInstance conncection must be made with.
	DefinedInstance bool

	// notification subjects
	CreateSubject string
	UpdateSubject string
	DeleteSubject string

	// handler names
	GetVersionHandlerName    string
	AddHandlerName           string
	AddMiddlewareFuncName    string
	GetAllHandlerName        string
	GetOneHandlerName        string
	GetMiddlewareFuncName    string
	PatchHandlerName         string
	PatchMiddlewareFuncName  string
	PutHandlerName           string
	PutMiddlewareFuncName    string
	DeleteHandlerName        string
	DeleteMiddlewareFuncName string
}

// ReconciledObject is a struct that contains the name and version of a
// reconciled object.
type ReconciledObject struct {
	// The name of the reconciled object.
	Name string

	// All the versions of the reconciled object.
	Versions []string

	// If true, do not persist notifications in NATS JetStream.
	DisableNotificationPersistence bool
}

// New populates a new Generator in preparation for source code generation.  It
// primarily uses two data sources to populate the Generator:
// * the SDK config provided by the threeport-sdk user
// * the data model defined by the threeport-sdk user in pkg/api/
func (g *Generator) New(sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	// determine if an extension and get module path from go.mod
	extension, modulePath, err := sdkutil.IsExtension()
	if err != nil {
		return fmt.Errorf("could not determine if generating code for an extension: %w", err)
	}
	g.Extension = extension
	g.ModulePath = modulePath

	// determine Go version
	goVersion, err := sdkutil.GetMajorMinorVersionFromGoModule()
	if err != nil {
		return fmt.Errorf("failed to retrieve go version from go.mod: %w", err)
	}
	g.GoVersion = goVersion

	// map the API versions to the API objects in each version
	versionObjMap := make(map[string][]*sdk.ApiObject, 0)
	for _, apiObjectGroup := range sdkConfig.ApiObjectConfig.ApiObjectGroups {
		for _, obj := range apiObjectGroup.Objects {
			for _, v := range obj.Versions {
				if _, exists := versionObjMap[*v]; exists {
					versionObjMap[*v] = append(versionObjMap[*v], obj)
				} else {
					versionObjMap[*v] = []*sdk.ApiObject{obj}
				}
			}
		}
	}

	///////////////// populate Generator.GlobalVersionConfig ///////////////////
	// iterate over the map to populate the generator's GlobalVersionConfig
	for version, mappedApiObjects := range versionObjMap {
		sort.Slice(mappedApiObjects, func(i, j int) bool {
			return *mappedApiObjects[i].Name < *mappedApiObjects[j].Name
		})

		versionConf := VersionConfig{VersionName: version}
		var routeNames []string = make([]string, 0)
		var dbInitNames []string = make([]string, 0)
		var reconciledNames []string = make([]string, 0)

		for _, obj := range mappedApiObjects {
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

		g.GlobalVersionConfig.Versions = append(
			g.GlobalVersionConfig.Versions,
			versionConf,
		)
	}

	/////////////////// populate Generator.ApiObjectGroups /////////////////////
	for _, apiObjectGroup := range sdkConfig.ApiObjectConfig.ApiObjectGroups {
		filename := fmt.Sprintf("%s.go", *apiObjectGroup.Name)

		// map the API versions to the API objects in each version for this
		// object group
		versionObjMap := make(map[string][]*sdk.ApiObject, 0)
		for _, obj := range apiObjectGroup.Objects {
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

		// iterate over the objects in each version in the map to populate the
		// generator's ApiObjectGroup
		var genApiObjectGroup ApiObjectGroup
		var apiObjects []*ApiObject
		for version, mappedApiObjects := range versionObjMap {
			var reconcilerModels []string
			var tptctlModels []string
			var tptctlModelsConfigPath []string
			var allowDuplicateNameModels []string
			var allowCustomMiddleware []string
			var dbLoadAssociations []string

			for _, obj := range mappedApiObjects {

				mc := &ApiObject{
					PackageName: version,
					Version:     version,
					TypeName:    *obj.Name,
				}

				// if a definition object, determine if a part of a
				// DefinedInstance abstraction
				if strings.HasSuffix(*obj.Name, "Definition") {
					definedInstance, _, _ := sdk.IsOfDefinedInstance(
						*obj.Name,
						apiObjectGroup.Objects,
					)
					if definedInstance {
						mc.DefinedInstance = true
					}
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

				// handler names
				mc.GetVersionHandlerName = fmt.Sprintf("Get%sVersions", *obj.Name)
				mc.AddHandlerName = fmt.Sprintf("Add%s", *obj.Name)
				mc.AddMiddlewareFuncName = fmt.Sprintf("Add%sMiddleware", *obj.Name)
				mc.GetAllHandlerName = fmt.Sprintf("Get%s", pluralize.Pluralize(*obj.Name, 2, false))
				mc.GetOneHandlerName = fmt.Sprintf("Get%s", *obj.Name)
				mc.GetMiddlewareFuncName = fmt.Sprintf("Get%sMiddleware", *obj.Name)
				mc.PatchHandlerName = fmt.Sprintf("Update%s", *obj.Name)
				mc.PatchMiddlewareFuncName = fmt.Sprintf("Patch%sMiddleware", *obj.Name)
				mc.PutHandlerName = fmt.Sprintf("Replace%s", *obj.Name)
				mc.PutMiddlewareFuncName = fmt.Sprintf("Put%sMiddleware", *obj.Name)
				mc.DeleteHandlerName = fmt.Sprintf("Delete%s", *obj.Name)
				mc.DeleteMiddlewareFuncName = fmt.Sprintf("Delete%sMiddleware", *obj.Name)

				// notif subject names
				mc.CreateSubject = *obj.Name + "CreateSubject"
				mc.UpdateSubject = *obj.Name + "UpdateSubject"
				mc.DeleteSubject = *obj.Name + "DeleteSubject"

				apiObjects = append(apiObjects, mc)
			}

			sort.Slice(apiObjects, func(i, j int) bool {
				return apiObjects[i].TypeName < apiObjects[j].TypeName
			})

			// inspect source code
			filepath := filepath.Join("pkg", "api", version, filename)
			fset := token.NewFileSet()
			pf, err := parser.ParseFile(fset, filepath, nil, parser.ParseComments|parser.AllErrors)
			if err != nil {
				return fmt.Errorf("failed to parse source code file: %w", err)
			}

			// determine which objects must be reconciled and build a map
			// of struct tags for each object
			structTags := make(map[string]map[string]map[string]string)

			// inspect the syntax tree for the object models
			for _, node := range pf.Decls {
				switch node.(type) {
				case *ast.GenDecl:
					var objectName string
					genDecl := node.(*ast.GenDecl)
					for _, spec := range genDecl.Specs {
						switch spec.(type) {
						// in the case we're looking at a struct type definition, inspect
						case *ast.TypeSpec:
							// if the spec is a type spec, get the type spec and
							// its name
							typeSpec := spec.(*ast.TypeSpec)
							objectName = typeSpec.Name.Name

							// check if this is a struct type
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								var mc *ApiObject
								for _, c := range apiObjects {
									if c.TypeName == objectName {
										mc = c
									}
								}

								structTags[objectName] = make(map[string]map[string]string)

								// if so, iterate over the fields
								for _, field := range structType.Fields.List {
									// populate the struct tags map
									if len(field.Names) == 0 {
										continue
									}
									fieldName := field.Names[0].Name
									tagMap := util.ParseStructTag(field.Tag.Value)
									structTags[objectName][fieldName] = tagMap

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

			// populate the ApiObjectGroup
			genApiObjectGroup = ApiObjectGroup{
				ModelFilename:            filename,
				ControllerDomain:         strcase.ToCamel(sdkutil.FilenameSansExt(filename)),
				ControllerDomainLower:    strcase.ToLowerCamel(sdkutil.FilenameSansExt(filename)),
				ApiObjects:               apiObjects,
				ReconciledApiObjectNames: reconcilerModels,
				TptctlModels:             tptctlModels,
				TptctlConfigPathModels:   tptctlModelsConfigPath,
				StructTags:               structTags,
			}

			// validate model configs
			for _, mc := range genApiObjectGroup.ApiObjects {
				// ensure no naming conflicts between controller domain and models
				if mc.TypeName == genApiObjectGroup.ControllerDomain {
					err := fmt.Sprintf(
						"controller domain %s has naming conflict with model %s",
						genApiObjectGroup.ControllerDomain,
						mc.TypeName,
					)
					return fmt.Errorf("naming conflict encountered: %s", err)
				}
			}

			// for all objects with a reconciler:
			// * validate the model includes the Reconciled field
			// * set Reconciler field in model config to true
			//for _, rm := range genApiObjectGroup.ReconcilerModels {
			for _, rm := range genApiObjectGroup.ReconciledApiObjectNames {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if rm == mc.TypeName {
						if !mc.ReconciledField && !extension {
							return errors.New(fmt.Sprintf(
								"%s object does not include a Reconciled field - all objects with reconcilers must include this field", rm,
							))
						} else {
							genApiObjectGroup.ApiObjects[i].Reconciler = true
						}
					}
				}
			}

			// for all objects getting tptctl commands:
			// * set TptctlCommands field in model config to true
			for _, tc := range genApiObjectGroup.TptctlModels {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if tc == mc.TypeName {
						genApiObjectGroup.ApiObjects[i].TptctlCommands = true
					}
				}
			}

			// for all objects getting tptctl command with config packages that have
			// a config path for external files:
			// * set TptctlConfigPath field in model config to true
			for _, tc := range genApiObjectGroup.TptctlConfigPathModels {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if tc == mc.TypeName {
						genApiObjectGroup.ApiObjects[i].TptctlConfigPath = true
					}
				}
			}

			// for all objects with we allow duplicate names for:
			// * set AllowDuplicateNames field in model config to true
			for _, nm := range allowDuplicateNameModels {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if nm == mc.TypeName {
						genApiObjectGroup.ApiObjects[i].AllowDuplicateNames = true
					}
				}
			}

			// for all objects with we allow custom middleware for:
			// * set AllowCustomMiddleware field in model config to true
			for _, nm := range allowCustomMiddleware {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if nm == mc.TypeName {
						genApiObjectGroup.ApiObjects[i].AllowCustomMiddleware = true
					}
				}
			}

			// for all objects that load associated data from db in handlers:
			// * set DbLoadAssociations field in model config to true
			for _, nm := range dbLoadAssociations {
				for i, mc := range genApiObjectGroup.ApiObjects {
					if nm == mc.TypeName {
						genApiObjectGroup.ApiObjects[i].DbLoadAssociations = true
					}
				}
			}

		}

		// add the controller fields to the ApiObjectGroup
		genApiObjectGroup.ControllerName = strings.ReplaceAll(
			fmt.Sprintf("%s-controller", *apiObjectGroup.Name),
			"_",
			"-",
		)
		genApiObjectGroup.ControllerShortName = strings.ReplaceAll(*apiObjectGroup.Name, "_", "-")
		genApiObjectGroup.ControllerPackageName = strings.ReplaceAll(*apiObjectGroup.Name, "_", "")
		genApiObjectGroup.StreamName = fmt.Sprintf(
			"%sStreamName", strcase.ToCamel(*apiObjectGroup.Name),
		)

		genApiObjectGroup.ReconciledObjects = make([]ReconciledObject, 0)
		for _, apiObject := range apiObjectGroup.Objects {
			var versions []string
			for _, version := range apiObject.Versions {
				versions = append(versions, *version)
			}
			if apiObject.Reconcilable != nil && *apiObject.Reconcilable {
				disableNotificationPersistense := false
				if apiObject.DisableNotificationPersistence != nil && *apiObject.DisableNotificationPersistence {
					disableNotificationPersistense = true
				}

				genApiObjectGroup.ReconciledObjects = append(
					genApiObjectGroup.ReconciledObjects,
					ReconciledObject{
						Name:                           *apiObject.Name,
						Versions:                       versions,
						DisableNotificationPersistence: disableNotificationPersistense,
					},
				)
			}
		}

		// append the assembled ApiObjectGroup in the generator
		g.ApiObjectGroups = append(g.ApiObjectGroups, genApiObjectGroup)
	}

	////////////// populate Generator.VersionedApiObjectCollections //////////////
	// add each API object to the versioned collection
	var versionedApiObjectCollections []VersionedApiObjectCollection
	for _, objGroup := range g.ApiObjectGroups {
		for _, apiObject := range objGroup.ApiObjects {
			// check for version
			versionFound := false
			for i, versionedApiObjCollection := range versionedApiObjectCollections {
				if versionedApiObjCollection.Version == apiObject.Version {
					versionFound = true
					// check for API object group
					groupFound := false
					for j, versionedGroup := range versionedApiObjCollection.VersionedApiObjectGroups {
						if versionedGroup.Name == objGroup.ControllerShortName {
							groupFound = true
							versionedApiObjectCollections[i].VersionedApiObjectGroups[j].ApiObjects = append(
								versionedApiObjectCollections[i].VersionedApiObjectGroups[j].ApiObjects,
								apiObject,
							)
							break
						}
					}
					if !groupFound {
						versionedApiObjectCollections[i].VersionedApiObjectGroups = append(
							versionedApiObjectCollections[i].VersionedApiObjectGroups,
							VersionedApiObjectGroup{
								Name: objGroup.ControllerShortName,
								ApiObjects: []*ApiObject{
									apiObject,
								},
							},
						)
					}
					break
				}
			}
			if !versionFound {
				versionedApiObjectCollections = append(
					versionedApiObjectCollections,
					VersionedApiObjectCollection{
						Version: apiObject.Version,
						VersionedApiObjectGroups: []VersionedApiObjectGroup{
							{
								Name: objGroup.ControllerShortName,
								ApiObjects: []*ApiObject{
									apiObject,
								},
							},
						},
					},
				)
			}
		}
	}

	g.VersionedApiObjectCollections = versionedApiObjectCollections

	return nil
}

// CheckStructTagMap checks if a struct tag map contains a specific value.
func (a *ApiObjectGroup) CheckStructTagMap(
	object,
	field,
	tagKey,
	expectedTagValue string,
) bool {
	if fieldTagMap, objectKeyFound := a.StructTags[object]; objectKeyFound {
		if tagValueMap, fieldKeyFound := fieldTagMap[field]; fieldKeyFound {
			if tagValue, tagKeyFound := tagValueMap[tagKey]; tagKeyFound {
				if tagValue == expectedTagValue {
					return true
				}
			}
		}
	}
	return false
}

// nameFields returns a list of struct type fields that indicate a struct
// requires a unique name for the object.
func nameFields() []string {
	return []string{
		"Name",
		"Definition",
		"Instance",
	}
}
