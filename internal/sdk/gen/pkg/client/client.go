package client

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

const (
	MarshalObjectErr       = "failed to marshal provided object to JSON: %w"
	ResponseErr            = "call to threeport API returned unexpected response: %w"
	MarshalResponseDataErr = "failed to marshal response data from threeport API: %w"
)

// GenClientLib generates an the API objects' client library.
func GenClientLib(gen *gen.Generator) error {
	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			pluralize := pluralize.NewClient()
			f := NewFile(objCollection.Version)
			f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

			f.ImportAlias(util.SetImportAlias(
				"github.com/threeport/threeport/pkg/util/v0",
				"util",
				"tputil",
				gen.Module,
			))
			f.ImportAlias(util.SetImportAlias(
				"github.com/threeport/threeport/pkg/client/lib/v0",
				"client_lib",
				"tpclient_lib",
				gen.Module,
			))

			for _, apiObject := range objGroup.ApiObjects {
				// get all objects
				getAllFuncName := fmt.Sprintf("Get%s", pluralize.Pluralize(apiObject.TypeName, 2, false))
				f.Comment(fmt.Sprintf(
					"%s fetches all %s.",
					getAllFuncName,
					pluralize.Pluralize(strcase.ToDelimited(apiObject.TypeName, ' '), 2, false),
				))
				f.Comment("TODO: implement pagination")
				f.Func().Id(getAllFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
				).Parens(List(
					Op("*").Index().Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Var().Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Index().Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
						),
						Line().Qual("net/http", "MethodGet"),
						Line().New(Qual("bytes", "Buffer")),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data"),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Nil(),
				)
				f.Line()
				// get object by ID
				getByIDFuncName := fmt.Sprintf("Get%sByID", apiObject.TypeName)
				f.Comment(fmt.Sprintf(
					"%s fetches a %s by ID.",
					getByIDFuncName,
					strcase.ToDelimited(apiObject.TypeName, ' '),
				))
				f.Func().Id(getByIDFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
					Id("id").Uint(),
				).Parens(List(
					Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Var().Id(strcase.ToLowerCamel(apiObject.TypeName)).Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s/%d"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
							Id("id"),
						),
						Line().Qual("net/http", "MethodGet"),
						Line().New(Qual("bytes", "Buffer")),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data").Index(Lit(0)),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Nil(),
				)
				f.Line()
				// get object by query string
				getByQueryStringFuncName := fmt.Sprintf("Get%sByQueryString", pluralize.Pluralize(apiObject.TypeName, 2, false))
				f.Comment(fmt.Sprintf(
					"%s fetches %s by provided query string.",
					getByQueryStringFuncName,
					pluralize.Pluralize(strcase.ToDelimited(apiObject.TypeName, ' '), 2, false),
				))
				f.Func().Id(getByQueryStringFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
					Id("queryString").String(),
				).Parens(List(
					Op("*").Index().Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Var().Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Index().Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s?%s"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
							Id("queryString"),
						),
						Line().Qual("net/http", "MethodGet"),
						Line().New(Qual("bytes", "Buffer")),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data"),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).Op(",").Nil(),
				)
				f.Line()
				// get object by name
				getByNameFuncName := fmt.Sprintf("Get%sByName", apiObject.TypeName)
				f.Comment(fmt.Sprintf(
					"%s fetches a %s by name.",
					getByNameFuncName,
					strcase.ToDelimited(apiObject.TypeName, ' '),
				))
				f.Func().Id(getByNameFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").Op(",").Id("name").String(),
				).Parens(List(
					Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Var().Id(
						pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false),
					).Index().Id(objCollection.Version).Dot(apiObject.TypeName),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s?name=%s"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
							Id("name"),
						),
						Line().Qual("net/http", "MethodGet"),
						Line().New(Qual("bytes", "Buffer")),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Qual(
							fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
							apiObject.TypeName,
						).Values().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data"),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Qual(
							fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
							apiObject.TypeName,
						).Values().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Switch().Block(
						Case(Len(Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false))).Op("<").Lit(1)).Block(
							Return().Op("&").Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								apiObject.TypeName,
							).Values().Op(",").Qual("errors", "New").Call(
								Qual("fmt", "Sprintf").Call(
									Lit("no "+strcase.ToDelimited(apiObject.TypeName, ' ')+" with name %s").Op(",").Id("name"),
								),
							),
						),
						Case(Len(Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false))).Op(">").Lit(1)).Block(
							Return().Op("&").Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								apiObject.TypeName,
							).Values().Op(",").Qual("errors", "New").Call(
								Qual("fmt", "Sprintf").Call(
									Lit("more than one "+strcase.ToDelimited(apiObject.TypeName, ' ')+" with name %s returned").Op(",").Id("name"),
								),
							),
						),
					),
					Line(),
					Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(apiObject.TypeName), 2, false)).
						Index(Lit(0)).Op(",").Nil(),
				)
				f.Line()
				// create object
				createFuncName := fmt.Sprintf("Create%s", apiObject.TypeName)
				f.Comment(fmt.Sprintf(
					"%s creates a new %s.",
					createFuncName,
					strcase.ToDelimited(apiObject.TypeName, ' '),
				))
				f.Func().Id(createFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
					Id(strcase.ToLowerCamel(apiObject.TypeName)).Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
				).Parens(List(
					Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"ReplaceAssociatedObjectsWithNil",
					).Call(Id(strcase.ToLowerCamel(apiObject.TypeName))),
					Id(fmt.Sprintf("json%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"MarshalObject",
					).Call(Id(strcase.ToLowerCamel(apiObject.TypeName))),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalObjectErr).Op(",").Id("err")),
					)),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
						),
						Line().Qual("net/http", "MethodPost"),
						Line().Qual("bytes", "NewBuffer").Call(Id(
							fmt.Sprintf("json%s", apiObject.TypeName),
						)),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusCreated"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data").Index(Lit(0)),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Nil(),
				)
				f.Line()
				// update object
				updateFuncName := fmt.Sprintf("Update%s", apiObject.TypeName)
				f.Comment(fmt.Sprintf(
					"%s updates a %s.",
					updateFuncName,
					strcase.ToDelimited(apiObject.TypeName, ' '),
				))
				f.Func().Id(updateFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
					Id(strcase.ToLowerCamel(apiObject.TypeName)).Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
				).Parens(List(
					Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"ReplaceAssociatedObjectsWithNil",
					).Call(Id(strcase.ToLowerCamel(apiObject.TypeName))),
					Comment("capture the object ID, make a copy of the object, then remove fields that"),
					Comment("cannot be updated in the API"),
					Id(
						fmt.Sprintf("%sID", strcase.ToLowerCamel(apiObject.TypeName)),
					).Op(":=").Op("*").Id(strcase.ToLowerCamel(apiObject.TypeName)).Dot("ID"),
					Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Op(":=").Op("*").Id(strcase.ToLowerCamel(apiObject.TypeName)),
					Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Dot("ID").Op("=").Nil(),
					Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Dot("CreatedAt").Op("=").Nil(),
					Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Dot("UpdatedAt").Op("=").Nil(),
					Line(),
					Id(fmt.Sprintf("json%s", apiObject.TypeName)).Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"MarshalObject",
					).Call(Id(fmt.Sprintf("payload%s", apiObject.TypeName))),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalObjectErr).Op(",").Id("err")),
					)),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s/%d"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
							Id(fmt.Sprintf("%sID", strcase.ToLowerCamel(apiObject.TypeName))),
						),
						Line().Qual("net/http", "MethodPatch"),
						Line().Qual("bytes", "NewBuffer").Call(Id(
							fmt.Sprintf("json%s", apiObject.TypeName),
						)),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data").Index(Lit(0)),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(fmt.Sprintf("payload%s", apiObject.TypeName)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Dot("ID").Op("=").Op("&").Id(fmt.Sprintf("%sID", strcase.ToLowerCamel(apiObject.TypeName))),
					Return().Op("&").Id(fmt.Sprintf("payload%s", apiObject.TypeName)).Op(",").Nil(),
				)
				f.Line()
				// delete object
				deleteFuncName := fmt.Sprintf("Delete%s", apiObject.TypeName)
				f.Comment(fmt.Sprintf(
					"%s deletes a %s by ID.",
					deleteFuncName,
					strcase.ToDelimited(apiObject.TypeName, ' '),
				))
				f.Func().Id(deleteFuncName).Params(
					Id("apiClient").Op("*").Qual("net/http", "Client"),
					Id("apiAddr").String(),
					Id("id").Uint(),
				).Parens(List(
					Op("*").Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Error(),
				)).Block(
					Var().Id(strcase.ToLowerCamel(apiObject.TypeName)).Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					),
					Line(),
					Id("response").Op(",").Id("err").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/client/lib/v0",
						"GetResponse",
					).Call(
						Line().Id("apiClient"),
						Line().Qual("fmt", "Sprintf").Call(
							Lit("%s%s/%d"),
							Id("apiAddr"),
							Qual(
								fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
								fmt.Sprintf("Path%s", pluralize.Pluralize(apiObject.TypeName, 2, false)),
							),
							Id("id"),
						),
						Line().Qual("net/http", "MethodDelete"),
						Line().New(Qual("bytes", "Buffer")),
						Line().Map(String()).String().Block(),
						Line().Qual("net/http", "StatusOK"),
						Line(),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(ResponseErr).Op(",").Id("err")),
					)),
					Line(),
					Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
						Id("response").Dot("Data").Index(Lit(0)),
					),
					If(Id("err").Op("!=").Nil().Block(
						Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit(MarshalResponseDataErr).Op(",").Id("err")),
					)),
					Line(),
					Id("decoder").Op(":=").Qual(
						"encoding/json", "NewDecoder",
					).Call(Qual(
						"bytes", "NewReader",
					).Call(Id("jsonData"))),
					Id("decoder").Dot("UseNumber").Call(),
					If(Id("err").Op(":=").Id("decoder").Dot("Decode").Call(
						Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)),
					).Op(";").Id("err").Op("!=").Nil()).Block(
						Return().Nil().Op(",").Qual(
							"fmt", "Errorf",
						).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
					),
					Line(),
					Return().Op("&").Id(strcase.ToLowerCamel(apiObject.TypeName)).Op(",").Nil(),
				)
				f.Line()
				// TODO: replace object
			}

			// write code to file
			genFilepath := filepath.Join(
				"pkg",
				"client",
				objCollection.Version,
				fmt.Sprintf("%s_gen.go", strcase.ToSnake(objGroup.Name)),
			)
			_, err := util.WriteCodeToFile(f, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf("source code for API client library written to %s", genFilepath))
		}
	}

	return nil
}
