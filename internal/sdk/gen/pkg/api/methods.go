package api

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenApiObjectMethods generates the source code for the API objects constants
// and methods.
func GenApiObjectMethods(gen *gen.Generator) error {
	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			pluralize := pluralize.NewClient()
			f := NewFile(objCollection.Version)
			f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

			f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")
			f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpv0")

			// object type constants
			objectTypes := &Statement{}
			for _, mc := range objGroup.ApiObjects {
				objectTypes.Id(fmt.Sprintf(
					"ObjectType%s", mc.TypeName,
				)).String().Op("=").Lit(mc.TypeName)
				objectTypes.Line()
			}
			paths := &Statement{}
			for _, mc := range objGroup.ApiObjects {
				paths.Id("Path" + pluralize.Pluralize(mc.TypeName, 2, false)).Op("=").Lit(
					fmt.Sprintf("/%s/%s", objCollection.Version, pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false)),
				)
				paths.Line()
			}
			f.Const().Defs(
				objectTypes,
				Line(),
				paths,
			)
			f.Line()

			// API object methods
			for _, mc := range objGroup.ApiObjects {
				// NotificationPayload method
				f.Comment("NotificationPayload returns the notification payload that is delivered to the")
				f.Comment("controller when a change is made.  It includes the object as presented by the")
				f.Comment("client when the change was made.")
				f.Func().Params(
					Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
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
						Id("Operation"):    Id("operation"),
						Id("CreationTime"): Op("&").Id("creationTime"),
						Id("Object"):       Id(util.TypeAbbrev(mc.TypeName)),
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
								Id(util.TypeAbbrev(mc.TypeName)),
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
					Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
				).Id("DecodeNotifObject").Params(Id("object").Interface()).Error().Block(
					List(Id("jsonObject"), Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(Id("object")),
					If(Id("err").Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(
							Lit("failed to marshal object map from consumed notification message: %w"), Id("err")),
						),
					),
					If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
						Id("jsonObject"), Op("&").Id(util.TypeAbbrev(mc.TypeName)),
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
					Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
				).Id("GetId").Params().Uint().Block(
					Return(Op("*").Id(util.TypeAbbrev(mc.TypeName)).Dot("ID")),
				)
				// Type method
				f.Comment("Type returns the object type.")
				f.Func().Params(
					Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
				).Id("GetType").Params().String().Block(
					Return(Lit(mc.TypeName)),
				)
				// Version method
				f.Comment("Version returns the version of the API object.")
				f.Func().Params(
					Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
				).Id("GetVersion").Params().String().Block(
					Return(Lit(objCollection.Version)),
				)
				// ScheduledForDeletion method
				if mc.Reconciler {
					f.Comment("ScheduledForDeletion returns a pointer to the DeletionScheduled timestamp")
					f.Comment("if scheduled for deletion or nil if not scheduled for deletion.")
					f.Func().Params(
						Id(util.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
					).Id("ScheduledForDeletion").Params().Op("*").Qual("time", "Time").Block(
						Return(Id(util.TypeAbbrev(mc.TypeName)).Dot("DeletionScheduled")),
					)
				}
			}

			// write code to file
			genFilepath := filepath.Join(
				"pkg",
				"api",
				objCollection.Version,
				fmt.Sprintf("%s_gen.go", strcase.ToSnake(objGroup.Name)),
			)
			_, err := util.WriteCodeToFile(f, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf("source code for API object methods written to %s", genFilepath))
		}
	}

	return nil
}
