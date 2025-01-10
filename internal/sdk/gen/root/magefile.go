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

	// set installer package based for threeport and extensions
	var installerPkg string
	if gen.Extension {
		installerPkg = fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath)
	} else {
		installerPkg = fmt.Sprintf("%s/pkg/threeport-installer/v0", gen.ModulePath)
	}

	// set release image repo constant
	var releaseImageRepoConst string
	if gen.Extension {
		releaseImageRepoConst = "ReleaseImageRepo"
	} else {
		releaseImageRepoConst = "ThreeportImageRepo"
	}

	f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
	f.ImportAlias(installerPkg, "installer")

	// set function names for each component
	buildApiFuncName := "ApiBin"
	buildDbMigratorFuncName := "DbMigratorBin"
	buildAgentFuncName := "AgentBin"
	buildFuncNames := []string{buildApiFuncName, buildDbMigratorFuncName}

	buildApiDevFuncName := "ApiBinDev"
	buildDbMigratorDevFuncName := "DbMigratorBinDev"
	buildAgentDevFuncName := "AgentBinDev"
	buildDevFuncNames := []string{buildApiDevFuncName, buildDbMigratorDevFuncName}

	buildApiReleaseFuncName := "ApiBinRelease"
	buildDbMigratorReleaseFuncName := "DbMigratorBinRelease"
	buildAgentReleaseFuncName := "AgentBinRelease"
	buildReleaseFuncNames := []string{buildApiReleaseFuncName, buildDbMigratorReleaseFuncName}

	buildApiImageFuncName := "ApiImage"
	buildDbMigratorImageFuncName := "DbMigratorImage"
	buildAgentImageFuncName := "AgentImage"
	buildImageFuncNames := []string{buildApiImageFuncName, buildDbMigratorImageFuncName}

	buildApiDevImageFuncName := "ApiImageDev"
	buildDbMigratorDevImageFuncName := "DbMigratorImageDev"
	buildAgentDevImageFuncName := "AgentImageDev"
	buildDevImageFuncNames := []string{buildApiDevImageFuncName, buildDbMigratorDevImageFuncName}

	buildApiReleaseImageFuncName := "ApiImageRelease"
	buildDbMigratorReleaseImageFuncName := "DbMigratorImageRelease"
	buildAgentReleaseImageFuncName := "AgentImageRelease"
	buildReleaseImageFuncNames := []string{buildApiReleaseImageFuncName, buildDbMigratorReleaseImageFuncName}

	f.Const().Id("releaseArch").Op("=").Lit("amd64")
	f.Line()

	namespaces := []string{"Build", "Test", "Install", "Dev"}
	for _, ns := range namespaces {
		f.Comment(fmt.Sprintf(
			"%s provides a type for methods that implement %s targets.", ns, strcase.ToLowerCamel(ns),
		))
		f.Type().Id(ns).Qual("github.com/magefile/mage/mg", "Namespace")
		f.Line()
	}

	// binary build function for API
	f.Comment(fmt.Sprintf("%s builds the REST API binary.", buildApiFuncName))
	f.Func().Params(Id("Build")).Id(buildApiFuncName).Params(Id("arch").String()).Error().Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err()),
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
	f.Comment(fmt.Sprintf("%s builds the REST API binary for the architcture of the machine", buildApiDevFuncName))
	f.Comment("where it is built.")
	f.Func().Params(Id("Build")).Id(buildApiDevFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildApiFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build dev rest-api binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// release binary build function for API
	f.Comment(fmt.Sprintf("%s builds the REST API binary for release architecture.", buildApiReleaseFuncName))
	f.Func().Params(Id("Build")).Id(buildApiReleaseFuncName).Params().Error().Block(
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildApiFuncName).Call(Id("releaseArch")).Op(";").Err().Op("!=").Nil()).Block(
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
	f.Func().Params(Id("Build")).Id(buildApiImageFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Parens(Error()).Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err())),
		),
		Line(),

		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildApiFuncName).Call(Id("arch"))).Op(";").Err().Op("!=").Nil().Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build binary for image build: %w"), Err()),
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
	f.Func().Params(Id("Build")).Id(buildApiDevImageFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildApiImageFuncName).Call(
			Line().Qual(
				installerPkg,
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call(),
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
	f.Comment(fmt.Sprintf("%s builds and pushes a release REST API container image.", buildApiReleaseImageFuncName))
	f.Func().Params(Id("Build")).Id(buildApiReleaseImageFuncName).Params().Error().Block(
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildApiImageFuncName).Call(
			Line().Qual(
				installerPkg,
				releaseImageRepoConst,
			),
			Line().Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call(),
			Line().Id("releaseArch"),
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
	f.Func().Params(Id("Build")).Id(buildDbMigratorFuncName).Params(Id("arch").String()).Error().Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err()),
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
	f.Comment(fmt.Sprintf("%s builds the database migrator binary for the architcture of the machine", buildDbMigratorDevFuncName))
	f.Comment("where it is built.")
	f.Func().Params(Id("Build")).Id(buildDbMigratorDevFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildDbMigratorFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build dev database-migrator binary: %w"), Err()),
		),
		Line(),
		Return().Nil(),
	)
	f.Line()

	// release binary build function for database migrator
	f.Comment(fmt.Sprintf("%s builds the database migrator binary for release architecture.", buildDbMigratorReleaseFuncName))
	f.Func().Params(Id("Build")).Id(buildDbMigratorReleaseFuncName).Params().Error().Block(
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildDbMigratorFuncName).Call(Id("releaseArch")).Op(";").Err().Op("!=").Nil()).Block(
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
	f.Func().Params(Id("Build")).Id(buildDbMigratorImageFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Parens(Error()).Block(
		List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err())),
		),
		Line(),

		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildDbMigratorFuncName).Call(Id("arch"))).Op(";").Err().Op("!=").Nil().Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to build binary for image build: %w"), Err()),
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
	f.Func().Params(Id("Build")).Id(buildDbMigratorDevImageFuncName).Params().Error().Block(
		List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
		),
		Line(),
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildDbMigratorImageFuncName).Call(
			Line().Qual(
				installerPkg,
				"DevImageRepo",
			),
			Line().Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call(),
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
	f.Comment(fmt.Sprintf("%s builds and pushes a release database migrator container image.", buildDbMigratorReleaseImageFuncName))
	f.Func().Params(Id("Build")).Id(buildDbMigratorReleaseImageFuncName).Params().Error().Block(
		Id("build").Op(":=").Id("Build").Values(),
		If(Err().Op(":=").Id("build").Dot(buildDbMigratorImageFuncName).Call(
			Line().Qual(
				installerPkg,
				releaseImageRepoConst,
			),
			Line().Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call(),
			Line().Id("releaseArch"),
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

	if !gen.Extension {
		// add function names to "build all" functions
		buildFuncNames = append(buildFuncNames, buildAgentFuncName)
		buildDevFuncNames = append(buildDevFuncNames, buildAgentDevFuncName)
		buildReleaseFuncNames = append(buildReleaseFuncNames, buildAgentReleaseFuncName)
		buildImageFuncNames = append(buildImageFuncNames, buildAgentImageFuncName)
		buildDevImageFuncNames = append(buildDevImageFuncNames, buildAgentDevImageFuncName)
		buildReleaseImageFuncNames = append(buildReleaseImageFuncNames, buildAgentReleaseImageFuncName)

		// binary build function for agent
		f.Comment(fmt.Sprintf("%s builds the agent binary.", buildAgentFuncName))
		f.Func().Params(Id("Build")).Id(buildAgentFuncName).Params(Id("arch").String()).Error().Block(
			List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err()),
			),
			Line(),

			If(Err().Op(":=").Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"BuildBinary",
			).Call(
				Line().Id("workingDir"),
				Line().Id("arch"),
				Line().Lit("agent"),
				Line().Lit("cmd/agent/main.go"),
				Line().Lit(false),
				Line(),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build agent binary: %w"),
					Err(),
				),
			),
			Line(),

			Qual("fmt", "Println").Call(Lit("binary built and available at bin/agent")),
			Line(),

			Return().Nil(),
		)
		f.Line()

		// dev binary build function for agent
		f.Comment(fmt.Sprintf("%s builds the agent binary for the architcture of the machine", buildAgentDevFuncName))
		f.Comment("where it is built.")
		f.Func().Params(Id("Build")).Id(buildAgentDevFuncName).Params().Error().Block(
			List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
			),
			Line(),
			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot(buildAgentFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to build dev agent binary: %w"), Err()),
			),
			Line(),
			Return().Nil(),
		)
		f.Line()

		// release binary build function for agent
		f.Comment(fmt.Sprintf("%s builds the agent binary for release architecture.", buildAgentReleaseFuncName))
		f.Func().Params(Id("Build")).Id(buildAgentReleaseFuncName).Params().Error().Block(
			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot(buildAgentFuncName).Call(Id("releaseArch")).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to build release agent binary: %w"), Err()),
			),
			Line(),
			Return().Nil(),
		)
		f.Line()

		// image build and push function for agent
		dbMigratorImageName := "threeport-agent"
		if gen.Extension {
			dbMigratorImageName = fmt.Sprintf(
				"threeport-%s-agent",
				strcase.ToSnake(sdkConfig.ExtensionName),
			)
		}
		f.Comment(fmt.Sprintf("%s builds and pushes a agent container image.", buildAgentImageFuncName))
		f.Func().Params(Id("Build")).Id(buildAgentImageFuncName).Params(
			Line().Id("imageRepo").String(),
			Line().Id("imageTag").String(),
			Line().Id("arch").String(),
			Line(),
		).Parens(Error()).Block(
			List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err())),
			),
			Line(),

			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot(buildAgentFuncName).Call(Id("arch"))).Op(";").Err().Op("!=").Nil().Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to build binary for image build: %w"), Err()),
			),
			Line(),

			If(Err().Op(":=").Qual(
				"github.com/threeport/threeport/pkg/util/v0",
				"BuildImage",
			).Call(
				Line().Id("workingDir"),
				Line().Lit("cmd/agent/image/Dockerfile-alpine"),
				Line().Id("arch"),
				Line().Id("imageRepo"),
				Line().Lit(dbMigratorImageName),
				Line().Id("imageTag"),
				Line().True(),
				Line().False(),
				Line().Lit(""),
				Line(),
			), Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to build and push agent image: %w"), Err())),
			),
			Line(),

			Return(Nil()),
		)
		f.Line()

		// dev image build and push function for agent
		f.Comment(fmt.Sprintf("%s builds and pushes a development agent container image.", buildAgentDevImageFuncName))
		f.Func().Params(Id("Build")).Id(buildAgentDevImageFuncName).Params().Error().Block(
			List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
			),
			Line(),
			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot(buildAgentImageFuncName).Call(
				Line().Qual(
					installerPkg,
					"DevImageRepo",
				),
				Line().Qual(
					fmt.Sprintf("%s/internal/version", gen.ModulePath),
					"GetVersion",
				).Call(),
				Line().Id("arch"),
				Line(),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build and push dev agent image: %w"),
					Err(),
				),
			),
			Line(),

			Return(Nil()),
		)
		f.Line()

		// release image build and push function for agent
		f.Comment(fmt.Sprintf("%s builds and pushes a release agent container image.", buildAgentReleaseImageFuncName))
		f.Func().Params(Id("Build")).Id(buildAgentReleaseImageFuncName).Params().Error().Block(
			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot(buildAgentImageFuncName).Call(
				Line().Qual(
					installerPkg,
					releaseImageRepoConst,
				),
				Line().Qual(
					fmt.Sprintf("%s/internal/version", gen.ModulePath),
					"GetVersion",
				).Call(),
				Line().Id("releaseArch"),
				Line(),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build and push release agent image: %w"),
					Err(),
				),
			),
			Line(),

			Return(Nil()),
		)
		f.Line()
	}

	// binary build functions for controllers
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			// set func names
			buildFuncName := fmt.Sprintf("%sControllerBin", objGroup.ControllerDomain)
			buildFuncNames = append(buildFuncNames, buildFuncName)

			buildDevFuncName := fmt.Sprintf("%sControllerBinDev", objGroup.ControllerDomain)
			buildDevFuncNames = append(buildDevFuncNames, buildDevFuncName)

			buildReleaseFuncName := fmt.Sprintf("%sControllerBinRelease", objGroup.ControllerDomain)
			buildReleaseFuncNames = append(buildReleaseFuncNames, buildReleaseFuncName)

			buildImageFuncName := fmt.Sprintf("%sControllerImage", objGroup.ControllerDomain)
			buildImageFuncNames = append(buildImageFuncNames, buildImageFuncName)

			buildDevImageFuncName := fmt.Sprintf("%sControllerImageDev", objGroup.ControllerDomain)
			buildDevImageFuncNames = append(buildDevImageFuncNames, buildDevImageFuncName)

			buildReleaseImageFuncName := fmt.Sprintf("%sControllerImageRelease", objGroup.ControllerDomain)
			buildReleaseImageFuncNames = append(buildReleaseImageFuncNames, buildReleaseImageFuncName)

			// binary build function
			f.Comment(fmt.Sprintf(
				"%s builds the binary for the %s.",
				buildFuncName,
				objGroup.ControllerName,
			))
			f.Func().Params(Id("Build")).Id(buildFuncName).Params(Id("arch").String()).Error().Block(
				List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err()),
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
			f.Comment(fmt.Sprintf(
				"%s builds the %s binary for the architcture of the machine",
				buildDevFuncName,
				objGroup.ControllerName,
			))
			f.Comment("where it is built.")
			f.Func().Params(Id("Build")).Id(buildDevFuncName).Params().Error().Block(
				List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
				),
				Line(),
				Id("build").Op(":=").Id("Build").Values(),
				If(Err().Op(":=").Id("build").Dot(buildFuncName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit(fmt.Sprintf(
						"failed to build dev %s binary: %%w",
						objGroup.ControllerName,
					)), Err()),
				),
				Line(),
				Return().Nil(),
			)
			f.Line()

			// release binary build function
			f.Comment(fmt.Sprintf(
				"%s builds the %s binary for release architecture.",
				buildReleaseFuncName,
				objGroup.ControllerName,
			))
			f.Func().Params(Id("Build")).Id(buildReleaseFuncName).Params().Error().Block(
				Id("build").Op(":=").Id("Build").Values(),
				If(Err().Op(":=").Id("build").Dot(buildFuncName).Call(Id("releaseArch")).Op(";").Err().Op("!=").Nil()).Block(
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
			f.Comment(fmt.Sprintf(
				"%s builds and pushes the container image for the %s.",
				buildImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Params(Id("Build")).Id(buildImageFuncName).Params(
				Line().Id("imageRepo").String(),
				Line().Id("imageTag").String(),
				Line().Id("arch").String(),
				Line(),
			).Error().Block(
				List(Id("workingDir"), Id("_"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(Lit("failed to get working directory: %w"), Err())),
				),
				Line(),

				Id("build").Op(":=").Id("Build").Values(),
				If(Err().Op(":=").Id("build").Dot(buildFuncName).Call(Id("arch"))).Op(";").Err().Op("!=").Nil().Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to build binary for image build: %w"), Err()),
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
			f.Comment(fmt.Sprintf(
				"%s builds and pushes a development %s container image.",
				buildDevImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Params(Id("Build")).Id(buildDevImageFuncName).Params().Error().Block(
				List(Id("_"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
				If(Err().Op("!=").Nil()).Block(
					Return().Qual("fmt", "Errorf").Call(Lit("failed to get local CPU architecture: %w"), Err()),
				),
				Line(),
				Id("build").Op(":=").Id("Build").Values(),
				If(Err().Op(":=").Id("build").Dot(buildImageFuncName).Call(
					Line().Qual(
						installerPkg,
						"DevImageRepo",
					),
					Line().Qual(
						fmt.Sprintf("%s/internal/version", gen.ModulePath),
						"GetVersion",
					).Call(),
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
			f.Comment(fmt.Sprintf(
				"%s builds and pushes a release %s container image.",
				buildReleaseImageFuncName,
				objGroup.ControllerName,
			))
			f.Func().Params(Id("Build")).Id(buildReleaseImageFuncName).Params().Error().Block(
				Id("build").Op(":=").Id("Build").Values(),
				If(Err().Op(":=").Id("build").Dot(buildImageFuncName).Call(
					Line().Qual(
						installerPkg,
						releaseImageRepoConst,
					),
					Line().Qual(
						fmt.Sprintf("%s/internal/version", gen.ModulePath),
						"GetVersion",
					).Call(),
					Line().Id("releaseArch"),
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
	buildAllFuncName := "AllBins"
	f.Comment(fmt.Sprintf("%s builds the binaries for all components.", buildAllFuncName))
	f.Func().Params(Id("Build")).Id(buildAllFuncName).Params(Id("arch").String()).Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call(Id("arch")).Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build binary: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// build all dev binaries
	buildAllDevFuncName := "AllBinsDev"
	f.Comment(fmt.Sprintf("%s builds the development binaries for all components.", buildAllFuncName))
	f.Func().Params(Id("Build")).Id(buildAllDevFuncName).Params().Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildDevFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call().Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build binary: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// build all release binaries
	buildAllReleaseFuncName := "AllBinsRelease"
	f.Comment(fmt.Sprintf("%s builds the release binaries for all components.", buildAllFuncName))
	f.Func().Params(Id("Build")).Id(buildAllReleaseFuncName).Params().Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildReleaseFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call().Op(";").Err().Op("!=").Nil()).Block(
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
	buildAllImagesFuncName := "AllImages"
	f.Comment(fmt.Sprintf("%s builds and pushes images for all components.", buildAllImagesFuncName))
	f.Func().Params(Id("Build")).Id(buildAllImagesFuncName).Params(
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildImageFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call(
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
	buildAllDevImagesFuncName := "AllImagesDev"
	f.Comment(fmt.Sprintf("%s builds and pushes development images for all components.", buildAllDevImagesFuncName))
	f.Func().Params(Id("Build")).Id(buildAllDevImagesFuncName).Params().Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildDevImageFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call().Op(";").Err().Op("!=").Nil()).Block(
				Return().Qual("fmt", "Errorf").Call(
					Lit("failed to build and push image: %w"),
					Err(),
				),
			)
			g.Line()
		}

		g.Return().Nil()
	})

	// build and push all release images
	buildAllReleaseImagesFuncName := "AllImagesRelease"
	f.Comment(fmt.Sprintf("%s builds and pushes development images for all components.", buildAllReleaseImagesFuncName))
	f.Func().Params(Id("Build")).Id(buildAllReleaseImagesFuncName).Params().Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildReleaseImageFuncNames {
			g.If(Err().Op(":=").Id("build").Dot(funcName).Call().Op(";").Err().Op("!=").Nil()).Block(
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
	f.Comment("LoadImage builds and loads an image to the provided kind cluster.")
	f.Func().Params(Id("Dev")).Id("LoadImage").Params(
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
				installerPkg,
				"DevImageRepo",
			),
			Line().Id("imageName"),
			Line().Qual(
				fmt.Sprintf("%s/internal/version", gen.ModulePath),
				"GetVersion",
			).Call(),
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
		f.Comment("Plugin compiles the extension's tptctl plugin.")
		f.Func().Params(Id("Build")).Id("Plugin").Params().Error().Block(
			Id("buildCmd").Op(":=").Qual("os/exec", "Command").Call(
				Line().Lit("go"),
				Line().Lit("build"),
				Line().Lit("-o"),
				Line().Lit(fmt.Sprintf(
					"bin/%s",
					strcase.ToKebab(sdkConfig.ExtensionName),
				)),
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
	f.Comment("GenerateSwaggerDocs generates the API server swagger documentation served by the API.")
	f.Func().Params(Id("Dev")).Id("GenerateSwaggerDocs").Params().Error().Block(
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

	// local registry creation
	f.Comment("LocalRegistryUp starts a docker container to serve as a local container registry.")
	f.Func().Params(Id("Dev")).Id("LocalRegistryUp").Params().Error().Block(
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev",
			"CreateLocalRegistry",
		).Call()).Op(";").Err().Op("!=").Nil().Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to create local container registry: %w"), Err())),
		),
		Line(),

		Return().Nil(),
	)

	// local registry deletion
	f.Comment("LocalRegistryDown stops and removes the local container registry.")
	f.Func().Params(Id("Dev")).Id("LocalRegistryDown").Params().Error().Block(
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev",
			"DeleteLocalRegistry",
		).Call()).Op(";").Err().Op("!=").Nil().Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to remove local container registry: %w"), Err())),
		),
		Line(),

		Return().Nil(),
	)

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
