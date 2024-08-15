package apiserver

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk/gen"
	sdkutil "github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GenAddVersionsFuncs adds the functions to add all API object versions to the API
// server.
func GenAddVersionsFuncs(gen *gen.Generator) error {
	for _, version := range gen.GlobalVersionConfig.Versions {
		f := NewFile("versions")
		f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

		var versionFuncs []string
		for _, name := range version.RouteNames {
			if !util.StringSliceContains(versionFuncs, name, true) {
				versionFuncs = append(versionFuncs, name)
			}
		}

		f.Func().Id("AddVersions").Params().BlockFunc(func(g *Group) {
			for _, vf := range versionFuncs {
				g.Id(fmt.Sprintf("Add%sVersions", vf)).Call()
			}
		})

		// write code to file
		genFilepath := filepath.Join(
			"pkg",
			"api-server",
			version.VersionName,
			"versions",
			"versions_gen.go",
		)
		_, err := sdkutil.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code to add object versions to API server written to %s", genFilepath))
	}

	return nil
}