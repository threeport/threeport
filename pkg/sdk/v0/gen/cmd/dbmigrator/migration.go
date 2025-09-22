package dbmigrator

import (
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/dave/jennifer/jen"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenDbMigratorMigration generates the migration used to set the database
// schema before the API server starts.
func GenDbMigratorMigration(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("migrations")
	f.HeaderComment(util.HeaderCommentGenMod)

	f.ImportAlias("github.com/pressly/goose/v3", "goose")

	// TODO: currently, only initial migration generation is supported.  When
	// subsequent migration generation is supported, this value will be
	// determined at runtime.
	migrationVersion := "000001"

	f.Func().Id("init").Params().Block(
		Qual("github.com/pressly/goose/v3", "AddMigrationNoTxContext").Call(
			Id(fmt.Sprintf("Up%s", migrationVersion)),
			Id(fmt.Sprintf("Down%s", migrationVersion)),
		),
	)
	f.Line()

	f.Func().Id(fmt.Sprintf("Up%s", migrationVersion)).Params(
		Id("ctx").Qual("context", "Context"),
		Id("db").Op("*").Qual("database/sql", "DB"),
	).Error().Block(
		List(Id("gormDb"), Err()).Op(":=").Id("getGormDbFromContext").Call(Id("ctx")),
		If(Err().Op("!=").Nil()).Block(
			Return(Err()),
		),
		Line(),

		If(Err().Op(":=").Id("gormDb").Dot("AutoMigrate").Call(
			Id(fmt.Sprintf("dbInterfaces%s", migrationVersion)).Call().Op("..."),
		),
			Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("could not run gorm AutoMigrate: %w"), Err())),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	f.Func().Id(fmt.Sprintf("Down%s", migrationVersion)).Params(
		Id("ctx").Qual("context", "Context"),
		Id("db").Op("*").Qual("database/sql", "DB"),
	).Error().Block(
		List(Id("gormDb"), Err()).Op(":=").Id("getGormDbFromContext").Call(Id("ctx")),
		If(Err().Op("!=").Nil()).Block(
			Return(Err()),
		),
		Line(),

		Id("tablesToDrop").Op(":=").Id(fmt.Sprintf("dbInterfaces%s", migrationVersion)).Call(),
		For(List(Id("_"), Id("table")).Op(":=").Range().Id("tablesToDrop")).Block(
			If(Err().Op(":=").Id("gormDb").Dot("Migrator").Call().Dot("DropTable").Call(
				Id("table"),
			),
				Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("could not drop table with gorm db: %w"), Err())),
			),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	f.Func().Id(fmt.Sprintf("dbInterfaces%s", migrationVersion)).Parens(Empty()).Params(
		Index().Interface(),
	).Block(
		Return().Index().Interface().BlockFunc(func(g *Group) {
			for _, version := range gen.GlobalVersionConfig.Versions {
				for _, name := range version.DatabaseInitNames {
					g.List(
						Op("&").Qual(
							fmt.Sprintf(
								"%s/pkg/api/%s", gen.ModulePath, version.VersionName,
							),
							name,
						).Values().Op(","),
					)
				}
			}
		}),
	)
	f.Line()

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join(
		"cmd",
		"database-migrator",
		"migrations",
		fmt.Sprintf("%s_init.go", migrationVersion),
	)
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		fileWritten, err := util.WriteCodeToFile(f, genFilepath, false)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		if fileWritten {
			cli.Info(fmt.Sprintf("source code for database migration written to %s", genFilepath))
		} else {
			cli.Info(fmt.Sprintf("source code for database migration already exists at %s - not overwritten", genFilepath))
		}
	}

	return nil
}
