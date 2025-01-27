package apiserver

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenObjValidationVersions adds object validation and versions to the API
// server.
func GenObjValidationVersions(gen *gen.Generator) error {
	for _, objCollection := range gen.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			f := NewFile("versions")
			f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

			f.ImportAlias(
				fmt.Sprintf("%s/pkg/api-server/%s", gen.ModulePath, objCollection.Version),
				fmt.Sprintf("apiserver_%s", objCollection.Version),
			)
			f.ImportAlias(
				fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
				fmt.Sprintf("api_%s", objCollection.Version),
			)
			f.ImportAlias(util.SetImportAlias(
				"github.com/threeport/threeport/pkg/api-server/lib/v0",
				"apiserver_lib",
				"tpapiserver_lib",
				gen.Module,
			))
			f.ImportAlias(util.SetImportAlias(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", objCollection.Version),
				fmt.Sprintf("api_%s", objCollection.Version),
				fmt.Sprintf("tpapi_%s", objCollection.Version),
				gen.Module,
			))
			f.ImportAlias(util.SetImportAlias(
				"github.com/threeport/threeport/pkg/api",
				"api",
				"tpapi",
				gen.Module,
			))

			for _, apiObject := range objGroup.ApiObjects {
				taggedFieldsMapName := fmt.Sprintf("%sTaggedFields", apiObject.TypeName)

				f.Comment(fmt.Sprintf(
					"Add%sVersions adds field validation info and adds it",
					apiObject.TypeName,
				))
				f.Comment("to the REST API versions.")
				f.Func().Id(
					fmt.Sprintf("Add%sVersions", apiObject.TypeName),
				).Call().Block(
					Qual(
						fmt.Sprintf("%s/pkg/api-server/%s", gen.ModulePath, objCollection.Version),
						taggedFieldsMapName,
					).Index(Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"TagNameValidate",
					)).Op("=").Op("&").Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"FieldsByTag",
					).Values(Dict{
						Id("TagName"): Qual(
							"github.com/threeport/threeport/pkg/api-server/lib/v0",
							"TagNameValidate",
						),
						Id("Required"):             Index().String().Values(),
						Id("Optional"):             Index().String().Values(),
						Id("OptionalAssociations"): Index().String().Values(),
					}),
					Line(),
					Comment("parse struct and populate the FieldsByTag object"),
					Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"ParseStruct",
					).Call(Line().Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"TagNameValidate",
					).Op(",").Line().Qual(
						"reflect",
						"ValueOf",
					).Call(Id("new").Call(Qual(
						fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
						apiObject.TypeName,
					))).Op(",").Line().Lit("").Op(",").Line().Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"Translate",
					).Op(",").Line().Qual(
						fmt.Sprintf("%s/pkg/api-server/%s", gen.ModulePath, objCollection.Version),
						taggedFieldsMapName,
					).Op(",").Line(),
					),
					Line(),
					Comment("create a version object which contains the object name and versions"),
					Id("versionObj").Op(":=").Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"VersionObject",
					).Values(Dict{
						Id("Version"): Lit(objCollection.Version),
						Id("Object"): Id("string").Call(Qual(
							fmt.Sprintf("%s/pkg/api/%s", gen.ModulePath, objCollection.Version),
							fmt.Sprintf("ObjectType%s", apiObject.TypeName),
						)),
					}),
					Line(),
					Comment("add the object tagged fields to the global tagged fields map"),
					Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"ObjectTaggedFields",
					).Index(Id("versionObj")).Op("=").Qual(
						fmt.Sprintf("%s/pkg/api-server/%s", gen.ModulePath, objCollection.Version),
						taggedFieldsMapName,
					).Index(Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"TagNameValidate",
					)),
					Line(),
					Comment("add the object tagged fields to the rest API version"),
					Qual(
						"github.com/threeport/threeport/pkg/api-server/lib/v0",
						"AddObjectVersion",
					).Call(Id("versionObj")),
				)
				f.Line()
			}

			// write code to file
			genFilepath := filepath.Join(
				"pkg",
				"api-server",
				objCollection.Version,
				"versions",
				fmt.Sprintf("%s_gen.go", strcase.ToSnake(objGroup.Name)),
			)
			_, err := util.WriteCodeToFile(f, genFilepath, true)
			if err != nil {
				return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
			}
			cli.Info(fmt.Sprintf("source code for API object validataion and versions written to %s", genFilepath))
		}
	}

	return nil
}
