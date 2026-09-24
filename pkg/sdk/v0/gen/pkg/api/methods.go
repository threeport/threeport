package api

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	api "github.com/threeport/threeport/pkg/api/v0"
	lib "github.com/threeport/threeport/pkg/api/lib/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	sdkgen "github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenApiObjectMethods generates the source code for the API objects constants
// and methods.
func GenApiObjectMethods(gen *sdkgen.Generator, sdkConfig *sdk.SdkConfig) error {
	pluralize := pluralize.NewClient()

	// flatten StructTags so we can look up an API type's relationship-tagged
	// fields by TypeName when emitting ForeignKeys methods
	typeToTags := make(map[string]map[string]map[string]string)
	for _, group := range gen.ApiObjectGroups {
		for typeName, tagMap := range group.StructTags {
			typeToTags[typeName] = tagMap
		}
	}

	// types declared locally; relationship targets outside this set are
	// module-only references into threeport core
	localTypes := map[string]bool{}
	for _, group := range gen.ApiObjectGroups {
		for _, obj := range group.ApiObjects {
			localTypes[obj.TypeName] = true
		}
	}

	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			f := NewFilePath(fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version))
			f.HeaderComment(sdk.HeaderCommentGenNoEdit)

			f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")
			f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "api")
			f.ImportAlias("github.com/threeport/threeport/pkg/api/lib/v0", "lib")
			f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")

			// object type constants
			objectTypes := &Statement{}
			for _, apiObj := range objGroup.ApiObjects {
				objectTypes.Id(fmt.Sprintf(
					"ObjectType%s", apiObj.TypeName,
				)).String().Op("=").Lit(apiObj.TypeName)
				objectTypes.Line()
			}

			// object REST path constants
			paths := &Statement{}
			for _, apiObj := range objGroup.ApiObjects {
				if gen.Module {
					paths.Id(fmt.Sprintf(
						"Path%sVersions",
						apiObj.TypeName,
					)).Op("=").Lit(fmt.Sprintf(
						"/%s/%s/versions",
						util.RestPath(sdkConfig.ApiNamespace),
						pluralize.Pluralize(strcase.ToKebab(apiObj.TypeName), 2, false),
					))
					paths.Line()
					paths.Id(fmt.Sprintf(
						"Path%s",
						pluralize.Pluralize(apiObj.TypeName, 2, false),
					)).Op("=").Lit(fmt.Sprintf(
						"/%s/%s/%s",
						util.RestPath(sdkConfig.ApiNamespace),
						objCollection.Version,
						pluralize.Pluralize(strcase.ToKebab(apiObj.TypeName), 2, false),
					))
					paths.Line()
				} else {
					paths.Id(fmt.Sprintf(
						"Path%sVersions",
						apiObj.TypeName,
					)).Op("=").Lit(fmt.Sprintf(
						"/%s/versions",
						pluralize.Pluralize(strcase.ToKebab(apiObj.TypeName), 2, false),
					))
					paths.Line()
					paths.Id(fmt.Sprintf(
						"Path%s",
						pluralize.Pluralize(apiObj.TypeName, 2, false),
					)).Op("=").Lit(fmt.Sprintf(
						"/%s/%s",
						objCollection.Version,
						pluralize.Pluralize(strcase.ToKebab(apiObj.TypeName), 2, false),
					))
					paths.Line()
				}
			}
			f.Const().Defs(
				objectTypes,
				Line(),
				paths,
			)
			f.Line()

			// API object methods
			for _, apiObj := range objGroup.ApiObjects {
				// NotificationPayload method
				f.Comment("NotificationPayload returns the notification payload that is delivered to the")
				f.Comment("controller when a change is made.  It includes the object as presented by the")
				f.Comment("client when the change was made.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("NotificationPayload").Params(
					Line().Id("operation").Qual(
						"github.com/threeport/threeport/pkg/notifications/v0",
						"NotificationOperation",
					),
					Line().Id("requeue").Bool(),
					Line().Id("creationTime").Int64(),
					Line(),
				).Parens(List(
					Op("*").Index().Byte(),
					Error(),
				)).Block(
					Id("notif").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/notifications/v0",
						"Notification",
					).Values(Dict{
						Id("Operation"):     Id("operation"),
						Id("CreationTime"):  Op("&").Id("creationTime"),
						Id("Object"):        Id(util.TypeAbbrev(apiObj.TypeName)),
						Id("ObjectVersion"): Id(util.TypeAbbrev(apiObj.TypeName)).Dot("GetVersion").Call(),
					}),
					Line(),
					List(
						Id("payload"), Err(),
					).Op(":=").Qual("encoding/json", "Marshal").Call(Id("notif")),
					If(
						Err().Op("!=").Nil(),
					).Block(
						Return(List(
							Op("&").Id("payload"),
							Qual("fmt", "Errorf").Call(
								Lit("failed to marshal notification payload %+v: %w"),
								Id(util.TypeAbbrev(apiObj.TypeName)),
								Err(),
							),
						)),
					),
					Line(),
					Return(
						Op("&").Id("payload"),
						Nil(),
					),
				)
				f.Line()

				// DecodeNotifObject method
				f.Comment("DecodeNotifObject takes the threeport object in the form of a")
				f.Comment("map[string]interface and returns the typed object by marshalling into JSON")
				f.Comment("and then unmarshalling into the typed object.  We are not using the")
				f.Comment("mapstructure library here as that requires custom decode hooks to manage")
				f.Comment("fields with non-native go types.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("DecodeNotifObject").Params(Id("object").Interface()).Error().Block(
					List(Id("jsonObject"), Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(Id("object")),
					If(Id("err").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit("failed to marshal object map from consumed notification message: %w"), Id("err")),
						),
					),
					If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
						Id("jsonObject"), Op("&").Id(util.TypeAbbrev(apiObj.TypeName)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit("failed to unmarshal json object to typed object: %w"), Id("err"),
						)),
					),
					Return(Nil()),
				)
				// GetId method
				f.Comment("GetId returns the unique ID for the object.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("GetId").Params().Uint().Block(
					Return(Op("*").Id(util.TypeAbbrev(apiObj.TypeName)).Dot("ID")),
				)
				// GetType method - bare TypeName, same shape for core
				// and modules. For the API-namespace-qualified form
				// (used as the AOR identity string) use
				// GetFullyQualifiedType instead
				f.Comment("GetType returns the object type.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("GetType").Params().String().Block(
					Return(Lit(apiObj.TypeName)),
				)
				// GetVersion method
				f.Comment("GetVersion returns the version of the API object.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("GetVersion").Params().String().Block(
					Return(Lit(objCollection.Version)),
				)
				// GetFullyQualifiedType method - returns
				// "<api-namespace>/<version>.<TypeName>". The namespace
				// is resolved at gen time (core uses "threeport.io",
				// modules use their configured ApiNamespace) and baked
				// into the returned literal.
				namespace := "threeport.io"
				if gen.Module {
					namespace = sdkConfig.ApiNamespace
				}
				fullyQualifiedTypeLiteral := fmt.Sprintf(
					"%s/%s.%s",
					namespace,
					objCollection.Version,
					apiObj.TypeName,
				)
				f.Comment("GetFullyQualifiedType returns the API-namespace-qualified type name.")
				f.Func().Params(
					Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
				).Id("GetFullyQualifiedType").Params().String().Block(
					Return(Lit(fullyQualifiedTypeLiteral)),
				)
				// ScheduledForDeletion method
				if apiObj.Reconciler {
					f.Comment("ScheduledForDeletion returns a pointer to the DeletionScheduled timestamp")
					f.Comment("if scheduled for deletion or nil if not scheduled for deletion.")
					f.Func().Params(
						Id(util.TypeAbbrev(apiObj.TypeName)).Op("*").Id(apiObj.TypeName),
					).Id("ScheduledForDeletion").Params().Op("*").Qual("time", "Time").Block(
						Return(Id(util.TypeAbbrev(apiObj.TypeName)).Dot("DeletionScheduled")),
					)
				}

				// RelationshipTaggedForeignKeys method emitted for any type
				// with at least one relationship-tagged FK
				{
					type fkEntry struct {
						fieldName    string
						objectType   string
						relationship string
					}
					var foreignKeys []fkEntry
					if tagsForType, ok := typeToTags[apiObj.TypeName]; ok {
						for fieldName, tagMap := range tagsForType {
							rel, ok := tagMap[string(lib.RelationshipTag)]
							if !ok || rel == "" {
								continue
							}
							kind, modifiers, _ := sdkgen.ParseRelationshipTagValue(rel)
							objectType := strings.TrimSuffix(fieldName, "ID")
							if v, ok := modifiers[lib.RelationshipTypeKey]; ok {
								objectType = v
							}
							foreignKeys = append(foreignKeys, fkEntry{
								fieldName:    fieldName,
								objectType:   objectType,
								relationship: kind,
							})
						}
						sort.Slice(foreignKeys, func(i, j int) bool {
							return foreignKeys[i].fieldName < foreignKeys[j].fieldName
						})
					}
					if len(foreignKeys) > 0 {
						receiver := strings.ToLower(string(apiObj.TypeName[0]))
						foreignKeyType := Qual("github.com/threeport/threeport/pkg/api/v0", "RelationshipTaggedForeignKey")
						f.Comment(fmt.Sprintf(
							"RelationshipTaggedForeignKeys returns the relationship-tagged foreign keys on %s.",
							apiObj.TypeName,
						))
						f.Func().Params(
							Id(receiver).Op("*").Id(apiObj.TypeName),
						).Id("RelationshipTaggedForeignKeys").Params().Index().Add(foreignKeyType).BlockFunc(func(g *Group) {
							g.Return().Index().Add(foreignKeyType).ValuesFunc(func(vg *Group) {
								for _, fk := range foreignKeys {
									var relationshipQual *Statement
									switch api.Relationship(fk.relationship) {
									case api.RelationshipOwns:
										relationshipQual = Qual("github.com/threeport/threeport/pkg/api/v0", "RelationshipOwns")
									case api.RelationshipDescribes:
										relationshipQual = Qual("github.com/threeport/threeport/pkg/api/v0", "RelationshipDescribes")
									case api.RelationshipMarries:
										relationshipQual = Qual("github.com/threeport/threeport/pkg/api/v0", "RelationshipMarries")
									default:
										relationshipQual = Qual("github.com/threeport/threeport/pkg/api/v0", "RelationshipRequires")
									}
								// reference the target type either locally or via Qual for
								// a cross-module ref (which is always to a core type, since
								// modules can't ref other modules)
								var targetTypeRef *Statement
								if gen.Module && !localTypes[fk.objectType] {
									targetTypeRef = Qual(
										"github.com/threeport/threeport/pkg/api/v0",
										fk.objectType,
									)
								} else {
									targetTypeRef = Id(fk.objectType)
								}
								vg.Values(Dict{
									Id("FieldName"): Lit(fk.fieldName),
									// call GetFullyQualifiedType on the target type
									// so the call site reads as intent ("identify rows
									// in the AOR table by qualified type") and the
									// reader can hop to the method to see the literal
									Id("ObjectType"): Id("new").Call(targetTypeRef).
										Dot("GetFullyQualifiedType").Call(),
									Id("Relationship"): relationshipQual,
									Id("ObjectID"):     Id(receiver).Dot(fk.fieldName),
								})
								}
							})
						})
						f.Line()
					}
				}

				// EncryptedFields method emitted for any type with at least
				// one encrypt-tagged field
				{
					var encryptedFields []string
					if tagsForType, ok := typeToTags[apiObj.TypeName]; ok {
						for fieldName, tagMap := range tagsForType {
							if tagMap[string(lib.EncryptTag)] != lib.EncryptTrue {
								continue
							}
							encryptedFields = append(encryptedFields, fieldName)
						}
						sort.Strings(encryptedFields)
					}
					if len(encryptedFields) > 0 {
						receiver := strings.ToLower(string(apiObj.TypeName[0]))
						encryptedFieldType := Qual("github.com/threeport/threeport/pkg/api/lib/v0", "EncryptedField")
						f.Comment(fmt.Sprintf(
							"EncryptedFields returns the encrypt-tagged fields on %s.",
							apiObj.TypeName,
						))
						f.Func().Params(
							Id(receiver).Op("*").Id(apiObj.TypeName),
						).Id("EncryptedFields").Params().Index().Add(encryptedFieldType).BlockFunc(func(g *Group) {
							g.Return().Index().Add(encryptedFieldType).ValuesFunc(func(vg *Group) {
								for _, fieldName := range encryptedFields {
									vg.Values(Dict{
										Id("Name"):  Lit(fieldName),
										Id("Value"): Id(receiver).Dot(fieldName),
									})
								}
							})
						})
						f.Line()
					}
				}

				// PersistFalseFields method emitted for any type with at
				// least one persist:"false"-tagged field
				{
					var persistFalseFields []string
					if tagsForType, ok := typeToTags[apiObj.TypeName]; ok {
						for fieldName, tagMap := range tagsForType {
							if tagMap[string(lib.PersistTag)] != lib.PersistFalse {
								continue
							}
							persistFalseFields = append(persistFalseFields, fieldName)
						}
						sort.Strings(persistFalseFields)
					}
					if len(persistFalseFields) > 0 {
						receiver := strings.ToLower(string(apiObj.TypeName[0]))
						persistFalseFieldType := Qual("github.com/threeport/threeport/pkg/api/lib/v0", "PersistFalseField")
						f.Comment(fmt.Sprintf(
							"PersistFalseFields returns the persist:\"false\"-tagged fields on %s.",
							apiObj.TypeName,
						))
						f.Func().Params(
							Id(receiver).Op("*").Id(apiObj.TypeName),
						).Id("PersistFalseFields").Params().Index().Add(persistFalseFieldType).BlockFunc(func(g *Group) {
							g.Return().Index().Add(persistFalseFieldType).ValuesFunc(func(vg *Group) {
								for _, fieldName := range persistFalseFields {
									vg.Values(Dict{
										Id("Name"): Lit(fieldName),
									})
								}
							})
						})
						f.Line()
					}
				}
			}

			// write code to file if not excluded by SDK config
			genFilepath := filepath.Join(
				"pkg",
				"api",
				objCollection.Version,
				fmt.Sprintf("%s_gen.go", strcase.ToSnake(objGroup.Name)),
			)
			if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
				cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
			} else {
				_, err := util.WriteCodeToFile(f, genFilepath, true)
				if err != nil {
					return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
				}
				cli.Info(fmt.Sprintf("source code for API object methods written to %s", genFilepath))
			}
		}
	}

	return nil
}

