package versions

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
)

// TaggedFields generates the tagged field map vars.
func (gvc *GlobalVersionConfig) TaggedFields() error {
	f := NewFile("v0")
	f.HeaderComment("generated by 'threeport-codegen api-version' - do not edit")

	for _, version := range gvc.Versions {
		for _, name := range version.DatabaseInitNames {
			f.Var().Id(fmt.Sprintf("%sTaggedFields", name)).Op("=").Id("make").Call(
				Map(String()).Op("*").Id("FieldsByTag"),
			)
		}
	}

	// write code to file
	taggedFieldsFilepath := filepath.Join("..", "..", "pkg", "api-server", "v0", "tagged_fields_gen.go")
	file, err := os.OpenFile(taggedFieldsFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for tagged fields maps: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for tagged fields maps: %w", err)
	}
	fmt.Println("code generation complete for tagged fields maps")

	return nil
}

// ExtensionTaggedFields generates the tagged field map vars for an extension
func (gvc *GlobalVersionConfig) ExtensionTaggedFields() error {
	f := NewFile("v0")
	f.HeaderComment("generated by 'threeport-codegen api-version' - do not edit")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0", "tpiapi")

	for _, version := range gvc.Versions {
		for _, name := range version.DatabaseInitNames {
			f.Var().Id(fmt.Sprintf("%sTaggedFields", name)).Op("=").Id("make").Call(
				Map(String()).Op("*").Qual(
					"github.com/threeport/threeport/pkg/api-server/v0",
					"FieldsByTag",
				),
			)
		}
	}

	// write code to file
	taggedFieldsFilepath := filepath.Join("..", "..", "pkg", "api-server", "v0", "tagged_fields_gen.go")
	file, err := os.OpenFile(taggedFieldsFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for tagged fields maps: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for tagged fields maps: %w", err)
	}
	fmt.Println("code generation complete for tagged fields maps")

	return nil
}