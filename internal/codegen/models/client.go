package models

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/codegen"
)

const (
	MarshalObjectErr       = "failed to marshal provided object to JSON: %w"
	ResponseErr            = "call to threeport API returned unexpected response: %w"
	MarshalResponseDataErr = "failed to marshal response data from threeport API: %w"
	UnmarshalObjectErr     = "failed unmarshal object from threeport response data: %w"
)

// clientLibPath returns the path from the models to the API's internal handlers
// package.
func clientLibPath(packageName string) string {
	return filepath.Join("..", "..", "..", "pkg", "client", packageName)
}

// ClientLib generates the client library code for the API models in a model
// file.
func (cc *ControllerConfig) ClientLib() error {
	pluralize := pluralize.NewClient()
	f := NewFile(cc.PackageName)
	f.HeaderComment("generated by 'threeport-codegen api-model' - do not edit")

	for _, mc := range cc.ModelConfigs {
		// get all objects
		getAllFuncName := fmt.Sprintf("Get%s", pluralize.Pluralize(mc.TypeName, 2, false))
		f.Comment(fmt.Sprintf(
			"%s fetches all %s.",
			getAllFuncName,
			pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false),
		))
		f.Comment("TODO: implement pagination")
		f.Func().Id(getAllFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").String(),
		).Parens(List(
			Op("*").Index().Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Var().Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)).Index().Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").Id("ApiVersion").Op(","),
				),
				Line().Qual("net/http", "MethodGet"),
				Line().New(Qual("bytes", "Buffer")),
				Line().Qual("net/http", "StatusOK"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(ResponseErr).Op(",").Id("err")),
			)),
			Line(),
			Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
				Id("response").Dot("Data"),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)).Op(",").Qual(
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
				Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)).Op(",").Nil(),
		)
		f.Line()
		// get object by ID
		getByIDFuncName := fmt.Sprintf("Get%sByID", mc.TypeName)
		f.Comment(fmt.Sprintf(
			"%s fetches a %s by ID.",
			getByIDFuncName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Func().Id(getByIDFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").String(),
			Id("id").Uint(),
		).Parens(List(
			Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Var().Id(strcase.ToLowerCamel(mc.TypeName)).Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s/%%d", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").Id("ApiVersion").Op(",").Id("id"),
				),
				Line().Qual("net/http", "MethodGet"),
				Line().New(Qual("bytes", "Buffer")),
				Line().Qual("net/http", "StatusOK"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(ResponseErr).Op(",").Id("err")),
			)),
			Line(),
			Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
				Id("response").Dot("Data").Index(Lit(0)),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
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
				Op("&").Id(strcase.ToLowerCamel(mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Nil(),
		)
		f.Line()
		// get object by name
		getByNameFuncName := fmt.Sprintf("Get%sByName", mc.TypeName)
		f.Comment(fmt.Sprintf(
			"%s fetches a %s by name.",
			getByNameFuncName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Func().Id(getByNameFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").Op(",").Id("name").String(),
		).Parens(List(
			Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Var().Id(
				pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false),
			).Index().Id(cc.PackageName).Dot(mc.TypeName),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s?name=%%s", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").Id("ApiVersion").Op(",").Id("name"),
				),
				Line().Qual("net/http", "MethodGet"),
				Line().New(Qual("bytes", "Buffer")),
				Line().Qual("net/http", "StatusOK"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Qual(
					fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
					mc.TypeName,
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
					fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
					mc.TypeName,
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
				Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Switch().Block(
				Case(Len(Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false))).Op("<").Lit(1)).Block(
					Return().Op("&").Qual(
						fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
						mc.TypeName,
					).Values().Op(",").Qual("errors", "New").Call(
						Qual("fmt", "Sprintf").Call(
							Lit("no "+strcase.ToDelimited(mc.TypeName, ' ')+" with name %s").Op(",").Id("name"),
						),
					),
				),
				Case(Len(Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false))).Op(">").Lit(1)).Block(
					Return().Op("&").Qual(
						fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
						mc.TypeName,
					).Values().Op(",").Qual("errors", "New").Call(
						Qual("fmt", "Sprintf").Call(
							Lit("more than one "+strcase.ToDelimited(mc.TypeName, ' ')+" with name %s returned").Op(",").Id("name"),
						),
					),
				),
			),
			Line(),
			Return().Op("&").Id(pluralize.Pluralize(strcase.ToLowerCamel(mc.TypeName), 2, false)).
				Index(Lit(0)).Op(",").Nil(),
		)
		f.Line()
		// create object
		createFuncName := fmt.Sprintf("Create%s", mc.TypeName)
		f.Comment(fmt.Sprintf(
			"%s creates a new %s.",
			createFuncName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Func().Id(createFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").String(),
			Id(strcase.ToLowerCamel(mc.TypeName)).Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
		).Parens(List(
			Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Id(fmt.Sprintf("json%s", mc.TypeName)).Op(",").Id("err").Op(":=").Qual(
				"github.com/threeport/threeport/internal/util",
				"MarshalObject",
			).Call(Id(strcase.ToLowerCamel(mc.TypeName))),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(MarshalObjectErr).Op(",").Id("err")),
			)),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").Id("ApiVersion"),
				),
				Line().Qual("net/http", "MethodPost"),
				Line().Qual("bytes", "NewBuffer").Call(Id(
					fmt.Sprintf("json%s", mc.TypeName),
				)),
				Line().Qual("net/http", "StatusCreated"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(ResponseErr).Op(",").Id("err")),
			)),
			Line(),
			Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
				Id("response").Dot("Data").Index(Lit(0)),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
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
				Op("&").Id(strcase.ToLowerCamel(mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Nil(),
		)
		f.Line()
		// update object
		updateFuncName := fmt.Sprintf("Update%s", mc.TypeName)
		f.Comment(fmt.Sprintf(
			"%s updates a %s.",
			updateFuncName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Func().Id(updateFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").String(),
			Id(strcase.ToLowerCamel(mc.TypeName)).Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
		).Parens(List(
			Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Comment("capture the object ID, make a copy of the object, then remove fields that"),
			Comment("cannot be updated in the API"),
			Id(
				fmt.Sprintf("%sID", strcase.ToLowerCamel(mc.TypeName)),
			).Op(":=").Op("*").Id(strcase.ToLowerCamel(mc.TypeName)).Dot("ID"),
			Id(fmt.Sprintf("payload%s", mc.TypeName)).Op(":=").Op("*").Id(strcase.ToLowerCamel(mc.TypeName)),
			Id(fmt.Sprintf("payload%s", mc.TypeName)).Dot("ID").Op("=").Nil(),
			Id(fmt.Sprintf("payload%s", mc.TypeName)).Dot("CreatedAt").Op("=").Nil(),
			Id(fmt.Sprintf("payload%s", mc.TypeName)).Dot("UpdatedAt").Op("=").Nil(),
			Line(),
			Id(fmt.Sprintf("json%s", mc.TypeName)).Op(",").Id("err").Op(":=").Qual(
				"github.com/threeport/threeport/internal/util",
				"MarshalObject",
			).Call(Id(fmt.Sprintf("payload%s", mc.TypeName))),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(MarshalObjectErr).Op(",").Id("err")),
			)),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s/%%d", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").
						Id("ApiVersion").Op(",").
						Id(fmt.Sprintf("%sID", strcase.ToLowerCamel(mc.TypeName))),
				),
				Line().Qual("net/http", "MethodPatch"),
				Line().Qual("bytes", "NewBuffer").Call(Id(
					fmt.Sprintf("json%s", mc.TypeName),
				)),
				Line().Qual("net/http", "StatusOK"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(ResponseErr).Op(",").Id("err")),
			)),
			Line(),
			Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
				Id("response").Dot("Data").Index(Lit(0)),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
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
				Op("&").Id(fmt.Sprintf("payload%s", mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Id(fmt.Sprintf("payload%s", mc.TypeName)).Dot("ID").Op("=").Op("&").Id(fmt.Sprintf("%sID", strcase.ToLowerCamel(mc.TypeName))),
			Return().Op("&").Id(fmt.Sprintf("payload%s", mc.TypeName)).Op(",").Nil(),
		)
		f.Line()
		// delete object
		deleteFuncName := fmt.Sprintf("Delete%s", mc.TypeName)
		f.Comment(fmt.Sprintf(
			"%s deletes a %s by ID.",
			deleteFuncName,
			strcase.ToDelimited(mc.TypeName, ' '),
		))
		f.Func().Id(deleteFuncName).Params(
			Id("apiClient").Op("*").Qual("net/http", "Client"),
			Id("apiAddr").String(),
			Id("id").Uint(),
		).Parens(List(
			Op("*").Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Error(),
		)).Block(
			Var().Id(strcase.ToLowerCamel(mc.TypeName)).Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.PackageName),
				mc.TypeName,
			),
			Line(),
			Id("response").Op(",").Id("err").Op(":=").Id("GetResponse").Call(
				Line().Id("apiClient"),
				Line().Qual("fmt", "Sprintf").Call(
					Lit(fmt.Sprintf(
						"%%s/%%s/%s/%%d", pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false),
					)).Op(",").
						Id("apiAddr").Op(",").Id("ApiVersion").Op(",").Id("id"),
				),
				Line().Qual("net/http", "MethodDelete"),
				Line().New(Qual("bytes", "Buffer")),
				Line().Qual("net/http", "StatusOK"),
				Line(),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit(ResponseErr).Op(",").Id("err")),
			)),
			Line(),
			Id("jsonData").Op(",").Id("err").Op(":=").Qual("encoding/json", "Marshal").Call(
				Id("response").Dot("Data").Index(Lit(0)),
			),
			If(Id("err").Op("!=").Nil().Block(
				Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Qual(
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
				Op("&").Id(strcase.ToLowerCamel(mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return().Nil().Op(",").Qual(
					"fmt", "Errorf",
				).Call(Lit("failed to decode object in response data from threeport API: %w").Op(",").Id("err")),
			),
			Line(),
			Return().Op("&").Id(strcase.ToLowerCamel(mc.TypeName)).Op(",").Nil(),
		)
		f.Line()
		// TODO: replace object
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", codegen.FilenameSansExt(cc.ModelFilename))
	genFilepath := filepath.Join(clientLibPath(cc.PackageName), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed open file to write generated code for model client library: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for model client library: %w", err)
	}
	fmt.Printf("code generation complete for %s client library\n", cc.ControllerDomainLower)

	return nil
}
