package apiserver

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenHandlerWrapper generates the handler wrapper that wraps Threeport handlers
// for extensions.
func GenHandlerWrapper(gen *gen.Generator) error {
	for _, version := range gen.GlobalVersionConfig.Versions {
		f := NewFile("handlers")
		f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

		f.ImportAlias("github.com/nats-io/nats.go", "nats")
		f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0/handlers", "tp_handlers")

		f.Comment("Handler is a wrapper for the threeport Handler object.")
		f.Type().Id("Handler").Struct(
			Id("Handler").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0/handlers",
				"Handler",
			),
		)

		f.Comment("New returns a new Handler.")
		f.Func().Id("New").Params(
			Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
			Id("nc").Op("*").Qual("github.com/nats-io/nats.go", "Conn"),
			Id("rc").Qual("github.com/nats-io/nats.go", "JetStreamContext"),
		).Id("Handler").Block(
			Id("handler").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/api-server/v0/handlers",
				"New",
			).Call(List(Id("db"), Id("nc"), Id("rc"))),

			Return(Id("Handler").Values(Dict{
				Id("Handler"): Id("handler"),
			})),
		)

		// write code to file
		genFilepath := filepath.Join(
			"pkg",
			"api-server",
			version.VersionName,
			"handlers",
			"handlers_gen.go",
		)
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for extension handler wrapper written to %s", genFilepath))
	}

	return nil
}
