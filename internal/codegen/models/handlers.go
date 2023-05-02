package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/codegen"
)

// deletionInstanceCheckTypeNames returns the definition objects that need to
// have related instances checked when deleting
func deletionInstanceCheckTypeNames() []string {
	return []string{"WorkloadDefinition"}
}

// apiHandlersPath returns the path from the models to the API's internal handlers
// package.
func apiHandlersPath() string {
	return filepath.Join("..", "..", "..", "internal", "api", "handlers")
}

// ModelHandlers generates the handlers for each model.
func (cc *ControllerConfig) ModelHandlers() error {
	pluralize := pluralize.NewClient()
	f := NewFile("handlers")
	f.HeaderComment("generated by 'threeport-codegen api-model' - do not edit")
	f.ImportAlias("github.com/labstack/echo/v4", "echo")
	f.ImportAlias("github.com/threeport/threeport/internal/api", "iapi")

	for _, mc := range cc.ModelConfigs {
		// delete handler includes instance check for definition objects to ensure
		// no definitions with related instances get deleted
		instanceCheck := false
		for _, typeName := range deletionInstanceCheckTypeNames() {
			if mc.TypeName == typeName {
				instanceCheck = true
			}
		}
		deleteObjectHandler := &Statement{}
		if instanceCheck {
			instancesName := strings.TrimSuffix(mc.TypeName, "Definition") + "Instances"
			deleteObjectHandler = If(
				Id("result").Op(":=").Id("h").Dot("DB").Dot("Preload").Call(
					Lit(instancesName),
				).Dot("First").Call(Op("&").Id(
					strcase.ToLowerCamel(mc.TypeName),
				).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
			).Line()
			deleteObjectHandler.Line()
			deleteObjectHandler.Comment("check to make sure no dependent instances exist for this definition")
			deleteObjectHandler.Line()
			deleteObjectHandler.If(
				Len(Id(strcase.ToLowerCamel(mc.TypeName)).Dot(instancesName)).Op("!=").Lit(0).Block(
					Id("err").Op(":=").Qual("errors", "New").Call(
						Lit(fmt.Sprintf(
							"%s has related %s - cannot be deleted",
							strcase.ToDelimited(mc.TypeName, ' '),
							strcase.ToDelimited(instancesName, ' '),
						)),
					),
					Return().Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus409",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType")),
				),
			)
			deleteObjectHandler.Line()
		} else {
			deleteObjectHandler = If(
				Id("result").Op(":=").Id("h").Dot("DB").
					Dot("First").Call(Op("&").Id(
					strcase.ToLowerCamel(mc.TypeName),
				).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
			).Line()
		}

		f.Comment("///////////////////////////////////////////////////////////////////////////////")
		f.Comment(mc.TypeName)
		f.Comment("///////////////////////////////////////////////////////////////////////////////")
		f.Line()
		// get object versions
		f.Comment(fmt.Sprintf(
			"@Summary %s gets the supported versions for the %s API.",
			mc.GetVersionHandlerName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Get the supported API versions for %s.",
			pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false),
		))
		f.Comment(fmt.Sprintf(
			"@ID %s-get-versions", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Produce json")
		f.Comment("@Success 200 {object} api.RESTAPIVersions \"OK\"")
		f.Comment(fmt.Sprintf(
			"@Router /%s/versions [get]", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.GetVersionHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Return(
				Id("c").Dot("JSON").Call(
					Qual("net/http", "StatusOK"),
					Qual(
						"github.com/threeport/threeport/pkg/api",
						"RestapiVersions",
					).Index(String().Call(Qual(
						fmt.Sprintf(
							"github.com/threeport/threeport/pkg/api/%s",
							cc.ParsedModelFile.Name.Name,
						),
						fmt.Sprintf(
							"ObjectType%s", mc.TypeName,
						),
					))),
				),
			),
		)
		// add object handler
		f.Comment(fmt.Sprintf(
			"@Summary adds a new %s.", strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Add a new %s to the Threeport database.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@ID add-%s", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment(fmt.Sprintf(
			"@Param %[1]s body %[2]s.%[3]s true \"%[3]s object\"",
			strcase.ToLowerCamel(mc.TypeName),
			cc.ParsedModelFile.Name.Name,
			mc.TypeName,
		))
		f.Comment(fmt.Sprintf(
			"@Success 201 {object} %s.Response \"Created\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 400 {object} %s.Response \"Bad Request\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s [post]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.AddHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Var().Id(strcase.ToLowerCamel(mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			Line(),
			Comment("check for empty payload, unsupported fields, GORM Model fields, optional associations, etc."),
			If(Id("id").Op(",").Id("err").Op(":=").Qual(
				"github.com/threeport/threeport/internal/api",
				"PayloadCheck",
			).Call(Id("c").Op(",").Lit(false).Op(",").Id("objectType")).Op(";").Id("err").Op("!=").Nil()).Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatusErr",
				).Call(Id("id").Op(",").Id("c").Op(",").Nil(), Qual(
					"errors",
					"New",
				).Call(Id("err").Dot("Error").Call()).Op(",").Id("objectType"),
				)),
			),
			Line(),
			If(Id("err").Op(":=").Id("c").Dot("Bind").Call(
				Op("&").Id(strcase.ToLowerCamel(mc.TypeName))).Op(";").Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType")),
				),
			)),
			Line(),
			Comment("check for missing required fields"),
			If(Id("id").Op(",").Id("err").Op(":=").Qual(
				"github.com/threeport/threeport/internal/api",
				"ValidateBoundData",
			).Call(Id("c").Op(",").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Id("objectType")).Op(";").
				Id("err").Op("!=").Nil(),
			).Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatusErr",
				).Call(Id("id").Op(",").Id("c").Op(",").Nil().Op(",").Qual(
					"errors",
					"New",
				).Call(Id("err").Dot("Error").Call()).Op(",").Id("objectType"))),
			),
			Line(),
			If(Id("result").Op(":=").Id("h").Dot("DB").Dot("Create").Call(
				Op("&").Id(strcase.ToLowerCamel(mc.TypeName)),
			).Op(";").Id("result").Dot("Error").Op("!=").Nil()).Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
				),
			),
			Line(),
			Comment("notify controller"),
			Id("notifPayload").Op(",").Id("err").Op(":=").Id(strcase.ToLowerCamel(mc.TypeName)).Dot("NotificationPayload").Call(
				Line().Qual(
					"github.com/threeport/threeport/pkg/notifications",
					"NotificationOperationCreated",
				),
				Line().Lit(false),
				Line().Lit(0),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType")))),
			),
			Id("h").Dot("JS").Dot("Publish").Call(Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.CreateSubject,
			).Op(",").Op("*").Id("notifPayload")),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateResponse",
			).Call(Nil().Op(",").Id(strcase.ToLowerCamel(mc.TypeName))),
			If(Id("err").Op("!=").Nil()).Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
			),
			Line(),
			Return(Qual(
				"github.com/threeport/threeport/internal/api",
				"ResponseStatus201",
			).Call(Id("c").Op(",").Op("*").Id("response"))),
		)
		// get all objects handler
		f.Comment(fmt.Sprintf(
			"@Summary gets all %s.",
			pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false),
		))
		f.Comment(fmt.Sprintf(
			"@Description Get all %s from the Threeport database.",
			pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false),
		))
		f.Comment(fmt.Sprintf(
			"@ID get-%s",
			pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment(fmt.Sprintf(
			"@Param %s query string false \"%s search by name\"",
			"name", // TODO: get fields from model for query params
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Success 200 {object} %s.Response \"OK\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 400 {object} %s.Response \"Bad Request\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s [get]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.GetAllHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Id("params").Op(",").Id("err").Op(":=").Id("c").Assert(Op("*").Qual(
				"github.com/threeport/threeport/internal/api",
				"CustomContext",
			)).Dot("GetPaginationParams").Call(),
			If(Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus400",
				).Call(Id("c").Op(",").Op("&").Id("params").Op(",").Id("err").Op(",").Id("objectType"))),
			)),
			Line(),
			Var().Id("filter").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(Id("err").Op(":=").Id("c").Dot("Bind").Call(Op("&").Id("filter")).Op(";").Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Op("&").Id("params").Op(",").Id("err").Op(",").Id("objectType"))),
			)),
			Line(),
			Var().Id("totalCount").Int64(),
			If(Id("result").Op(":=").Id("h").Dot("DB").Dot("Model").Call(
				Op("&").Qual(
					fmt.Sprintf(
						"github.com/threeport/threeport/pkg/api/%s",
						cc.ParsedModelFile.Name.Name,
					),
					mc.TypeName,
				).Values(),
			).Dot("Where").Call(Op("&").Id("filter")).Dot("Count").Call(Op("&").Id("totalCount")),
				Id("result").Dot("Error").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Op("&").Id("params").Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Id("records").Op(":=").Op("&").Index().Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			).Values(),
			If(Id("result").Op(":=").Id("h").Dot("DB").Dot("Order").Call(
				Lit("ID asc")).Dot("Where").Call(Op("&").Id("filter")).
				Dot("Limit").Call(Id("params").Dot("Size")).
				Dot("Offset").Call(Call(
				Id("params").Dot("Page").Op("-").Lit(1)).Op("*").Id("params").Dot("Size")).
				// TODO: figure out DB preloads
				Dot("Find").Call(Id("records")).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Op("&").Id("params").Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
				)),
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateResponse",
			).Call(Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateMeta",
			).Call(Id("params").Op(",").Id("totalCount")).Op(",").Op("*").Id("records")),
			If(Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Op("&").Id("params").Op(",").Id("err").Op(",").Id("objectType")),
				)),
			),
			Line(),
			Return(Qual(
				"github.com/threeport/threeport/internal/api",
				"ResponseStatus200",
			).Call(Id("c").Op(",").Op("*").Id("response"))),
		)
		// get object handler
		f.Comment(fmt.Sprintf(
			"@Summary gets a %s.", strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Get a particular %s from the database.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@ID get-%s", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment("@Param id path int true \"ID\"")
		f.Comment(fmt.Sprintf(
			"@Success 200 {object} %s.Response \"OK\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 404 {object} %s.Response \"Not Found\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s/{id} [get]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.GetOneHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(fmt.Sprintf(
				"github.com/threeport/threeport/pkg/api/%s",
				cc.ParsedModelFile.Name.Name,
			),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Id(fmt.Sprintf(
				"%sID", strcase.ToLowerCamel(mc.TypeName),
			)).Op(":=").Id("c").Dot("Param").Call(Lit("id")),
			Var().Id(strcase.ToLowerCamel(mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(
				// TODO: figure out preload objects
				Id("result").Op(":=").Id("h").Dot("DB").
					Dot("First").Call(Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
				Line(),
				Line(),
				Id("response").Op(",").Id("err").Op(":=").Qual(fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				), "CreateResponse").Call(Nil().Op(",").Id(strcase.ToLowerCamel(mc.TypeName))),
				If(Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				)),
				Line(),
				Line(),
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus200",
				).Call(Id("c").Op(",").Op("*").Id("response"))),
			),
		)
		// update object handler
		f.Comment(fmt.Sprintf(
			"@Summary updates specific fields for an existing %s.", strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Update a %s in the database.  Provide one or more fields to update.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Note: This API endpint is for updating %s objects only.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment("@Description Request bodies that include related objects will be accepted, however")
		f.Comment("@Description the related objects will not be changed.  Call the patch or put method for")
		f.Comment("@Description each particular existing object to change them.")
		f.Comment(fmt.Sprintf(
			"@ID update-%s", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment("@Param id path int true \"ID\"")
		f.Comment(fmt.Sprintf(
			"@Param %[1]s body %[2]s.%[3]s true \"%[3]s object\"",
			strcase.ToLowerCamel(mc.TypeName),
			cc.ParsedModelFile.Name.Name,
			mc.TypeName,
		))
		f.Comment(fmt.Sprintf(
			"@Success 200 {object} %s.Response \"OK\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 400 {object} %s.Response \"Bad Request\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 404 {object} %s.Response \"Not Found\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s/{id} [patch]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.PatchHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(fmt.Sprintf(
				"github.com/threeport/threeport/pkg/api/%s",
				cc.ParsedModelFile.Name.Name,
			),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Id(fmt.Sprintf(
				"%sID", strcase.ToLowerCamel(mc.TypeName),
			)).Op(":=").Id("c").Dot("Param").Call(Lit("id")),
			Var().Id(fmt.Sprintf("existing%s", mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(
				// TODO: figure out preload objects
				Id("result").Op(":=").Id("h").Dot("DB").
					Dot("First").Call(Op("&").Id(fmt.Sprintf("existing%s", mc.TypeName)).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
			),
			Line(),
			Comment("check for empty payload, invalid or unsupported fields, optional associations, etc."),
			If(
				Id("id").Op(",").Id("err").Op(":=").Qual(
					"github.com/threeport/threeport/internal/api",
					"PayloadCheck",
				).Call(Id("c").Op(",").Lit(true).Op(",").Id("objectType")).Op(";").Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatusErr",
					).Call(Id("id").Op(",").Id("c").Op(",").Nil().Op(",").Qual(
						"errors",
						"New",
					).Call(Id("err").Dot("Error").Call()).Op(",").Id("objectType"))),
				),
			),
			Line(),
			Comment("bind payload"),
			Var().Id(fmt.Sprintf("updated%s", mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(
				Id("err").Op(":=").Id("c").Dot("Bind").Call(
					Op("&").Id(fmt.Sprintf("updated%s", mc.TypeName)),
				).Op(";").Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				),
			),
			Line(),
			If(
				Id("result").Op(":=").Id("h").Dot("DB").Dot("Model").Call(
					Op("&").Id(fmt.Sprintf("existing%s", mc.TypeName)),
				).Dot("Updates").Call(
					Id(fmt.Sprintf("updated%s", mc.TypeName)),
				).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateResponse",
			).Call(Nil().Op(",").Id(fmt.Sprintf("existing%s", mc.TypeName))),
			If(
				Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Return(Qual(
				"github.com/threeport/threeport/internal/api",
				"ResponseStatus200",
			).Call(Id("c").Op(",").Op("*").Id("response"))),
		)
		// replace object handler
		f.Comment(fmt.Sprintf(
			"@Summary updates an existing %s by replacing the entire object.", strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Replace a %s in the database.  All required fields must be provided.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment("@Description If any optional fields are not provided, they will be null post-update.")
		f.Comment(fmt.Sprintf(
			"@Description Note: This API endpint is for updating %s objects only.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment("@Description Request bodies that include related objects will be accepted, however")
		f.Comment("@Description the related objects will not be changed.  Call the patch or put method for")
		f.Comment("@Description each particular existing object to change them.")
		f.Comment(fmt.Sprintf(
			"@ID replace-%s", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment("@Param id path int true \"ID\"")
		f.Comment(fmt.Sprintf(
			"@Param %[1]s body %[2]s.%[3]s true \"%[3]s object\"",
			strcase.ToLowerCamel(mc.TypeName),
			cc.ParsedModelFile.Name.Name,
			mc.TypeName,
		))
		f.Comment(fmt.Sprintf(
			"@Success 200 {object} %s.Response \"OK\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 400 {object} %s.Response \"Bad Request\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 404 {object} %s.Response \"Not Found\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s/{id} [put]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.PutHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(fmt.Sprintf(
				"github.com/threeport/threeport/pkg/api/%s",
				cc.ParsedModelFile.Name.Name,
			),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Id(fmt.Sprintf(
				"%sID", strcase.ToLowerCamel(mc.TypeName),
			)).Op(":=").Id("c").Dot("Param").Call(Lit("id")),
			Var().Id(fmt.Sprintf("existing%s", mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(
				// TODO: figure out preload objects
				Id("result").Op(":=").Id("h").Dot("DB").
					Dot("First").Call(Op("&").Id(fmt.Sprintf("existing%s", mc.TypeName)).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
			),
			Line(),
			Comment("check for empty payload, invalid or unsupported fields, optional associations, etc."),
			If(
				Id("id").Op(",").Id("err").Op(":=").Qual(
					"github.com/threeport/threeport/internal/api",
					"PayloadCheck",
				).Call(Id("c").Op(",").Lit(true).Op(",").Id("objectType")).Op(";").Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatusErr",
					).Call(Id("id").Op(",").Id("c").Op(",").Nil().Op(",").Qual(
						"errors",
						"New",
					).Call(Id("err").Dot("Error").Call()).Op(",").Id("objectType"))),
				),
			),
			Line(),
			Comment("bind payload"),
			Var().Id(fmt.Sprintf("updated%s", mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			If(
				Id("err").Op(":=").Id("c").Dot("Bind").Call(
					Op("&").Id(fmt.Sprintf("updated%s", mc.TypeName)),
				).Op(";").Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Comment("check for missing required fields"),
			If(
				Id("id").Op(",").Id("err").Op(":=").Qual(
					"github.com/threeport/threeport/internal/api",
					"ValidateBoundData",
				).Call(Id("c").Op(",").Id(fmt.Sprintf("updated%s", mc.TypeName)).Op(",").Id("objectType")).
					Op(";").Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatusErr",
					).Call(Id("id").Op(",").Id("c").Op(",").Nil().Op(",").Qual(
						"errors",
						"New",
					).Call(Id("err").Dot("Error").Call()).Op(",").Id("objectType"))),
				),
			),
			Line(),
			Comment("persist provided data"),
			Id(fmt.Sprintf("updated%s", mc.TypeName)).Dot("ID").Op("=").Id(fmt.Sprintf("existing%s", mc.TypeName)).Dot("ID"),
			If(
				Id("result").Op(":=").Id("h").Dot("DB").Dot("Session").Call(
					Op("&").Qual(
						"gorm.io/gorm",
						"Session",
					).Values(Dict{
						Id("FullSaveAssociations"): Lit(false),
					})).Dot("Omit").Call(
					Lit("CreatedAt").Op(",").Lit("DeletedAt"),
				).Dot("Save").Call(
					Op("&").Id(fmt.Sprintf("updated%s", mc.TypeName)),
				).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
					),
				),
			),
			Line(),
			Comment("reload updated data from DB"),
			If(
				// TODO: figure out preload objects
				Id("result").Op(":=").Id("h").Dot("DB").
					Dot("First").Call(Op("&").Id(fmt.Sprintf("existing%s", mc.TypeName)).Op(",").Id(fmt.Sprintf(
					"%sID", strcase.ToLowerCamel(mc.TypeName),
				))).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					If(
						Id("errors").Dot("Is").Call(Id("result").Dot("Error").Op(",").Qual(
							"gorm.io/gorm",
							"ErrRecordNotFound",
						)).Block(
							Return(Qual(
								"github.com/threeport/threeport/internal/api",
								"ResponseStatus404",
							).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType")),
							)),
						Return(Qual(
							"github.com/threeport/threeport/internal/api",
							"ResponseStatus500",
						).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
					),
				),
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateResponse",
			).Call(Nil().Op(",").Id(fmt.Sprintf("existing%s", mc.TypeName))),
			If(
				Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Return(Qual(
				"github.com/threeport/threeport/internal/api",
				"ResponseStatus200",
			).Call(Id("c").Op(",").Op("*").Id("response"))),
		)
		// delete object handler
		f.Comment(fmt.Sprintf(
			"@Summary deletes a %s.", strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@Description Delete a %s by from the database.",
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Comment(fmt.Sprintf(
			"@ID delete-%s", strcase.ToLowerCamel(mc.TypeName),
		))
		f.Comment("@Accept json")
		f.Comment("@Produce json")
		f.Comment("@Param id path int true \"ID\"")
		f.Comment(fmt.Sprintf(
			"@Success 200 {object} %s.Response \"OK\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 404 {object} %s.Response \"Not Found\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 409 {object} %s.Response \"Conflict\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Failure 500 {object} %s.Response \"Internal Server Error\"",
			cc.ParsedModelFile.Name.Name,
		))
		f.Comment(fmt.Sprintf(
			"@Router /%s/%s/{id} [delete]",
			cc.ParsedModelFile.Name.Name,
			pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
		))
		f.Func().Params(
			Id("h").Id("Handler"),
		).Id(mc.DeleteHandlerName).Params(
			Id("c").Qual(
				"github.com/labstack/echo/v4",
				"Context",
			),
		).Parens(List(
			Error(),
		)).Block(
			Id("objectType").Op(":=").Qual(fmt.Sprintf(
				"github.com/threeport/threeport/pkg/api/%s",
				cc.ParsedModelFile.Name.Name,
			),
				fmt.Sprintf(
					"ObjectType%s",
					mc.TypeName,
				),
			),
			Id(fmt.Sprintf(
				"%sID", strcase.ToLowerCamel(mc.TypeName),
			)).Op(":=").Id("c").Dot("Param").Call(Lit("id")),
			Var().Id(strcase.ToLowerCamel(mc.TypeName)).Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.TypeName,
			),
			// TODO: figure out all preload objects
			deleteObjectHandler,
			If(
				Id("result").Op(":=").Id("h").Dot("DB").Dot("Delete").Call(
					Op("&").Id(strcase.ToLowerCamel(mc.TypeName)),
				).Op(";").Id("result").Dot("Error").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("result").Dot("Error").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Comment("notify controller"),
			Id("notifPayload").Op(",").Id("err").Op(":=").Id(strcase.ToLowerCamel(mc.TypeName)).Dot("NotificationPayload").Call(
				Line().Qual(
					"github.com/threeport/threeport/pkg/notifications",
					"NotificationOperationDeleted",
				),
				Line().Lit(false),
				Line().Lit(0),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return(Qual(
					"github.com/threeport/threeport/internal/api",
					"ResponseStatus500",
				).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType")))),
			),
			Id("h").Dot("JS").Dot("Publish").Call(Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				mc.DeleteSubject,
			).Op(",").Op("*").Id("notifPayload")),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Qual(
				fmt.Sprintf(
					"github.com/threeport/threeport/pkg/api/%s",
					cc.ParsedModelFile.Name.Name,
				),
				"CreateResponse",
			).Call(Nil().Op(",").Id(strcase.ToLowerCamel(mc.TypeName))),
			If(
				Id("err").Op("!=").Nil().Block(
					Return(Qual(
						"github.com/threeport/threeport/internal/api",
						"ResponseStatus500",
					).Call(Id("c").Op(",").Nil().Op(",").Id("err").Op(",").Id("objectType"))),
				),
			),
			Line(),
			Return(Qual(
				"github.com/threeport/threeport/internal/api",
				"ResponseStatus200",
			).Call(Id("c").Op(",").Op("*").Id("response"))),
		)
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", codegen.FilenameSansExt(cc.ModelFilename))
	genFilepath := filepath.Join(apiHandlersPath(), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed open file to write generated code for model handlers: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for model handlers: %w", err)
	}
	fmt.Printf("code generation complete for %s model handlers\n", cc.ControllerDomainLower)

	return nil
}
