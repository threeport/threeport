package models

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk"
)

// apiVersionsPath returns the path from the models to the API's internal
// versions package.
func apiVersionsPath(apiVersion string) string {
	return filepath.Join("..", "..", "..", "pkg", "api-server", apiVersion, "versions")
}

// ModelVersions adds each API version and validation for the fields of the
// model.
func (cc *ControllerConfig) ModelVersions() error {
	f := NewFile("versions")
	f.HeaderComment("generated by 'threeport-sdk codegen api-model' - do not edit")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0", "iapi")

	for _, mc := range cc.ModelConfigs {
		taggedFieldsMapName := fmt.Sprintf("%sTaggedFields", mc.TypeName)

		f.Comment(fmt.Sprintf(
			"Add%sVersions adds field validation info and adds it",
			mc.TypeName,
		))
		f.Comment("to the REST API versions.")
		f.Func().Id(
			fmt.Sprintf("Add%sVersions", mc.TypeName),
		).Call().Block(
			Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				taggedFieldsMapName,
			).Index(Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			)).Op("=").Op("&").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"FieldsByTag",
			).Values(Dict{
				Id("TagName"): Qual(
					"github.com/threeport/threeport/pkg/api-server/v0",
					"TagNameValidate",
				),
				Id("Required"):             Index().String().Values(),
				Id("Optional"):             Index().String().Values(),
				Id("OptionalAssociations"): Index().String().Values(),
			}),
			Line(),
			Comment("parse struct and populate the FieldsByTag object"),
			Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"ParseStruct",
			).Call(Line().Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			).Op(",").Line().Qual(
				"reflect",
				"ValueOf",
			).Call(Id("new").Call(Qual(
				fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.ParsedModelFile.Name.Name),
				mc.TypeName,
			))).Op(",").Line().Lit("").Op(",").Line().Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"Translate",
			).Op(",").Line().Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				taggedFieldsMapName,
			).Op(",").Line(),
			),
			Line(),
			Comment("create a version object which contains the object name and versions"),
			Id("versionObj").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"VersionObject",
			).Values(Dict{
				Id("Version"): Lit(cc.ApiVersion),
				Id("Object"): Id("string").Call(Qual(
					fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", cc.ApiVersion),
					fmt.Sprintf("ObjectType%s", mc.TypeName),
				)),
			}),
			Line(),
			Comment("add the object tagged fields to the global tagged fields map"),
			Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"ObjectTaggedFields",
			).Index(Id("versionObj")).Op("=").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				taggedFieldsMapName,
			).Index(Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			)),
			Line(),
			Comment("add the object tagged fields to the rest API version"),
			Qual(
				"github.com/threeport/threeport/pkg/api",
				"AddRestApiVersion",
			).Call(Id("versionObj")),
		)
		f.Line()
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", sdk.FilenameSansExt(cc.ModelFilename))
	genFilepath := filepath.Join(apiVersionsPath(cc.ApiVersion), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for API versions: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for API versions: %w", err)
	}
	fmt.Printf("code generation complete for %s API versions\n", cc.ControllerDomainLower)

	return nil
}

// ExtensionModelVersions adds each API version and validation for the fields of the
// model on a threeport extensions .
func (cc *ControllerConfig) ExtensionModelVersions(modulePath string) error {
	f := NewFile("versions")
	f.HeaderComment("generated by 'threeport-sdk codegen api-model' - do not edit")
	f.ImportAlias(fmt.Sprintf("%s/pkg/api-server/v0", modulePath), "iapi")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0", "tpiapi")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpv0")

	for _, mc := range cc.ModelConfigs {
		taggedFieldsMapName := fmt.Sprintf("%sTaggedFields", mc.TypeName)

		f.Comment(fmt.Sprintf(
			"Add%sVersions adds field validation info and adds it",
			mc.TypeName,
		))
		f.Comment("to the REST API versions.")
		f.Func().Id(
			fmt.Sprintf("Add%sVersions", mc.TypeName),
		).Call().Block(
			Qual(
				fmt.Sprintf("%s/pkg/api-server/v0", modulePath),
				taggedFieldsMapName,
			).Index(Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			)).Op("=").Op("&").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"FieldsByTag",
			).Values(Dict{
				Id("TagName"): Qual(
					"github.com/threeport/threeport/pkg/api-server/v0",
					"TagNameValidate",
				),
				Id("Required"):             Index().String().Values(),
				Id("Optional"):             Index().String().Values(),
				Id("OptionalAssociations"): Index().String().Values(),
			}),
			Line(),
			Comment("parse struct and populate the FieldsByTag object"),
			Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"ParseStruct",
			).Call(Line().Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			).Op(",").Line().Qual(
				"reflect",
				"ValueOf",
			).Call(Id("new").Call(Qual(
				fmt.Sprintf("%s/pkg/api/v0", modulePath),
				mc.TypeName,
			))).Op(",").Line().Lit("").Op(",").Line().Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"Translate",
			).Op(",").Line().Qual(
				fmt.Sprintf("%s/pkg/api-server/v0", modulePath),
				taggedFieldsMapName,
			).Op(",").Line(),
			),
			Line(),
			Comment("create a version object which contains the object name and versions"),
			Id("versionObj").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"VersionObject",
			).Values(Dict{
				Id("Version"): Lit(cc.ApiVersion),
				Id("Object"): Id("string").Call(Qual(
					fmt.Sprintf("%s/pkg/api/%s", modulePath, cc.ApiVersion),
					fmt.Sprintf("ObjectType%s", mc.TypeName),
				)),
			}),
			Line(),
			Comment("add the object tagged fields to the global tagged fields map"),
			Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"ObjectTaggedFields",
			).Index(Id("versionObj")).Op("=").Qual(
				fmt.Sprintf("%s/pkg/api-server/v0", modulePath),
				taggedFieldsMapName,
			).Index(Qual(
				"github.com/threeport/threeport/pkg/api-server/v0",
				"TagNameValidate",
			)),
			Line(),
			Comment("add the object tagged fields to the rest API version"),
			Qual(
				"github.com/threeport/threeport/pkg/api",
				"AddRestApiVersion",
			).Call(Id("versionObj")),
		)
		f.Line()
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", sdk.FilenameSansExt(cc.ModelFilename))
	genFilepath := filepath.Join(apiVersionsPath(cc.ApiVersion), genFilename)
	file, err := os.OpenFile(genFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for API versions: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for API versions: %w", err)
	}
	fmt.Printf("code generation complete for %s API versions\n", cc.ControllerDomainLower)

	return nil
}
