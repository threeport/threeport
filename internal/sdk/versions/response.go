package versions

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
)

// ResponseObjects generates code for object type conversion used in API
// handlers.
func (gvc *GlobalVersionConfig) ResponseObjects() error {
	pluralize := pluralize.NewClient()
	for _, apiVersion := range gvc.Versions {
		f := NewFile(apiVersion.VersionName)
		f.HeaderComment("generated by 'threeport-sdk gen' for API response boilerplate - do not edit")

		objectTypesByPath := &Statement{}
		for _, name := range apiVersion.RouteNames {
			objectTypesByPath.Case(Id(fmt.Sprintf("Path%s", pluralize.Pluralize(name, 2, false)))).Block(
				Return().Id(fmt.Sprintf("ObjectType%s", name)),
			)
			objectTypesByPath.Line()
		}

		f.Comment("GetObjectTypeByPath returns the object type based on an API path.")
		f.Func().Id("GetObjectTypeByPath").Params(
			Id("path").String(),
		).Id("ObjectType").Block(
			Switch(Id("path")).Block(
				objectTypesByPath,
			),
			Line(),
			Return().Id("ObjectTypeUnknown"),
		)

		// write code to file
		routesFilepath := filepath.Join("..", "..", "pkg", "api", apiVersion.VersionName, "response_gen.go")
		file, err := os.OpenFile(routesFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file to write generated code for response objects: %w", err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code for response objects: %w", err)
		}
		fmt.Println("code generation complete for response objects")
	}

	return nil
}

// ResponseObjects generates code for object type conversion used in API
// handlers.
func (gvc *GlobalVersionConfig) ExtensionResponseObjects() error {
	pluralize := pluralize.NewClient()
	for _, apiVersion := range gvc.Versions {
		f := NewFile(apiVersion.VersionName)
		f.HeaderComment("generated by 'threeport-sdk codegen api-version' - do not edit")
		f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi")

		objectTypesByPath := &Statement{}
		for _, name := range apiVersion.RouteNames {
			objectTypesByPath.Case(Id(fmt.Sprintf("Path%s", pluralize.Pluralize(name, 2, false)))).Block(
				Return().Id(fmt.Sprintf("ObjectType%s", name)),
			)
			objectTypesByPath.Line()

		}

		f.Comment("GetObjectTypeByPath returns the object type based on an API path.")
		f.Func().Id("GetObjectTypeByPath").Params(
			Id("path").String(),
		).Qual(
			"github.com/threeport/threeport/pkg/api/v0",
			"ObjectType",
		).Block(
			Switch(Id("path")).Block(
				objectTypesByPath,
			),
			Line(),
			Return().Qual(
				"github.com/threeport/threeport/pkg/api/v0",
				"ObjectTypeUnknown",
			),
		)

		// write code to file
		routesFilepath := filepath.Join("pkg", "api", apiVersion.VersionName, "response_gen.go")
		file, err := os.OpenFile(routesFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file to write generated code for response objects: %w", err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code for response objects: %w", err)
		}
		fmt.Println("code generation complete for response objects")
	}

	return nil
}
