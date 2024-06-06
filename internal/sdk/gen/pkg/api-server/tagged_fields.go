package apiserver

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenObjectTaggedFields generates the tagged field vars for each API object.
func GenObjectTaggedFields(gen *gen.Generator) error {
	for _, version := range gen.GlobalVersionConfig.Versions {
		f := NewFile(version.VersionName)
		f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

		f.ImportAlias(util.SetImportAlias(
			"github.com/threeport/threeport/pkg/api-server/lib/v0",
			"apiserver_lib",
			"tpapiserver_lib",
			gen.Extension,
		))

		taggedFieldVars := &Statement{}

		for _, name := range version.DatabaseInitNames {
			taggedFieldVars.Id(fmt.Sprintf("%sTaggedFields", name)).Op("=").Id("make").Call(
				Map(String()).Op("*").Qual(
					"github.com/threeport/threeport/pkg/api-server/lib/v0",
					"FieldsByTag",
				),
			)
			taggedFieldVars.Line()
		}

		f.Var().Defs(
			taggedFieldVars,
		)

		// write code to file
		genFilepath := filepath.Join(
			"pkg",
			"api-server",
			version.VersionName,
			"tagged_fields_gen.go",
		)
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for API tagged field vars written to %s", genFilepath))
	}

	return nil
}
