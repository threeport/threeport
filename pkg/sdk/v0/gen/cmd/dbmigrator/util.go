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

// GenDbMigratorUtils generates the migrations utils.
func GenDbMigratorUtils(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("migrations")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	f.Func().Id("getGormDbFromContext").Params(
		Id("ctx").Qual("context", "Context"),
	).Params(
		Op("*").Qual("gorm.io/gorm", "DB"),
		Error(),
	).Block(
		Id("contextGorm").Op(":=").Id("ctx").Dot("Value").Call(Lit("gormdb")),
		If(Id("contextGorm").Op("==").Nil()).Block(
			Return(Nil(), Qual("fmt", "Errorf").Call(Lit("could not retrieve gormdb from ctx"))),
		),
		Line(),

		Var().Id("gormDb").Op("*").Qual("gorm.io/gorm", "DB"),
		If(List(Id("g"), Id("ok")).Op(":=").Id("contextGorm").Op(".(*").Qual("gorm.io/gorm", "DB").Op(")"), Id("ok")).Block(
			Id("gormDb").Op("=").Id("g"),
		).Else().Block(
			Return(Nil(), Qual("fmt", "Errorf").Call(Lit("could not type convert gormdb from ctx"))),
		),
		Line(),

		Return(Id("gormDb"), Nil()),
	)
	f.Line()

	f.Comment("createMissingTables creates a table for each model and many-to-many join that has none.")
	f.Comment("GORM's CreateTable on a parent model does not add that model's join tables.")
	f.Func().Id("createMissingTables").Params(
		Id("gormDb").Op("*").Qual("gorm.io/gorm", "DB"),
		Id("models").Index().Interface(),
	).Error().Block(
		Comment("create model tables"),
		For(List(Id("_"), Id("model")).Op(":=").Range().Id("models")).Block(
			If(Id("gormDb").Dot("Migrator").Call().Dot("HasTable").Call(Id("model"))).Block(
				Continue(),
			),
			If(Err().Op(":=").Id("gormDb").Dot("Migrator").Call().Dot("CreateTable").Call(Id("model")), Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to create table for %T: %w"), Id("model"), Err())),
			),
		),
		Line(),
		Comment("create join tables after both sides exist"),
		For(List(Id("_"), Id("model")).Op(":=").Range().Id("models")).Block(
			Id("stmt").Op(":=").Op("&").Qual("gorm.io/gorm", "Statement").Values(Dict{
				Id("DB"): Id("gormDb"),
			}),
			If(Err().Op(":=").Id("stmt").Dot("Parse").Call(Id("model")), Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to parse %T: %w"), Id("model"), Err())),
			),
			If(Id("stmt").Dot("Schema").Op("==").Nil()).Block(
				Continue(),
			),
			For(List(Id("_"), Id("rel")).Op(":=").Range().Id("stmt").Dot("Schema").Dot("Relationships").Dot("Many2Many")).Block(
				If(Id("rel").Dot("JoinTable").Op("==").Nil().Op("||").Id("rel").Dot("Field").Op("==").Nil().Op("||").Id("rel").Dot("Field").Dot("IgnoreMigration")).Block(
					Continue(),
				),
				Id("join").Op(":=").Qual("reflect", "New").Call(Id("rel").Dot("JoinTable").Dot("ModelType")).Dot("Interface").Call(),
				If(Id("gormDb").Dot("Migrator").Call().Dot("HasTable").Call(Id("join"))).Block(
					Continue(),
				),
				If(Err().Op(":=").Id("gormDb").Dot("Migrator").Call().Dot("CreateTable").Call(Id("join")), Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit("failed to create join table %s: %w"),
						Id("rel").Dot("JoinTable").Dot("Table"),
						Err(),
					)),
				),
			),
		),
		Line(),
		Return(Nil()),
	)
	f.Line()

	f.Comment("dropTables drops many-to-many join tables, then each model table.")
	f.Func().Id("dropTables").Params(
		Id("gormDb").Op("*").Qual("gorm.io/gorm", "DB"),
		Id("models").Index().Interface(),
	).Error().Block(
		For(List(Id("_"), Id("model")).Op(":=").Range().Id("models")).Block(
			Id("stmt").Op(":=").Op("&").Qual("gorm.io/gorm", "Statement").Values(Dict{
				Id("DB"): Id("gormDb"),
			}),
			If(Err().Op(":=").Id("stmt").Dot("Parse").Call(Id("model")), Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to parse %T: %w"), Id("model"), Err())),
			),
			If(Id("stmt").Dot("Schema").Op("==").Nil()).Block(
				Continue(),
			),
			For(List(Id("_"), Id("rel")).Op(":=").Range().Id("stmt").Dot("Schema").Dot("Relationships").Dot("Many2Many")).Block(
				If(Id("rel").Dot("JoinTable").Op("==").Nil()).Block(
					Continue(),
				),
				If(Err().Op(":=").Id("gormDb").Dot("Migrator").Call().Dot("DropTable").Call(Id("rel").Dot("JoinTable").Dot("Table")), Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit("failed to drop join table %s: %w"),
						Id("rel").Dot("JoinTable").Dot("Table"),
						Err(),
					)),
				),
			),
		),
		Line(),
		For(List(Id("_"), Id("model")).Op(":=").Range().Id("models")).Block(
			If(Err().Op(":=").Id("gormDb").Dot("Migrator").Call().Dot("DropTable").Call(Id("model")), Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("could not drop table with gorm db: %w"), Err())),
			),
		),
		Line(),
		Return(Nil()),
	)

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join("cmd", "database-migrator", "migrations", "util_gen.go")
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for DB migrator migrations utils written to %s", genFilepath))
	}

	return nil
}
