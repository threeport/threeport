package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	api "github.com/threeport/threeport/pkg/api/v0"
	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	sdkutil "github.com/threeport/threeport/pkg/sdk/v0/util"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Generator contains the values for generating the source code for Threeport
// and its extensions when the 'threeport-sdk gen' command is run.
type Generator struct {
	// If true, is a module of Threeport. If false, is the
	// threeeport/threeport project.
	Module bool

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

	// EmbedTypes carries struct-tag info for the shared base types that
	// model objects anonymously embed (Common, Definition, Instance,
	// Reconciliation). These types live in non-domain source files
	// (common.go, class.go) so they don't appear in any ApiObjectGroup's
	// StructTags, but their fields participate in binding via the
	// QueryBinder's anonymous-embed recursion. The collision check in
	// ValidateTags reads from here to flatten embedded fields into the
	// per-struct effective-key set.
	// Shape: typeName -> fieldName -> tagKey -> tagValue.
	EmbedTypes map[string]map[string]map[string]string
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

	// DockerfileTarget overrides the Dockerfile build target used for this
	// group's controller image. Empty means use the default `release` target.
	DockerfileTarget string

	// List of API object names that are reconciled by a controller.
	ReconciledApiObjectNames []string

	// The API objects that get CLI commands generated.
	TptctlModels []string

	// The API objects that have a CLI configuration that references a file on a
	// path on the filesystem.  Used to generate code to resolve that path
	// properly.
	TptctlConfigPathModels []string

	// The details for each API object in the group.  Contains an API object for
	// each version of each object.
	ApiObjects []*ApiObject

	// The details for each API object but not for each version of each object.
	UnversionedApiObjects []*UnversionedApiObject

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
	// An example of this data model with a KubernetesWorkloadDefinition is:
	// map["KubernetesWorkloadDefinition"]map["YAMLDocument"]map["validate"]"required"
	StructTags map[string]map[string]map[string]string

	// FieldTypes carries the Go type expression for every field captured
	// in StructTags, keyed by object name then field name. Tag validators
	// that need to verify a field's Go type read it here.
	FieldTypes map[string]map[string]string

	// StructEmbeds records the anonymous embed type names per struct in
	// this group. Keyed by the embedding struct's name; the value is the
	// list of type names embedded anonymously (e.g.
	// "KubernetesWorkloadInstance" -> ["Common", "Instance", "Reconciliation"]).
	// The collision check uses this to flatten embedded fields into the
	// effective-key set.
	StructEmbeds map[string][]string
}

// VersionedApiObjectCollection contains all API objects grouped by version and
// then grouped in the way the API is organized.
type VersionedApiObjectCollection struct {
	// The version for all API objects.
	Version string

	// The object groups organized by version.
	VersionedApiObjectGroups []VersionedApiObjectGroup
}

// VersionedApiObjectGroup contains all the API objects for a particular group
// of a particular version.
type VersionedApiObjectGroup struct {
	// Object group name in short kebab case, e.g. kubernetes-runtime
	Name string

	// The API objects for a particular version.
	ApiObjects []*ApiObject
}

// ApiObject contains the values for a particular model of a particular version.
type ApiObject struct {
	// The name of the go package where the API object's data models is defined.
	PackageName string

	// The version of the API object.
	Version string

	// The description of the API object as provided in the source code comments for
	// the API object.
	Description string

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

	// Will be set to true on an instance object that is a part of a defined
	// instance where the definition object has a Tptctl.ConfigPath set to true
	// in the SDK config.
	DefinedInstanceTptctlConfigPath bool

	// Only applied to definition objects - if true, there is a corresponding
	// instance object.  A defined instance abstraction will be generated.
	// Ref: https://threeport.io/concepts/definitions-instances/#defined-instance-abstractions
	DefinedInstanceDefinition bool

	// Only applied to instance objects - if true, there is a corresponding
	// definition object.  A defined instance abstraction will be generated.
	// Ref: https://threeport.io/concepts/definitions-instances/#defined-instance-abstractions
	DefinedInstanceInstance bool

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

// UnversionedApiObject represents one API object regardless of how many
// versions exist for that object.
type UnversionedApiObject struct {
	// All the available versions of this API object.
	Versions []string

	// The name of the object type.
	TypeName string
	// If true, generate tptctl commands for the model.
	TptctlCommands bool

	// If true, the config for the object, references another file and should
	// have code that includes passing the config path to config package object.
	TptctlConfigPath bool

	// Will be set to true on an instance object that is a part of a defined
	// instance where the definition object has a Tptctl.ConfigPath set to true
	// in the SDK config.
	DefinedInstanceTptctlConfigPath bool

	// Only applied to definition objects - if true, there is a corresponding
	// instance object.  A defined instance abstraction will be generated.
	// Ref: https://threeport.io/concepts/definitions-instances/#defined-instance-abstractions
	DefinedInstanceDefinition bool

	// Only applied to instance objects - if true, there is a corresponding
	// definition object.  A defined instance abstraction will be generated.
	// Ref: https://threeport.io/concepts/definitions-instances/#defined-instance-abstractions
	DefinedInstanceInstance bool
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

	// determine if an extension module and get module path from go.mod
	module, modulePath, err := sdkutil.IsModule()
	if err != nil {
		return fmt.Errorf("could not determine if generating code for a module: %w", err)
	}
	g.Module = module
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

		if version == "v0" && !module {
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

				if keyIndex == -1 && valueIndex == -1 && !module {
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

	/////////////////// populate Generator.EmbedTypes //////////////////////////
	// shared base types (Common, Definition, Instance, Reconciliation) live
	// in non-domain files. Parse them once up front so ValidateTags can
	// flatten anonymous-embed fields into the per-struct collision check.
	g.EmbedTypes = map[string]map[string]map[string]string{}
	for _, embedFile := range []string{"common.go", "class.go"} {
		embedPath := filepath.Join("pkg", "api", "v0", embedFile)
		embedFset := token.NewFileSet()
		embedAST, err := parser.ParseFile(embedFset, embedPath, nil, parser.ParseComments|parser.AllErrors)
		if err != nil {
			// missing file is fine on modules that don't carry the shared
			// types; only a real parse error should fail the run
			continue
		}
		// walk every top-level struct type, record each field's tag map
		// keyed by struct name. mirrors the per-model-file parser below,
		// minus the ApiObject wiring (we only need tag info here).
		for _, decl := range embedAST.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				typeName := typeSpec.Name.Name
				g.EmbedTypes[typeName] = map[string]map[string]string{}
				for _, field := range structType.Fields.List {
					// anon embeds within an embed type are out of scope;
					// the only embeds api types are allowed to use are the
					// flat base types parsed here. The ValidateTags whitelist
					// check enforces that constraint.
					if len(field.Names) == 0 {
						continue
					}
					fieldName := field.Names[0].Name
					if field.Tag == nil {
						g.EmbedTypes[typeName][fieldName] = map[string]string{}
						continue
					}
					g.EmbedTypes[typeName][fieldName] = util.ParseStructTag(field.Tag.Value)
				}
			}
		}
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
						mc.DefinedInstanceDefinition = true
					}
				}

				// if an instance object, determine if a part of a
				// DefinedInstance abstraction
				if strings.HasSuffix(*obj.Name, "Instance") {
					definedInstance, definitionName, _ := sdk.IsOfDefinedInstance(
						*obj.Name,
						apiObjectGroup.Objects,
					)
					if definedInstance {
						mc.DefinedInstanceInstance = true
						// determine if the definition paired with this instance
						// has a TptctlConfigPath set in SDK config
						definitionConfigObject, err := sdk.ApiObjectFromGroup(
							definitionName,
							apiObjectGroup,
						)
						if err != nil {
							return fmt.Errorf(
								"failed to get definition for instance in defined instance pair: %w",
								err,
							)
						}
						if definitionConfigObject.Tptctl != nil && definitionConfigObject.Tptctl.ConfigPath != nil {
							if *definitionConfigObject.Tptctl.ConfigPath {
								mc.DefinedInstanceTptctlConfigPath = true
							}
						}
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

			// create a comment map to associate comments with AST nodes
			commentMap := ast.NewCommentMap(fset, pf, pf.Comments)

			// determine which objects must be reconciled and build a map
			// of struct tags for each object
			structTags := make(map[string]map[string]map[string]string)
			fieldTypes := make(map[string]map[string]string)
			// record anonymous embed type names per struct so ValidateTags
			// can flatten embedded fields into the collision check
			structEmbeds := make(map[string][]string)

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

								// extract comment description for the struct type
								if mc != nil {
									if commentGroups, exists := commentMap[genDecl]; exists && len(commentGroups) > 0 {
										// extract the comment text from the first comment group and clean it up
										commentText := commentGroups[0].Text()
										commentText = strings.TrimSpace(commentText)
										// normalize whitespace and remove unnecessary line breaks
										commentText = strings.ReplaceAll(commentText, "\n", " ")
										commentText = strings.ReplaceAll(commentText, "\r", " ")
										// replace multiple consecutive spaces with single space
										for strings.Contains(commentText, "  ") {
											commentText = strings.ReplaceAll(commentText, "  ", " ")
										}
										commentText = strings.TrimSpace(commentText)
										mc.Description = commentText
									}
								}

								structTags[objectName] = make(map[string]map[string]string)
								fieldTypes[objectName] = make(map[string]string)

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

									// anonymous embed: record the embed type name on
									// this struct, then move on. The embed's own field
									// tags live in g.EmbedTypes (parsed once up front).
									if len(field.Names) == 0 {
										// anon embed type is either a bare identifier
										// (e.g. `Common`) or a selector (e.g.
										// `pkgalias.SomeType`); only the bare-ident case
										// applies to threeport's in-package embeds
										if ident, ok := field.Type.(*ast.Ident); ok {
											structEmbeds[objectName] = append(structEmbeds[objectName], ident.Name)
										}
										continue
									}
									fieldName := field.Names[0].Name
									if field.Tag == nil {
										return fmt.Errorf(
											"field %s in object %s has no struct tags defined",
											fieldName, objectName,
										)
									}
									tagMap := util.ParseStructTag(field.Tag.Value)
									structTags[objectName][fieldName] = tagMap
									fieldTypes[objectName][fieldName] = types.ExprString(field.Type)
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
				DockerfileTarget:         apiObjectGroup.DockerfileTarget,
				ApiObjects:               apiObjects,
				ReconciledApiObjectNames: reconcilerModels,
				TptctlModels:             tptctlModels,
				TptctlConfigPathModels:   tptctlModelsConfigPath,
				StructEmbeds:             structEmbeds,
				StructTags:               structTags,
				FieldTypes:               fieldTypes,
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
						if !mc.ReconciledField && !module {
							return fmt.Errorf(
								"%s object does not include a Reconciled field - all objects with reconcilers must include this field", rm,
							)
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

	// add `ApiObjects` to `UnversionedApiObjects` with all versions
	// listed for each.  This is used in tptctl command generation where we need
	// each unique API object with all its versions listed together.
	for i, objGroup := range g.ApiObjectGroups {
		var unversionedObjects []*UnversionedApiObject
		for _, obj := range objGroup.ApiObjects {
			foundUnversioned := false
			for j, uvObj := range unversionedObjects {
				if uvObj.TypeName == obj.TypeName {
					foundUnversioned = true
					unversionedObjects[j].Versions = append(
						unversionedObjects[j].Versions,
						obj.Version,
					)
				}
			}
			if !foundUnversioned {
				unversionedObj := UnversionedApiObject{
					Versions:                        []string{obj.Version},
					TypeName:                        obj.TypeName,
					TptctlCommands:                  obj.TptctlCommands,
					TptctlConfigPath:                obj.TptctlConfigPath,
					DefinedInstanceInstance:         obj.DefinedInstanceInstance,
					DefinedInstanceDefinition:       obj.DefinedInstanceDefinition,
					DefinedInstanceTptctlConfigPath: obj.DefinedInstanceTptctlConfigPath,
				}
				unversionedObjects = append(
					unversionedObjects,
					&unversionedObj,
				)
			}
		}
		g.ApiObjectGroups[i].UnversionedApiObjects = unversionedObjects
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

// HasFieldWithTagValue reports whether any field on the named object
// carries a struct tag with the given key set to the expected value.
// Unlike CheckStructTagMap, which targets a single named field, this
// search is field-agnostic — useful when codegen behavior is driven by
// the presence of a tag anywhere on the object (e.g. persist:"false").
func (a *ApiObjectGroup) HasFieldWithTagValue(
	object,
	tagKey,
	expectedTagValue string,
) bool {
	fieldTagMap, objectKeyFound := a.StructTags[object]
	if !objectKeyFound {
		return false
	}
	for _, tagValueMap := range fieldTagMap {
		if tagValue, tagKeyFound := tagValueMap[tagKey]; tagKeyFound {
			if tagValue == expectedTagValue {
				return true
			}
		}
	}
	return false
}

// allowedEmbed is the set of base types api models are permitted to embed
// anonymously. Keeping the set small enforces a flat, one-level mental
// model: model authors only ever embed one of these, and readers can
// scan an api type top to bottom without chasing arbitrary embed chains.
var allowedEmbed = map[string]bool{
	"Common":         true,
	"Definition":     true,
	"Instance":       true,
	"Reconciliation": true,
}

// allowedEmbedNames is the human-readable list of allowed embeds for
// error messages. Kept in sync with allowedEmbed.
const allowedEmbedNames = "Common, Definition, Instance, or Reconciliation"

// ValidateTags walks every API object's struct tags and returns a non-nil
// error if any threeport-specific tag has an invalid value.
//
// Validates:
//   - relationship: kind is recognized; modifier keys are recognized;
//     the target type (`type:<TypeName>` modifier or the field name minus
//     its "ID" suffix) names a registered API object; field type is *uint
//   - encrypt: value matches EncryptTrue
//   - validate: value matches a recognized validator value
//   - persist: value matches PersistFalse (true is the default; omit the tag)
//   - query: value matches queryNamePattern
func (g *Generator) ValidateTags() error {
	// build the set of registered API type names so the relationship
	// validator can verify that any `type:<TypeName>` modifier names a
	// real type
	knownTypes := map[string]bool{}
	for _, group := range g.ApiObjectGroups {
		for _, obj := range group.ApiObjects {
			knownTypes[obj.TypeName] = true
		}
	}

	// accumulate one human-readable problem string per invalid tag so a
	// single run reports every issue rather than failing on the first
	var problems []string
	for _, group := range g.ApiObjectGroups {
		for objectName, fieldMap := range group.StructTags {
			for fieldName, tagMap := range fieldMap {
				// relationship tags carry sub-structure; delegate to a
				// helper that splits and validates kind + modifiers
				if rel, ok := tagMap[string(lib.RelationshipTag)]; ok && rel != "" {
					fieldType := group.FieldTypes[objectName][fieldName]
					problems = append(problems,
						validateRelationshipTag(objectName, fieldName, fieldType, rel, knownTypes, g.Module)...)
				}
				// encrypt is a single-value flag; only EncryptTrue is meaningful
				if enc, ok := tagMap[string(lib.EncryptTag)]; ok && enc != lib.EncryptTrue {
					problems = append(problems, fmt.Sprintf(
						"%s.%s: %s:%q invalid (only %q allowed)",
						objectName, fieldName,
						lib.EncryptTag, enc, lib.EncryptTrue,
					))
				}
				// validate accepts one of three known values; anything else
				// is a typo or a stale value and must be flagged
				if val, ok := tagMap[string(lib.ValidateTag)]; ok &&
					val != string(lib.ValidateRequired) &&
					val != string(lib.ValidateOptional) &&
					val != string(lib.ValidateOptionalAssociation) {
					problems = append(problems, fmt.Sprintf(
						"%s.%s: %s:%q invalid (allowed: %q, %q, %q)",
						objectName, fieldName,
						lib.ValidateTag, val,
						lib.ValidateRequired, lib.ValidateOptional, lib.ValidateOptionalAssociation,
					))
				}
				// every validate-tagged field must carry json:",omitempty".
				// the field-name part is dropped (Go default is the field
				// name itself); the omitempty matters for partial PATCH
				// payloads. Without it, a nil-pointer required field would
				// serialize as JSON null and the PayloadCheck null-on-required
				// guard would reject the request, even when the caller never
				// meant to touch that field. Required, optional, and
				// optional-association all follow the same rule.
				validateValue := tagMap[string(lib.ValidateTag)]
				if validateValue == string(lib.ValidateRequired) ||
					validateValue == string(lib.ValidateOptional) ||
					validateValue == string(lib.ValidateOptionalAssociation) {
					j, ok := tagMap[string(lib.JsonTag)]
					if !ok || !strings.Contains(j, lib.JsonOmitempty) {
						problems = append(problems, fmt.Sprintf(
							"%s.%s: %s:%q field requires json:%q",
							objectName, fieldName,
							lib.ValidateTag, validateValue, ","+lib.JsonOmitempty,
						))
					}
				}
				// persist defaults to true — only PersistFalse opts out;
				// any other value (including an explicit "true") is noise
				// and likely indicates a misunderstanding
				if per, ok := tagMap[string(lib.PersistTag)]; ok && per != lib.PersistFalse {
					problems = append(problems, fmt.Sprintf(
						"%s.%s: %s:%q invalid (only %q is meaningful; omit the tag for the default)",
						objectName, fieldName,
						lib.PersistTag, per, lib.PersistFalse,
					))
				}
				// query tag is forbidden. URL parameter keys derive from
				// the lowercased Go field name automatically, so an
				// explicit override is redundant and a silent rename hazard
				if _, ok := tagMap[string(lib.QueryTag)]; ok {
					problems = append(problems, fmt.Sprintf(
						"%s.%s: %s tag is not allowed; query keys derive from the lowercased field name",
						objectName, fieldName, lib.QueryTag,
					))
				}
			}

			// reject api type embeds outside the allowed base-type set.
			// keeping the allowed set small (Common, Definition, Instance,
			// Reconciliation) means model authors only have to reason about
			// one level of embed promotion, and the validator only has to
			// flatten one level deep.
			for _, embedType := range group.StructEmbeds[objectName] {
				if !allowedEmbed[embedType] {
					problems = append(problems, fmt.Sprintf(
						"%s embeds %s: api types may only embed %s",
						objectName, embedType, allowedEmbedNames,
					))
				}
			}
		}
	}

	if len(problems) > 0 {
		// sort so the error output is stable across runs — map iteration
		// order is otherwise random and would make the message jitter
		sort.Strings(problems)
		return fmt.Errorf(
			"%d invalid tag(s):\n  %s",
			len(problems), strings.Join(problems, "\n  "),
		)
	}
	return nil
}

// ParseRelationshipTagValue splits a relationship tag value of the form
// `<kind>[;<key>:<value>...]` into its kind and modifier key/value pairs,
// returning any malformed modifier entries separately.
func ParseRelationshipTagValue(rel string) (kind string, modifiers map[string]string, malformed []string) {
	// the kind is the first `;`-separated segment; everything after it is
	// a modifier list. Split on `;` rather than parsing greedily so a
	// missing modifier list (just `<kind>`) still parses cleanly with an
	// empty modifier slice
	parts := strings.Split(rel, ";")
	kind = parts[0]
	modifiers = make(map[string]string)
	for _, p := range parts[1:] {
		// each modifier is `key:value`; preserve malformed entries so the
		// caller can report them rather than silently dropping garbage
		k, v, ok := strings.Cut(p, ":")
		if !ok {
			malformed = append(malformed, p)
			continue
		}
		modifiers[k] = v
	}
	return
}

// validateRelationshipTag returns one error per problem in a relationship
// tag value. In module mode, types outside the local set are presumed to
// be in threeport core (an already-built imported package) and verified
// when the module compiles.
func validateRelationshipTag(object, field, fieldType, rel string, knownTypes map[string]bool, module bool) []string {
	var problems []string
	kind, modifiers, malformed := ParseRelationshipTagValue(rel)

	// kind anchors the whole tag — flag anything that isn't one of the
	// recognized values up front so downstream emission can rely on
	// the value being safe
	switch api.Relationship(kind) {
	case api.RelationshipRequires, api.RelationshipOwns, api.RelationshipDescribes, api.RelationshipMarries:
		// valid
	default:
		problems = append(problems, fmt.Sprintf(
			"%s.%s: invalid relationship kind %q (expected %q, %q, %q, or %q)",
			object, field, kind,
			api.RelationshipRequires, api.RelationshipOwns, api.RelationshipDescribes, api.RelationshipMarries,
		))
	}
	// relationship-tagged fields are emitted as *uint foreign keys; a
	// non-*uint field would produce generated code that fails to compile
	if fieldType != "*uint" {
		problems = append(problems, fmt.Sprintf(
			"%s.%s: relationship tag requires field type *uint, got %q",
			object, field, fieldType,
		))
	}
	// surface malformed modifier entries (no colon) verbatim; they were
	// preserved by the parser specifically so the caller can report them
	for _, m := range malformed {
		problems = append(problems, fmt.Sprintf(
			"%s.%s: malformed relationship modifier %q (expected key:value)",
			object, field, m,
		))
	}
	// only the type modifier is currently meaningful; reject any other key
	// to catch typos early. When new modifiers are added, extend this
	// switch rather than letting unknown keys through silently
	for k, v := range modifiers {
		switch k {
		case lib.RelationshipTypeKey:
			// in module mode, types may live in the imported threeport
			// core package; the Go compiler verifies them at build time
			if !knownTypes[v] && !module {
				problems = append(problems, fmt.Sprintf(
					"%s.%s: relationship references unknown API type %q",
					object, field, v,
				))
			}
		default:
			problems = append(problems, fmt.Sprintf(
				"%s.%s: unknown relationship modifier key %q (only %q is supported)",
				object, field, k, lib.RelationshipTypeKey,
			))
		}
	}
	// when there's no `type:` modifier, the target type is derived by
	// stripping the field's "ID" suffix. Reject anything the derivation
	// can't resolve to a registered API type
	if _, hasTypeModifier := modifiers[lib.RelationshipTypeKey]; !hasTypeModifier {
		if !strings.HasSuffix(field, "ID") {
			problems = append(problems, fmt.Sprintf(
				"%s.%s: relationship-tagged field name must end in %q or include a %q modifier",
				object, field, "ID", lib.RelationshipTypeKey,
			))
		} else {
			derived := strings.TrimSuffix(field, "ID")
			if !knownTypes[derived] && !module {
				problems = append(problems, fmt.Sprintf(
					"%s.%s: relationship references unknown API type %q (derived from field name; add %q modifier to override)",
					object, field, derived, lib.RelationshipTypeKey,
				))
			}
		}
	}
	return problems
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
