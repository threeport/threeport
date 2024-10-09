package root

import (
	"fmt"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenMagefile generates the source code for mage which is a Make-like tool
// using Go.
// Ref: https://github.com/magefile/mage
func GenMagefile(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("main")
	f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

	f.PackageComment("+build mage")

	f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
	f.ImportAlias(
		fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
		"installer",
	)

	// set function names for each component
	buildApiFuncName := "BuildApi"
	buildDbMigratorFuncName := "BuildDbMigrator"
	buildFuncNames := []string{buildApiFuncName, buildDbMigratorFuncName}

	buildApiImageFuncName := "BuildApiImage"
	buildDbMigratorImageFuncName := "BuildDbMigratorImage"
	buildImageFuncNames := []string{buildApiImageFuncName, buildDbMigratorImageFuncName}

	buildApiDevImageFuncName := "BuildApiDevImage"
	buildDbMigratorDevImageFuncName := "BuildDbMigratorDevImage"
	buildDevImageFuncNames := []string{buildApiDevImageFuncName, buildDbMigratorDevImageFuncName}

	// binary build function for API
	f.Comment(fmt.Sprintf("%s builds the REST API binary.", buildApiFuncName))
	f.Func().Id(buildApiFuncName).Params(Id("arch").String()).Error().Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err()),
		),
		Line(),

		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildBinary",
		).Call(
			Line().Id("workingDir"),
			Line().Id("arch"),
			Line().Lit("rest-api"),
			Line().Lit("cmd/rest-api/main_gen.go"),
			Line().Lit(false),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build rest-api binary: %w"),
				Err(),
			),
		),
		Line(),

		Qual("fmt", "Println").Call(Lit("binary built and available at bin/rest-api")),
		Line(),

		Return().Nil(),
	)
	f.Line()

	// dev binary build function for API
	f.Comment("BuildDevApi builds the REST API binary for the architcture of the machine")
	f.Comment("where it is built.")
	f.Func().Id("BuildDevApi").Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		If(Err().Op(":=").Id(buildApiFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build dev rest-api binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// release binary build function for API (amd64 arch)
	f.Comment("BuildReleaseApi builds the REST API binary for amd64 architecture.")
	f.Func().Id("BuildReleaseApi").Params().Error().Block(
		If(Err().Op(":=").Id(buildApiFuncName).Call(Lit("amd64")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build release rest-api binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// image build and push function for API
	apiImageName := "threeport-rest-api"
	if gen.Extension {
		apiImageName = fmt.Sprintf(
			"threeport-%s-rest-api",
			strcase.ToSnake(sdkConfig.ExtensionName),
		)
	}
	f.Comment(fmt.Sprintf("%s builds and pushes a REST API container image.", buildApiImageFuncName))
	f.Func().Id(buildApiImageFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Parens(Error()).Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err())),
		),
		Line(),

		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildImage",
		).Call(
			Line().Id("workingDir"),
			Line().Lit("cmd/rest-api/image/Dockerfile-alpine"),
			Line().Id("arch"),
			Line().Id("imageRepo"),
			Line().Lit(apiImageName),
			Line().Id("imageTag"),
			Line().True(),
			Line().False(),
			Line().Lit(""),
			Line(),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to build and push rest-api image: %w"), Err())),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// dev image build and push function for API
	f.Comment(fmt.Sprintf("%s builds and pushes a development REST API container image.", buildApiDevImageFuncName))
	f.Func().Id(buildApiDevImageFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		If(Err().Op(":=").Id(buildApiImageFuncName).Call(
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageTag",
			),
			Line().Id("arch"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build and push dev rest-api image: %w"),
				Err(),
			),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// release image build and push function for API
	f.Comment("BuildApiReleaseImage builds and pushes a release REST API container image.")
	f.Func().Id("BuildApiReleaseImage").Params().Error().Block(
		If(Err().Op(":=").Id(buildApiImageFuncName).Call(
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageTag",
			),
			Line().Lit("amd64"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build and push release rest-api image: %w"),
				Err(),
			),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// binary build function for database migrator
	f.Comment(fmt.Sprintf("%s builds the database migrator binary.", buildDbMigratorFuncName))
	f.Func().Id(buildDbMigratorFuncName).Params(Id("arch").String()).Error().Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err()),
		),
		Line(),

		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildBinary",
		).Call(
			Line().Id("workingDir"),
			Line().Id("arch"),
			Line().Lit("database-migrator"),
			Line().Lit("cmd/database-migrator/main_gen.go"),
			Line().Lit(false),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build database-migrator binary: %w"),
				Err(),
			),
		),
		Line(),

		Qual("fmt", "Println").Call(Lit("binary built and available at bin/database-migrator")),
		Line(),

		Return().Nil(),
	)
	f.Line()

	// dev binary build function for database migrator
	f.Comment("BuildDevDbMigrator builds the database migrator binary for the architcture of the machine")
	f.Comment("where it is built.")
	f.Func().Id("BuildDevDbMigrator").Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		If(Err().Op(":=").Id(buildDbMigratorFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build dev database-migrator binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// release binary build function for database migrator (amd64 arch)
	f.Comment("BuildReleaseDbMigrator builds the database migrator binary for amd64 architecture.")
	f.Func().Id("BuildReleaseDbMigrator").Params().Error().Block(
		If(Err().Op(":=").Id(buildDbMigratorFuncName).Call(Lit("amd64")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build release database-migrator binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// image build and push function for database migrator
	dbMigratorImageName := "threeport-database-migrator"
	if gen.Extension {
		dbMigratorImageName = fmt.Sprintf(
			"threeport-%s-database-migrator",
			strcase.ToSnake(sdkConfig.ExtensionName),
		)
	}
	f.Comment(fmt.Sprintf("%s builds and pushes a database migrator container image.", buildDbMigratorImageFuncName))
	f.Func().Id(buildDbMigratorImageFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Parens(Error()).Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err())),
		),
		Line(),

		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildImage",
		).Call(
			Line().Id("workingDir"),
			Line().Lit("cmd/database-migrator/image/Dockerfile-alpine"),
			Line().Id("arch"),
			Line().Id("imageRepo"),
			Line().Lit(dbMigratorImageName),
			Line().Id("imageTag"),
			Line().True(),
			Line().False(),
			Line().Lit(""),
			Line(),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to build and push database-migrator image: %w"), Err())),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// dev image build and push function for database migrator
	f.Comment(fmt.Sprintf("%s builds and pushes a development database migrator container image.", buildDbMigratorDevImageFuncName))
	f.Func().Id(buildDbMigratorDevImageFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		If(Err().Op(":=").Id(buildDbMigratorImageFuncName).Call(
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageTag",
			),
			Line().Id("arch"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build and push dev database-migrator image: %w"),
				Err(),
			),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// release image build and push function for database migrator
	f.Comment("BuildDbMigratorReleaseImage builds and pushes a release database migrator container image.")
	f.Func().Id("BuildDbMigratorReleaseImage").Params().Error().Block(
		If(Err().Op(":=").Id(buildDbMigratorImageFuncName).Call(
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageTag",
			),
			Line().Lit("amd64"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit("failed to build and push release database-migrator image: %w"),
				Err(),
			),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// binary build functions for controllers
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			// binary build function
			buildFuncName := fmt.Sprintf("Build%sController", objGroup.ControllerDomain)
			buildFuncNames = append(buildFuncNames, buildFuncName)

			f.Comment(fmt.Sprintf(
				"%s builds the binary for the %s.",
				buildFuncName,
				objGroup.ControllerName,
			))
			f.Func().Id(buildFuncName).Params(Id("arch").String()).Error().Block(
				List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err()),
				),
				Line(),

				If(Err().Op(":=").Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"BuildBinary",
				).Call(
					Line().Id("workingDir"),
					Line().Id("arch"),
					Line().Lit(objGroup.ControllerName),
					Line().Lit(fmt.Sprintf("cmd/%s/main_gen.go", objGroup.ControllerName)),
					Line().Lit(false),
					Line(),
				).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf("failed to build %s binary: %%w", objGroup.ControllerName)),
						Err(),
					),
				),
				Line(),

				Qual("fmt", "Println").Call(Lit(
					fmt.Sprintf("binary built and available at bin/%s", objGroup.ControllerName),
				)),
				Line(),

				Return().Nil(),
			)
			f.Line()

			// dev binary build function
			buildDevFuncName := fmt.Sprintf("BuildDev%sController", objGroup.ControllerDomain)
			f.Comment(fmt.Sprintf(
				"%s builds the %s binary for the architcture of the machine",
				buildDevFuncName,
				objGroup.ControllerName,
			))
			f.Comment("where it is built.")
			f.Func().Id(buildDevFuncName).Params().Error().Block(
				List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
				),
				Line(),
				If(Err().Op(":=").Id(buildFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit(fmt.Sprintf(
						"failed to build dev %s binary: %%w",
						objGroup.ControllerName,
					)), Err()),
				),
				Line(),
				Return().Nil(),
			)
			f.Line()

			// release binary build function (amd64 arch)
			buildReleaseFuncName := fmt.Sprintf("BuildRelease%sController", objGroup.ControllerDomain)
			f.Comment(fmt.Sprintf(
				"%s builds the %s binary for amd64 architecture.",
				buildReleaseFuncName,
				objGroup.ControllerName,
			))
			f.Func().Id(buildReleaseFuncName).Params().Error().Block(
				If(Err().Op(":=").Id(buildFuncName).Call(Lit("amd64")).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit(fmt.Sprintf(
						"failed to build release %s binary: %%w",
						objGroup.ControllerName,
					)), Err()),
				),
				Line(),
				Return().Nil(),
			)
			f.Line()

			// image build and push function
			buildImageFuncName := fmt.Sprintf("Build%sControllerImage", objGroup.ControllerDomain)
			buildImageFuncNames = append(buildImageFuncNames, buildImageFuncName)

			f.Comment(fmt.Sprintf(
				"%s builds and pushes the container image for the %s.",
				buildImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Id(buildImageFuncName).Params(
				Line().Id("imageRepo").String(),
				Line().Id("imageTag").String(),
				Line().Id("arch").String(),
				Line(),
			).Error().Block(
				List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory for extension repo: %w"), Err())),
				),
				Line(),

				If(Err().Op(":=").Qual(
					"github.com/threeport/threeport/pkg/util/v0",
					"BuildImage",
				).Call(
					Line().Id("workingDir"),
					Line().Lit(fmt.Sprintf("cmd/%s/image/Dockerfile-alpine", objGroup.ControllerName)),
					Line().Id("arch"),
					Line().Id("imageRepo"),
					Line().Lit(fmt.Sprintf("threeport-%s", objGroup.ControllerName)),
					Line().Id("imageTag"),
					Line().True(),
					Line().False(),
					Line().Lit(""),
					Line(),
				), Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf(
							"failed to build and push %s image: %%w",
							objGroup.ControllerName,
						)),
						Err(),
					)),
				),
				Line(),

				Return(Nil()),
			)
			f.Line()

			// dev image build and push function for controllers
			buildDevImageFuncName := fmt.Sprintf("Build%sControllerDevImage", objGroup.ControllerDomain)
			buildDevImageFuncNames = append(buildDevImageFuncNames, buildDevImageFuncName)
			f.Comment(fmt.Sprintf(
				"%s builds and pushes a development %s container image.",
				buildDevImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Id(buildDevImageFuncName).Params().Error().Block(
				List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
				),
				Line(),
				If(Err().Op(":=").Id(buildImageFuncName).Call(
					Line().Qual(
						fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
						"DevImageRepo",
					),
					Line().Qual(
						fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
						"DevImageTag",
					),
					Line().Id("arch"),
					Line(),
				).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf(
							"failed to build and push dev %s image: %%w",
							objGroup.ControllerName,
						)),
						Err(),
					),
				),
				Line(),

				Return().Nil(),
			)
			f.Line()

			// release image build and push function
			buildReleaseImageFuncName := fmt.Sprintf("Build%sControllerReleaseImage", objGroup.ControllerDomain)
			f.Comment(fmt.Sprintf(
				"%s builds and pushes a release %s container image.",
				buildReleaseImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Id(buildReleaseImageFuncName).Params().Error().Block(
				If(Err().Op(":=").Id(buildImageFuncName).Call(
					Line().Qual(
						fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
						"DevImageRepo",
					),
					Line().Qual(
						fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
						"DevImageTag",
					),
					Line().Lit("amd64"),
					Line(),
				).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(
						Lit(fmt.Sprintf(
							"failed to build and push release %s image: %%w",
							objGroup.ControllerName,
						)),
						Err(),
					),
				),
				Line(),

				Return(Nil()),
			)
			f.Line()

		}
	}
	f.Line()

	// build all binaries
	buildAllFuncName := "BuildAll"
	f.Comment(fmt.Sprintf("%s builds the binaries for all components.", buildAllFuncName))
	f.Func().Id(buildAllFuncName).Params(Id("arch").String()).Error().BlockFunc(func(g *Group) {
		for _, funcName := range buildFuncNames {
			g.If(Err().Op(":=").Id(funcName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build binary: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// build and push all images
	buildAllImagesFuncName := "BuildAllImages"
	f.Comment(fmt.Sprintf("%s builds and pushes images for all components.", buildAllImagesFuncName))
	f.Func().Id(buildAllImagesFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Error().BlockFunc(func(g *Group) {
		for _, funcName := range buildImageFuncNames {
			g.If(Err().Op(":=").Id(funcName).Call(
				Id("imageRepo"),
				Id("imageTag"),
				Id("arch"),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build and push image: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// build and push all dev images
	buildAllDevImagesFuncName := "BuildAllDevImages"
	f.Comment(fmt.Sprintf("%s builds and pushes development images for all components.", buildAllDevImagesFuncName))
	f.Func().Id(buildAllDevImagesFuncName).Params().Error().BlockFunc(func(g *Group) {
		for _, funcName := range buildDevImageFuncNames {
			g.If(Err().Op(":=").Id(funcName).Call().Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build and push image: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// dev image loads to kind clusters
	f.Comment("LoadDevImage builds and loads an image to the provided kind cluster.")
	f.Func().Id("LoadDevImage").Params(
		Id("kindClusterName").String(),
		Id("component").String(),
	).Error().BlockFunc(func(g *Group) {
		g.List(Id("workingDir"), Id("arch"), Id("err")).Op(":=").Id("getBuildVals").Call()
		g.If(Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get build values: %w"), Id("err"))),
		)
		g.Line()

		g.Id("imageName").Op(":=").Qual("fmt", "Sprintf").Call(Lit("threeport-%s"), Id("component"))
		if gen.Extension {
			g.If(Id("component").Op("==").Lit("rest-api")).Block(
				Id("imageName").Op("=").Qual("fmt", "Sprintf").Call(
					Lit("threeport-%s-%s"),
					Lit(strcase.ToSnake(sdkConfig.ExtensionName)),
					Id("component"),
				),
			)
		}
		g.Line()

		g.If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildImage",
		).Call(
			Line().Id("workingDir"),
			Line().Qual("fmt", "Sprintf").Call(Lit("cmd/%s/image/Dockerfile-alpine"), Id("component")),
			Line().Id("arch"),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageRepo",
			),
			Line().Id("imageName"),
			Line().Qual(
				fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath),
				"DevImageTag",
			),
			Line().False(),
			Line().True(),
			Line().Id("kindClusterName"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to build and load image: %w"), Id("err"))),
		)
		g.Line()

		g.Return(Nil())
	})
	f.Line()

	// extension plugin build
	if gen.Extension {
		f.Comment("BuildPlugin compiles the extension's tptctl plugin")
		f.Func().Id("BuildPlugin").Params().Error().Block(
			Id("buildCmd").Op(":=").Qual("os/exec", "Command").Call(
				Line().Lit("go"),
				Line().Lit("build"),
				Line().Lit("-o"),
				Line().Lit(fmt.Sprintf(
					"bin/%s.so",
					strcase.ToKebab(sdkConfig.ExtensionName),
				)),
				Line().Lit("-buildmode=plugin"),
				Line().Lit(fmt.Sprintf(
					"cmd/%s/main_gen.go",
					strcase.ToSnake(sdkConfig.ExtensionName),
				)),
				Line(),
			),
			Line(),

			Id("output").Op(",").Id("err").Op(":=").Id("buildCmd").Dot("CombinedOutput").Call(),
			If(Id("err").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("build failed for tptctl plugin with output '%s': %w"),
					Id("output"),
					Id("err"),
				)),
			),
			Line(),

			Qual("fmt", "Println").Call(Lit(fmt.Sprintf(
				"tptctl plugin built and available at bin/%s",
				strcase.ToKebab(sdkConfig.ExtensionName),
			))),
			Line(),

			Return(Nil()),
		)
		f.Line()
	}

	// API docs generation
	f.Comment("Docs generates the API server documentation that is served by the API")
	f.Func().Id("Docs").Params().Error().Block(
		Id("docsDestination").Op(":=").Lit("pkg/api-server/v0/docs"),
		Id("swagCmd").Op(":=").Qual("os/exec", "Command").Call(
			Line().Lit("swag"),
			Line().Lit("init"),
			Line().Lit("--dir"),
			Line().Lit("cmd/rest-api,pkg/api,pkg/api-server/v0"),
			Line().Lit("--parseDependency"),
			Line().Lit("--generalInfo"),
			Line().Lit("main_gen.go"),
			Line().Lit("--output"),
			Line().Id("docsDestination"),
			Line(),
		),
		Line(),

		List(Id("output"), Err()).Op(":=").Id("swagCmd").Dot("CombinedOutput").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("API docs generation failed with output '%s': %w"), Id("output"), Err())),
		),
		Line(),

		Qual("fmt", "Printf").Call(Lit("API docs generated in %s\n"), Id("docsDestination")),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// build vals utility function
	f.Comment("getBuildVals returns the working directory and arch for builds.")
	f.Func().Id("getBuildVals").Params().Params(
		String(),
		String(),
		Error(),
	).Block(
		List(Id("workingDir"), Err()).Op(":=").Qual("os", "Getwd").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(""), Lit(""), Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err())),
		),
		Line(),

		Id("arch").Op(":=").Qual("runtime", "GOARCH"),
		Line(),

		Return(Id("workingDir"), Id("arch"), Nil()),
	)

	// write code to file
	genFilepath := "magefile_gen.go"
	_, err := util.WriteCodeToFile(f, genFilepath, true)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	cli.Info(fmt.Sprintf("source code for magefile written to %s", genFilepath))

	return nil
}
