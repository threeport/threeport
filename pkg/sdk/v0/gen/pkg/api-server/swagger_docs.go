package apiserver

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/dave/jennifer/jen"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// moduleSwaggerDocsPath is where a module's swagger docs package lives, which
// its API server imports and its GenerateSwaggerDocs mage target writes.
var moduleSwaggerDocsPath = filepath.Join("pkg", "api-server", "v0", "docs", "docs.go")

// GenModuleSwaggerDocsPlaceholder writes a module's swagger docs package the
// first time, so a module compiles before its docs are generated: its API
// server imports the package. The placeholder documents no paths, and 'mage
// dev:generateSwaggerDocs' replaces it with docs describing the module's API.
// An existing package is never overwritten.
func GenModuleSwaggerDocsPlaceholder(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	title := sdkConfig.ApiDocs.Title
	if title == "" {
		title = "Threeport Module API"
	}
	document, err := json.Marshal(map[string]any{
		"swagger": "2.0",
		"info": map[string]any{
			"title":       title,
			"description": sdkConfig.ApiDocs.Description,
		},
		"paths": map[string]any{},
	})
	if err != nil {
		return fmt.Errorf("failed to build the placeholder swagger document: %w", err)
	}

	f := NewFile("docs")
	f.PackageComment("Package docs holds the module API's swagger docs. This placeholder lets")
	f.PackageComment("the module compile before they are generated, and documents no paths:")
	f.PackageComment("run 'mage dev:generateSwaggerDocs' to replace it with docs describing the")
	f.PackageComment("module's API.")

	f.Comment("SwaggerInfo holds the module API's swagger document.")
	f.Var().Id("SwaggerInfo").Op("=").Op("&").Qual("github.com/swaggo/swag", "Spec").Values(Dict{
		Id("Title"):            Lit(title),
		Id("Description"):      Lit(sdkConfig.ApiDocs.Description),
		Id("InfoInstanceName"): Lit("swagger"),
		Id("SwaggerTemplate"):  Lit(string(document)),
	})
	f.Line()

	f.Func().Id("init").Params().Block(
		Qual("github.com/swaggo/swag", "Register").Call(
			Id("SwaggerInfo").Dot("InstanceName").Call(),
			Id("SwaggerInfo"),
		),
	)

	// write code to file if not excluded by SDK config, and never over the
	// docs the module generated
	if slices.Contains(sdkConfig.ExcludeFiles, moduleSwaggerDocsPath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", moduleSwaggerDocsPath))
		return nil
	}
	written, err := util.WriteCodeToFile(f, moduleSwaggerDocsPath, false)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", moduleSwaggerDocsPath, err)
	}
	if written {
		cli.Info(fmt.Sprintf("placeholder swagger docs written to %s", moduleSwaggerDocsPath))
	}

	return nil
}
