package root

import (
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// componentSpec carries the bits the magefile generator needs to emit a
// per-component build target: the binary name on disk (used both for the
// `bin/<arch>/<name>` output path and the `BINARY=<name>` build-arg), the
// package dir the Go compiler builds, the container image name, and the
// name of the generated package-only function that the AllImages* tasks
// call to skip redundant compile work.
type componentSpec struct {
	BinaryName       string
	PackageDir       string
	ImageName        string
	PackageFuncName  string
	DockerfileTarget string
}

// GenMagefile generates the source code for mage which is a Make-like tool
// using Go.
// Ref: https://github.com/magefile/mage
func GenMagefile(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("main")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	// set installer package for threeport and modules
	var installerPkg string
	if gen.Module {
		installerPkg = fmt.Sprintf("%s/pkg/installer/v0", gen.ModulePath)
	} else {
		installerPkg = fmt.Sprintf("%s/pkg/threeport-installer/v0", gen.ModulePath)
	}

	f.ImportAlias("github.com/threeport/threeport/pkg/util/v0", "util")
	f.ImportAlias(installerPkg, "installer")
	f.ImportAlias("github.com/threeport/threeport/pkg/cli/v0", "cli")

	// the module ci targets read the embedded version
	if gen.Module {
		f.ImportAlias(fmt.Sprintf("%s/internal/version", gen.ModulePath), "version")
	}

	// collect specs for every per-component image function so AllImages
	// can pre-build the binaries up front in one go build per arch.
	var allComponents []componentSpec

	// set function names for each component
	buildApiFuncName := "ApiBin"
	buildDbMigratorFuncName := "DbMigratorBin"
	buildAgentFuncName := "AgentBin"
	buildFuncNames := []string{buildApiFuncName, buildDbMigratorFuncName}

	buildApiImageFuncName := "ApiImage"
	buildDbMigratorImageFuncName := "DbMigratorImage"
	buildAgentImageFuncName := "AgentImage"

	namespaces := []string{"Build", "Test", "Install", "Dev", "Package", "Download"}
	for _, ns := range namespaces {
		f.Comment(fmt.Sprintf(
			"%s provides a type for methods that implement %s targets.", ns, strcase.ToLowerCamel(ns),
		))
		f.Type().Id(ns).Qual("github.com/magefile/mage/mg", "Namespace")
		f.Line()
	}

	// modules emit a Ci namespace whose targets feed values to CI workflow
	// steps and tear down what an integration job leaves behind. The core
	// threeport repo keeps its hand-written ci targets, so this stays gated on
	// the module case.
	if gen.Module {
		f.Comment("Ci provides a type for methods that emit values for CI workflow steps.")
		f.Type().Id("Ci").Qual("github.com/magefile/mage/mg", "Namespace")
		f.Line()

		emitCiEnvFunc(f, gen.ModulePath)
		emitCiTeardownFunc(f)
		emitTeardownStepFunc(f)
	}

	// test targets shared by every repo that runs the generator
	emitTestUnitFunc(f)
	emitTestIntegrationFunc(f)

	// download targets shared by every repo that runs the generator, fetching
	// the threeport binaries from a github release and installing them where
	// the install targets place locally-built binaries.
	emitInstallDirFunc(f)
	emitDownloadHelper(f)
	emitDownloadFunc(f, "Sdk", "threeport-sdk")
	emitDownloadFunc(f, "Tptctl", "tptctl")

	// binary build function for API
	emitBinFunc(f, buildApiFuncName, "REST API", "rest-api", "cmd/rest-api")

	apiImageName := "threeport-rest-api"
	if gen.Module {
		apiImageName = fmt.Sprintf(
			"threeport-%s-rest-api",
			strcase.ToKebab(sdkConfig.ModuleName),
		)
	}
	apiPackageFuncName := "restApiImagePackage"
	allComponents = append(allComponents, componentSpec{
		BinaryName:       "rest-api",
		PackageDir:       "cmd/rest-api",
		ImageName:        apiImageName,
		PackageFuncName:  apiPackageFuncName,
		DockerfileTarget: "release",
	})
	emitImagePackageFunc(f, apiPackageFuncName, "REST API", "release", "rest-api", apiImageName)
	emitImageFunc(f, buildApiImageFuncName, "REST API", "rest-api", "cmd/rest-api", apiPackageFuncName, installerPkg, gen.ModulePath)

	// binary build function for database migrator
	emitBinFunc(f, buildDbMigratorFuncName, "database migrator", "database-migrator", "cmd/database-migrator")

	dbMigratorImageName := "threeport-database-migrator"
	if gen.Module {
		dbMigratorImageName = fmt.Sprintf(
			"threeport-%s-database-migrator",
			strcase.ToKebab(sdkConfig.ModuleName),
		)
	}
	dbMigratorPackageFuncName := "dbMigratorImagePackage"
	allComponents = append(allComponents, componentSpec{
		BinaryName:       "database-migrator",
		PackageDir:       "cmd/database-migrator",
		ImageName:        dbMigratorImageName,
		PackageFuncName:  dbMigratorPackageFuncName,
		DockerfileTarget: "release",
	})
	emitImagePackageFunc(f, dbMigratorPackageFuncName, "database migrator", "release", "database-migrator", dbMigratorImageName)
	emitImageFunc(f, buildDbMigratorImageFuncName, "database migrator", "database-migrator", "cmd/database-migrator", dbMigratorPackageFuncName, installerPkg, gen.ModulePath)

	if !gen.Module {
		// add function names to "build all" functions
		buildFuncNames = append(buildFuncNames, buildAgentFuncName)

		emitBinFunc(f, buildAgentFuncName, "agent", "agent", "cmd/agent")

		agentPackageFuncName := "agentImagePackage"
		allComponents = append(allComponents, componentSpec{
			BinaryName:       "agent",
			PackageDir:       "cmd/agent",
			ImageName:        "threeport-agent",
			PackageFuncName:  agentPackageFuncName,
			DockerfileTarget: "release",
		})
		emitImagePackageFunc(f, agentPackageFuncName, "agent", "release", "agent", "threeport-agent")
		emitImageFunc(f, buildAgentImageFuncName, "agent", "agent", "cmd/agent", agentPackageFuncName, installerPkg, gen.ModulePath)
	}

	// binary build functions for controllers
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			// set func names
			buildFuncName := fmt.Sprintf("%sControllerBin", objGroup.ControllerDomain)
			buildFuncNames = append(buildFuncNames, buildFuncName)

			buildImageFuncName := fmt.Sprintf("%sControllerImage", objGroup.ControllerDomain)

			// set image name
			imageName := fmt.Sprintf("threeport-%s", objGroup.ControllerName)
			if gen.Module {
				imageName = fmt.Sprintf("threeport-%s-%s", strcase.ToKebab(sdkConfig.ModuleName), objGroup.ControllerName)
			}

			packageDir := fmt.Sprintf("cmd/%s", objGroup.ControllerName)
			emitBinFunc(f, buildFuncName, objGroup.ControllerName, objGroup.ControllerName, packageDir)

			packageFuncName := fmt.Sprintf("%sControllerImagePackage", strcase.ToLowerCamel(objGroup.ControllerDomain))
			allComponents = append(allComponents, componentSpec{
				BinaryName:       objGroup.ControllerName,
				PackageDir:       packageDir,
				ImageName:        imageName,
				PackageFuncName:  packageFuncName,
				DockerfileTarget: objGroup.DockerfileTarget,
			})
			target := objGroup.DockerfileTarget
			if target == "" {
				target = "release"
			}
			emitImagePackageFunc(f, packageFuncName, objGroup.ControllerName, target, objGroup.ControllerName, imageName)
			emitImageFunc(f, buildImageFuncName, objGroup.ControllerName, objGroup.ControllerName, packageDir, packageFuncName, installerPkg, gen.ModulePath)
		}
	}
	f.Line()

	// build all binaries
	buildAllFuncName := "AllBins"
	f.Comment(fmt.Sprintf("%s builds the binaries for all components for the arch(es) in", buildAllFuncName))
	f.Comment("the ARCH env var, defaulting to the local CPU architecture.")
	f.Func().Params(Id("Build")).Id(buildAllFuncName).Params().Error().BlockFunc(func(g *Group) {
		g.Id("build").Op(":=").Id("Build").Values()
		for _, funcName := range buildFuncNames {
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
	f.Comment(fmt.Sprintf("%s builds and pushes images for all components. Repo and tag", buildAllImagesFuncName))
	f.Comment("derive from the CI context when GITHUB_ACTIONS is set, otherwise the dev")
	f.Comment("namespace and current version; the IMAGE_REPO and IMAGE_TAG env vars")
	f.Comment("override either way. Arch comes from the ARCH env var or")
	f.Comment("the local CPU architecture. Pre-compiles binaries for every requested")
	f.Comment("arch in parallel, then packages each component image in parallel. A")
	f.Comment("comma-separated ARCH value (e.g. amd64,arm64) produces a multi-arch")
	f.Comment("manifest in one push; a single arch pushes only that arch under the given")
	f.Comment("tag (use package:allManifests to stitch single-arch tags from separate")
	f.Comment("runs). Set PARALLEL_IMAGE_BUILD >= 1 to cap packaging concurrency (e.g.")
	f.Comment("`PARALLEL_IMAGE_BUILD=4 mage build:allImages`).")
	f.Func().Params(Id("Build")).Id(buildAllImagesFuncName).Params().Error().BlockFunc(func(g *Group) {
		emitPrebuildBlock(g, allComponents)

		g.List(Id("imageRepo"), Id("imageTag"), Id("err")).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageCoordinates").Call(
			Id("workingDir"),
			Qual(installerPkg, "DevImageNamespace"),
			Qual(fmt.Sprintf("%s/internal/version", gen.ModulePath), "GetVersion").Call(),
		)
		g.If(Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve image coordinates: %w"), Id("err"))),
		)
		g.Line()

		g.Id("build").Op(":=").Id("Build").Values()
		emitWrapHelper(g, Id("imageRepo"), Id("imageTag"))
		g.Id("tasks").Op(":=").Index().Func().Params().Error().ValuesFunc(func(v *Group) {
			for _, c := range allComponents {
				v.Line().Id("wrap").Call(Id("build").Dot(c.PackageFuncName))
			}
			v.Line()
		})

		g.Return().Qual("github.com/threeport/threeport/pkg/util/v0", "RunParallel").Call(
			Id("parallelFromEnv").Call(),
			Id("tasks"),
		)
	})

	// Package.Manifest stitches per-arch image tags into a multi-arch
	// manifest list under the canonical tag.
	f.Comment("Manifest stitches per-arch images for one component into a multi-arch")
	f.Comment("manifest list under the canonical tag. Repo and tag derive from the CI")
	f.Comment("context when GITHUB_ACTIONS is set, otherwise the dev namespace and")
	f.Comment("current version; IMAGE_REPO and IMAGE_TAG override either way. The arch")
	f.Comment("set is discovered from the per-arch tags already pushed to the registry,")
	f.Comment("so the stitch covers whatever single-arch images the build produced.")
	f.Comment("Sources are looked up at <repo>/<image>:<tag>-<arch> for each discovered")
	f.Comment("arch and combined into <repo>/<image>:<tag> via")
	f.Comment("`docker buildx imagetools create`.")
	f.Func().Params(Id("Package")).Id("Manifest").Params(Id("imageName").String()).Error().Block(
		Id("imageRepo").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageRepo").Call(
			Qual(installerPkg, "DevImageNamespace"),
		),
		List(Id("imageTag"), Err()).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageTag").Call(
			Lit("."),
			Qual(fmt.Sprintf("%s/internal/version", gen.ModulePath), "GetVersion").Call(),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve image tag: %w"), Err())),
		),
		Line(),

		List(Id("arches"), Err()).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "DiscoverArches").Call(
			Id("imageRepo").Op("+").Lit("/").Op("+").Id("imageName"),
			Id("imageTag"),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to discover arches: %w"), Err())),
		),
		Line(),

		Return().Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"PushMultiArchManifest",
		).Call(Id("imageRepo"), Id("imageName"), Id("imageTag"), Qual("strings", "Join").Call(Id("arches"), Lit(","))),
	)
	f.Line()

	// Package.AllManifests stitches multi-arch manifests for every
	// component image in parallel, sourced from the installer's
	// authoritative controller list so adding a new controller
	// automatically extends coverage.
	f.Comment("AllManifests stitches multi-arch manifest lists for every component")
	f.Comment("in parallel, sourced from the installer's authoritative controller")
	f.Comment("list so adding a new controller automatically extends coverage. Repo")
	f.Comment("and tag derive from the CI context when GITHUB_ACTIONS is set, otherwise")
	f.Comment("the dev namespace and current version; IMAGE_REPO and IMAGE_TAG override")
	f.Comment("either way. Each component's arch set is discovered from the per-arch")
	f.Comment("tags already pushed to the registry. Set PARALLEL_IMAGE_BUILD >= 1 to")
	f.Comment("control worker concurrency (e.g. `PARALLEL_IMAGE_BUILD=4 mage")
	f.Comment("package:allManifests`).")
	f.Func().Params(Id("Package")).Id("AllManifests").Params().Error().BlockFunc(func(g *Group) {
		g.Id("imageRepo").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageRepo").Call(
			Qual(installerPkg, "DevImageNamespace"),
		)
		g.List(Id("imageTag"), Id("err")).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageTag").Call(
			Lit("."),
			Qual(fmt.Sprintf("%s/internal/version", gen.ModulePath), "GetVersion").Call(),
		)
		g.If(Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve image tag: %w"), Id("err"))),
		)
		g.Line()
		// gather every component image. For threeport-core, source from
		// the installer's authoritative list so adding a new controller
		// extends coverage automatically. For module forks, emit the
		// per-component image names directly from the generator's
		// component slice since module installers don't share the
		// ThreeportRestApi / DatabaseMigrator / ThreeportAgent /
		// ThreeportControllerList identifiers.
		if gen.Module {
			g.Comment("gather every component image emitted by the generator")
			g.Id("images").Op(":=").Index().String().ValuesFunc(func(v *Group) {
				for _, c := range allComponents {
					v.Line().Lit(c.ImageName)
				}
				v.Line()
			})
			g.Line()
		} else {
			g.Comment("gather every component image: rest-api, db migrator, agent, and")
			g.Comment("all controllers from the installer's authoritative list")
			g.Id("images").Op(":=").Index().String().Values(
				Line().Qual(installerPkg, "ThreeportRestApi").Dot("ImageName"),
				Line().Qual(installerPkg, "DatabaseMigrator").Dot("ImageName"),
				Line().Qual(installerPkg, "ThreeportAgent").Dot("ImageName"),
				Line(),
			)
			g.For(List(Id("_"), Id("c")).Op(":=").Range().Qual(installerPkg, "ThreeportControllerList")).Block(
				Id("images").Op("=").Append(Id("images"), Id("c").Dot("ImageName")),
			)
			g.Line()
		}

		g.Id("tasks").Op(":=").Make(
			Index().Func().Params().Error(),
			Lit(0),
			Len(Id("images")),
		)
		g.For(List(Id("_"), Id("image")).Op(":=").Range().Id("images")).Block(
			Id("image").Op(":=").Id("image"),
			Id("tasks").Op("=").Append(
				Id("tasks"),
				Func().Params().Error().Block(
					List(Id("arches"), Err()).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "DiscoverArches").Call(
						Id("imageRepo").Op("+").Lit("/").Op("+").Id("image"),
						Id("imageTag"),
					),
					If(Err().Op("!=").Nil()).Block(
						Return(Qual("fmt", "Errorf").Call(Lit("failed to discover arches: %w"), Err())),
					),
					Return().Qual(
						"github.com/threeport/threeport/pkg/util/v0",
						"PushMultiArchManifest",
					).Call(Id("imageRepo"), Id("image"), Id("imageTag"), Qual("strings", "Join").Call(Id("arches"), Lit(","))),
				),
			),
		)
		g.Line()

		g.Return().Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"RunParallel",
		).Call(Id("parallelFromEnv").Call(), Id("tasks"))
	})
	f.Line()

	// helper: parse the PARALLEL_IMAGE_BUILD env var, self-compute when unset
	f.Comment("parallelFromEnv returns the PARALLEL_IMAGE_BUILD env var as an int. When")
	f.Comment("unset or empty it self-computes twice the memory-derived build worker count,")
	f.Comment("since packaging and pushing images is lighter than compiling.")
	f.Func().Id("parallelFromEnv").Params().Int().BlockFunc(func(g *Group) {
		g.Id("v").Op(":=").Qual("os", "Getenv").Call(Lit("PARALLEL_IMAGE_BUILD"))
		g.If(Id("v").Op("==").Lit("")).Block(
			Return(Qual("github.com/threeport/threeport/pkg/util/v0", "BuildParallelism").Call().Op("*").Lit(2)),
		)
		g.List(Id("n"), Err()).Op(":=").Qual("strconv", "Atoi").Call(Id("v"))
		g.If(Err().Op("!=").Nil().Op("||").Id("n").Op("<").Lit(1)).Block(
			Return(Lit(1)),
		)
		g.Return(Id("n"))
	})

	// helper: env var lookup with a fallback default
	f.Comment("envOr returns the trimmed value of the named env var, or def if it is unset or empty.")
	f.Func().Id("envOr").Params(Id("key").String(), Id("def").String()).String().Block(
		If(Id("v").Op(":=").Qual("strings", "TrimSpace").Call(Qual("os", "Getenv").Call(Id("key"))).Op(";").Id("v").Op("!=").Lit("")).Block(
			Return(Id("v")),
		),
		Return(Id("def")),
	)
	f.Line()

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

		g.If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildBinaries",
		).Call(
			Line().Id("workingDir"),
			Line().Index().String().Values(Id("arch")),
			Line().Index().String().Values(Qual("fmt", "Sprintf").Call(Lit("cmd/%s"), Id("component"))),
			Line().False(),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to build binary: %w"), Id("err"))),
		)
		g.Line()

		if gen.Module {
			g.Id("imageName").Op(":=").Qual("fmt", "Sprintf").Call(
				Lit("threeport-%s-%s"),
				Lit(strcase.ToKebab(sdkConfig.ModuleName)),
				Id("component"),
			)
		} else {
			g.Id("imageName").Op(":=").Qual("fmt", "Sprintf").Call(Lit("threeport-%s"), Id("component"))
		}
		g.Line()

		// build a map of components that need a non-default Dockerfile target.
		nonDefaultTargets := Dict{}
		for _, c := range allComponents {
			if c.DockerfileTarget != "" && c.DockerfileTarget != "release" {
				nonDefaultTargets[Lit(c.BinaryName)] = Lit(c.DockerfileTarget)
			}
		}
		if len(nonDefaultTargets) > 0 {
			g.Comment("components that require a non-standard Dockerfile target; all others use \"release\".")
			g.Id("componentTargets").Op(":=").Map(String()).String().Values(nonDefaultTargets)
			g.Id("dockerfileTarget").Op(":=").Lit("release")
			g.If(
				List(Id("t"), Id("ok")).Op(":=").Id("componentTargets").Index(Id("component")).Op(";").Id("ok"),
			).Block(
				Id("dockerfileTarget").Op("=").Id("t"),
			)
		} else {
			g.Id("dockerfileTarget").Op(":=").Lit("release")
		}
		g.Line()

		g.Comment("tag the loaded image the way an install resolves its tag, so a")
		g.Comment("later tptctl up with no --tag references the image just loaded")
		g.Comment("rather than the bare version, which names no image in the cluster.")
		g.List(Id("imageTag"), Id("err")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0", "ResolveImageTag",
		).Call(Id("workingDir"), Qual(
			fmt.Sprintf("%s/internal/version", gen.ModulePath), "GetVersion",
		).Call())
		g.If(Id("err").Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve image tag: %w"), Id("err"))),
		)
		g.Line()

		g.If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildImage",
		).Call(
			Line().Id("workingDir"),
			Line().Lit("Dockerfile"),
			Line().Id("dockerfileTarget"),
			Line().Id("arch"),
			Line().Id("component"),
			Line().Lit("bin"),
			Line().Nil(),
			Line().Qual(
				installerPkg,
				"DevImageNamespace",
			),
			Line().Id("imageName"),
			Line().Id("imageTag"),
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

	// extension plugin build and install
	if gen.Module {
		f.Comment("Plugin compiles the extension's tptctl plugin.")
		f.Func().Params(Id("Build")).Id("Plugin").Params().Error().Block(
			Id("buildCmd").Op(":=").Qual("os/exec", "Command").Call(
				Line().Lit("go"),
				Line().Lit("build"),
				Line().Lit("-o"),
				Line().Lit(fmt.Sprintf(
					"bin/%s",
					strcase.ToKebab(sdkConfig.ModuleName),
				)),
				Line().Lit(fmt.Sprintf(
					"cmd/%s/main_gen.go",
					strcase.ToSnake(sdkConfig.ModuleName),
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
				strcase.ToKebab(sdkConfig.ModuleName),
			))),
			Line(),

			Return(Nil()),
		)
		f.Line()

		f.Comment("Plugin builds the tptctl plugin and installs it in the tptctl plugin directory.")
		f.Func().Params(Id("Install")).Id("Plugin").Params().Error().Block(
			Id("build").Op(":=").Id("Build").Values(),
			If(Err().Op(":=").Id("build").Dot("Plugin").Call().Op(";").Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to build tptctl plugin: %w"),
					Err(),
				)),
			),
			Line(),

			Id("pluginDir").Op(":=").Qual("os", "Getenv").Call(Lit("THREEPORT_PLUGIN_DIR")),
			If(Id("pluginDir").Op("==").Lit("")).Block(
				List(Id("dir"), Err()).Op(":=").Qual(
					"github.com/threeport/threeport/pkg/cli/v0",
					"DefaultPluginDir",
				).Call(),
				If(Err().Op("!=").Nil()).Block(
					Return(Qual("fmt", "Errorf").Call(
						Lit("failed to determine tptctl plugin directory: %w"),
						Err(),
					)),
				),
				Id("pluginDir").Op("=").Id("dir"),
			),
			If(Err().Op(":=").Qual("os", "MkdirAll").Call(
				Id("pluginDir"),
				Id("0o755"),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to create tptctl plugin directory: %w"),
					Err(),
				)),
			),
			Line(),

			Id("outputPath").Op(":=").Qual("path/filepath", "Join").Call(
				Id("pluginDir"),
				Lit(strcase.ToKebab(sdkConfig.ModuleName)),
			),
			Id("installCmd").Op(":=").Qual("os/exec", "Command").Call(
				Line().Lit("cp"),
				Line().Lit(fmt.Sprintf(
					"bin/%s",
					strcase.ToKebab(sdkConfig.ModuleName),
				)),
				Line().Id("outputPath"),
				Line(),
			),
			Line(),

			List(Id("output"), Err()).Op(":=").Id("installCmd").Dot("CombinedOutput").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("install failed for tptctl plugin with output '%s': %w"),
					Id("output"),
					Err(),
				)),
			),
			Line(),

			Qual("fmt", "Printf").Call(
				Lit("tptctl plugin installed and available at %s\n"),
				Id("outputPath"),
			),
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
	f.Comment("getBuildVals returns the working directory and the arch(es) to build for.")
	f.Comment("Arch comes from the ARCH env var (comma-separated for multi-arch) or")
	f.Comment("defaults to the local CPU architecture.")
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

		Id("arch").Op(":=").Id("envOr").Call(Lit("ARCH"), Qual("runtime", "GOARCH")),
		Line(),

		Return(Id("workingDir"), Id("arch"), Nil()),
	)

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join("magefiles", "magefile_gen.go")
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for magefile written to %s", genFilepath))
	}

	return nil
}

// emitBinFunc writes a no-arg `func (Build) <BinFunc>() error` that compiles
// the component's binary for the arch(es) resolved by getBuildVals (the ARCH
// env var or the local CPU arch) via util.BuildBinaries. A comma-separated
// ARCH builds one binary per arch under bin/<arch>/.
func emitBinFunc(f *File, funcName, displayName, binaryName, packageDir string) {
	f.Comment(fmt.Sprintf("%s builds the %s binary for the arch(es) in the ARCH env", funcName, displayName))
	f.Comment("var, defaulting to the local CPU architecture.")
	f.Func().Params(Id("Build")).Id(funcName).Params().Error().Block(
		List(Id("workingDir"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(Lit("failed to get build values: %w"), Err()),
		),
		Line(),

		Id("arches").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ParseArches").Call(Id("arch")),
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildBinaries",
		).Call(
			Line().Id("workingDir"),
			Line().Id("arches"),
			Line().Index().String().Values(Lit(packageDir)),
			Line().False(),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return().Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf("failed to build %s binary: %%w", binaryName)),
				Err(),
			),
		),
		Line(),

		Qual("fmt", "Printf").Call(
			Lit(fmt.Sprintf("%s binary built for arch(es): %%s\n", binaryName)),
			Qual("strings", "Join").Call(Id("arches"), Lit(", ")),
		),
		Line(),

		Return().Nil(),
	)
	f.Line()
}

// emitTestUnitFunc writes a no-arg `func (Test) Unit() error` that runs the
// unit tests across the threeport packages via util.RunCommandStreamOutput.
// -race is always on, because the packages under test start goroutines the
// tests then assert against, and a data race there is invisible without the
// detector. -p is sized at runtime from util.BuildParallelism(), because the
// cgo-enabled go-sqlite3 build pulls per-package memory above the default
// GOMAXPROCS concurrency on small CI runners, so honoring the same
// memory-aware worker count the build targets use keeps `go test` from
// OOM-killing the pod. That sizing matters more with the detector on, which
// multiplies each test binary's memory several times over.
func emitTestUnitFunc(f *File) {
	f.Comment("Unit runs the unit tests across the threeport packages.")
	f.Func().Params(Id("Test")).Id("Unit").Params().Error().Block(
		Id("cmd").Op(":=").Lit("go"),
		Id("args").Op(":=").Index().String().Values(
			Line().Lit("test"),
			Line().Lit("-count=1"),
			Line().Lit("-race"),
			Line().Qual("fmt", "Sprintf").Call(
				Lit("-p=%d"),
				Qual("github.com/threeport/threeport/pkg/util/v0", "BuildParallelism").Call(),
			),
			Line().Lit("./pkg/..."),
			Line().Lit("./internal/..."),
			Line().Lit("./cmd/..."),
			Line().Lit("./magefiles/..."),
			Line(),
		),
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"RunCommandStreamOutput",
		).Call(Id("cmd"), Id("args").Op("...")).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to run unit tests: %w"), Err())),
		),
		Line(),

		Return().Nil(),
	)
	f.Line()
}

// emitTestIntegrationFunc writes a no-arg `func (Test) Integration() error`
// that runs the integration tests against an existing Threeport control plane
// via util.RunCommandStreamOutput. -p mirrors emitTestUnitFunc so the
// compile concurrency stays consistent across both test entry points; the
// integration tests share the cgo-enabled test binary build path.
func emitTestIntegrationFunc(f *File) {
	f.Comment("Integration runs integration tests against an existing Threeport control plane.")
	f.Func().Params(Id("Test")).Id("Integration").Params().Error().Block(
		Id("cmd").Op(":=").Lit("go"),
		Id("args").Op(":=").Index().String().Values(
			Line().Lit("test"),
			Line().Lit("-v"),
			Line().Qual("fmt", "Sprintf").Call(
				Lit("-p=%d"),
				Qual("github.com/threeport/threeport/pkg/util/v0", "BuildParallelism").Call(),
			),
			Line().Lit("./test/integration"),
			Line().Lit("-count=1"),
			Line(),
		),
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"RunCommandStreamOutput",
		).Call(Id("cmd"), Id("args").Op("...")).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to run integration tests: %w"), Err())),
		),
		Line(),

		Return().Nil(),
	)
	f.Line()
}

// emitDownloadFunc writes a no-arg `func (Download) <funcName>() error` that
// downloads the named threeport binary from a github release and installs it
// where the install targets place locally-built binaries. The body is thin: it
// delegates the repo, version, and destination resolution to the shared
// download helper.
func emitDownloadFunc(f *File, funcName, binary string) {
	f.Comment(fmt.Sprintf(
		"%s downloads the %s binary from a threeport github release and installs", funcName, binary,
	))
	f.Comment("it where the install targets place locally-built binaries.")
	f.Func().Params(Id("Download")).Id(funcName).Params().Error().Block(
		Return(Id("downloadThreeportBinary").Call(Lit(binary))),
	)
	f.Line()
}

// emitInstallDirFunc writes the helper that resolves where go install places
// binaries. Both the install targets and the download targets write there, and
// emitting it keeps a repo that has no hand-written magefile from generating a
// call to a function nothing defines.
func emitInstallDirFunc(f *File) {
	f.Comment("installDir returns the directory `go install` writes binaries to:")
	f.Comment("$GOBIN if set, otherwise $GOPATH/bin. build.Default.GOPATH falls back")
	f.Comment("to ~/go when $GOPATH is unset, so the result is always non-empty.")
	f.Func().Id("installDir").Params().String().Block(
		If(
			Id("gobin").Op(":=").Qual("os", "Getenv").Call(Lit("GOBIN")),
			Id("gobin").Op("!=").Lit(""),
		).Block(
			Return(Id("gobin")),
		),
		Return(Qual("path/filepath", "Join").Call(
			Qual("go/build", "Default").Dot("GOPATH"),
			Lit("bin"),
		)),
	)
	f.Line()
}

// emitDownloadHelper writes the side-effecting glue the download targets share:
// it reads go.mod to find the threeport dependency, falling back to the core
// repository's own release tags when no dependency is declared, then downloads
// the requested binary into the install directory. The pure parsing lives in
// the util package; this helper performs the file reads, git calls, and the
// network download.
func emitDownloadHelper(f *File) {
	// shared download helper: resolve (repo, version, destDir, token) then download
	f.Comment("downloadThreeportBinary downloads the named binary from a threeport github")
	f.Comment("release and installs it into the directory the install targets use. It")
	f.Comment("resolves the source release from the threeport dependency in go.mod when one")
	f.Comment("is declared (the consumer or module case), and otherwise from this")
	f.Comment("repository's own highest release tag under the version file's base (the core")
	f.Comment("threeport case). The GITHUB_TOKEN env var authenticates the download and may")
	f.Comment("be empty for public releases.")
	f.Func().Id("downloadThreeportBinary").Params(Id("binary").String()).Error().Block(
		List(Id("gomod"), Err()).Op(":=").Qual("os", "ReadFile").Call(Lit("go.mod")),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to read go.mod: %w"), Err())),
		),
		Line(),

		List(Id("repo"), Id("version"), Id("found"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0", "ParseThreeportDependency",
		).Call(String().Call(Id("gomod"))),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to parse threeport dependency: %w"), Err())),
		),
		Line(),

		Comment("no threeport dependency means this is the core threeport repo; derive"),
		Comment("the repo and release tag from the origin remote and the version file."),
		If(Op("!").Id("found")).Block(
			List(Id("repo"), Id("version"), Err()).Op("=").Id("coreThreeportRelease").Call(),
			If(Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve core threeport release: %w"), Err())),
			),
		),
		Line(),

		Id("destDir").Op(":=").Id("installDir").Call(),
		Id("token").Op(":=").Qual("os", "Getenv").Call(Lit("GITHUB_TOKEN")),
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0", "DownloadReleaseBinary",
		).Call(
			Line().Id("repo"),
			Line().Id("version"),
			Line().Id("binary"),
			Line().Id("destDir"),
			Line().Id("token"),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to download %s: %w"), Id("binary"), Err())),
		),
		Line(),

		Qual("fmt", "Printf").Call(
			Lit("%s downloaded from %s release %s and installed at %s\n"),
			Id("binary"),
			Id("repo"),
			Id("version"),
			Qual("path/filepath", "Join").Call(Id("destDir"), Id("binary")),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()

	// core-repo resolver: version file base + highest matching remote tag + origin repo
	f.Comment("coreThreeportRelease resolves the release the core threeport repository should")
	f.Comment("download its own binaries from: the highest existing release tag matching the")
	f.Comment("version file's base, paired with the origin repository as an owner/name path.")
	f.Func().Id("coreThreeportRelease").Params().Params(
		Id("repo").String(),
		Id("version").String(),
		Err().Error(),
	).Block(
		List(Id("baseBytes"), Err()).Op(":=").Qual("os", "ReadFile").Call(Lit("internal/version/version.txt")),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(""), Lit(""), Qual("fmt", "Errorf").Call(Lit("failed to read version file: %w"), Err())),
		),
		Id("base").Op(":=").Qual("strings", "TrimSpace").Call(String().Call(Id("baseBytes"))),
		Line(),

		List(Id("out"), Err()).Op(":=").Qual("os/exec", "Command").Call(
			Lit("git"), Lit("ls-remote"), Lit("--tags"), Lit("origin"),
		).Dot("CombinedOutput").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(""), Lit(""), Qual("fmt", "Errorf").Call(
				Lit("failed to list remote tags with output '%s': %w"), Id("out"), Err(),
			)),
		),
		Line(),

		List(Id("tag"), Id("ok")).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0", "LatestMatchingTag",
		).Call(Id("parseLsRemoteTags").Call(String().Call(Id("out"))), Id("base")),
		If(Op("!").Id("ok")).Block(
			Return(Lit(""), Lit(""), Qual("fmt", "Errorf").Call(
				Lit("failed to find a release tag matching %s.N"), Id("base"),
			)),
		),
		Line(),

		List(Id("repo"), Err()).Op("=").Id("originRepo").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(""), Lit(""), Err()),
		),
		Line(),

		Return(Id("repo"), Id("tag"), Nil()),
	)
	f.Line()

	// parse `git ls-remote --tags` output into bare tag names
	f.Comment("parseLsRemoteTags extracts bare tag names from `git ls-remote --tags` output,")
	f.Comment("dropping the refs/tags/ prefix and the ^{} dereference lines so each annotated")
	f.Comment("tag is counted once.")
	f.Func().Id("parseLsRemoteTags").Params(Id("out").String()).Index().String().Block(
		Id("tags").Op(":=").Index().String().Values(),
		For(List(Id("_"), Id("line")).Op(":=").Range().Qual("strings", "Split").Call(Id("out"), Lit("\n"))).Block(
			Id("fields").Op(":=").Qual("strings", "Fields").Call(Id("line")),
			If(Len(Id("fields")).Op("<").Lit(2)).Block(Continue()),
			Id("ref").Op(":=").Id("fields").Index(Lit(1)),
			Comment("skip the dereferenced peeled-tag lines so annotated tags count once"),
			If(Qual("strings", "HasSuffix").Call(Id("ref"), Lit("^{}"))).Block(Continue()),
			Id("tags").Op("=").Append(Id("tags"), Qual("strings", "TrimPrefix").Call(Id("ref"), Lit("refs/tags/"))),
		),
		Return(Id("tags")),
	)
	f.Line()

	// origin repo as an owner/name path from $GITHUB_REPOSITORY or the remote url
	f.Comment("originRepo returns the current repository as an owner/name path, preferring")
	f.Comment("the GITHUB_REPOSITORY env var when set and otherwise parsing the origin")
	f.Comment("remote url. Both https and ssh remote forms are accepted.")
	f.Func().Id("originRepo").Params().Params(String(), Error()).Block(
		If(Id("repo").Op(":=").Qual("strings", "TrimSpace").Call(
			Qual("os", "Getenv").Call(Lit("GITHUB_REPOSITORY")),
		).Op(";").Id("repo").Op("!=").Lit("")).Block(
			Return(Id("repo"), Nil()),
		),
		Line(),

		List(Id("out"), Err()).Op(":=").Qual("os/exec", "Command").Call(
			Lit("git"), Lit("remote"), Lit("get-url"), Lit("origin"),
		).Dot("CombinedOutput").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Lit(""), Qual("fmt", "Errorf").Call(
				Lit("failed to read origin remote url with output '%s': %w"), Id("out"), Err(),
			)),
		),
		Line(),

		Id("repo").Op(":=").Id("parseOriginRepo").Call(Qual("strings", "TrimSpace").Call(String().Call(Id("out")))),
		If(Id("repo").Op("==").Lit("")).Block(
			Return(Lit(""), Qual("fmt", "Errorf").Call(
				Lit("failed to parse owner/name from origin remote url %q"),
				Qual("strings", "TrimSpace").Call(String().Call(Id("out"))),
			)),
		),
		Line(),

		Return(Id("repo"), Nil()),
	)
	f.Line()

	// reduce a git remote url to an owner/name path
	f.Comment("parseOriginRepo reduces a git remote url to an owner/name path, accepting the")
	f.Comment("https form (https://github.com/owner/name.git) and the ssh form")
	f.Comment("(git@github.com:owner/name.git). It returns an empty string when neither")
	f.Comment("shape yields an owner and name.")
	f.Func().Id("parseOriginRepo").Params(Id("url").String()).String().Block(
		Id("url").Op("=").Qual("strings", "TrimSuffix").Call(Id("url"), Lit(".git")),
		Comment("split on / and take the trailing two segments as owner/name"),
		Id("parts").Op(":=").Qual("strings", "Split").Call(Id("url"), Lit("/")),
		If(Len(Id("parts")).Op("<").Lit(2)).Block(
			Comment("an ssh url with no slash host separator: split on the colon instead"),
			Id("colonParts").Op(":=").Qual("strings", "SplitN").Call(Id("url"), Lit(":"), Lit(2)),
			If(Len(Id("colonParts")).Op("==").Lit(2)).Block(
				Id("parts").Op("=").Qual("strings", "Split").Call(Id("colonParts").Index(Lit(1)), Lit("/")),
			),
		),
		If(Len(Id("parts")).Op("<").Lit(2)).Block(
			Return(Lit("")),
		),
		Id("owner").Op(":=").Id("parts").Index(Len(Id("parts")).Op("-").Lit(2)),
		Id("name").Op(":=").Id("parts").Index(Len(Id("parts")).Op("-").Lit(1)),
		Comment("an ssh owner may still carry the host:owner prefix; keep the trailing owner"),
		If(Id("i").Op(":=").Qual("strings", "LastIndex").Call(Id("owner"), Lit(":")).Op(";").Id("i").Op(">=").Lit(0)).Block(
			Id("owner").Op("=").Id("owner").Index(Id("i").Op("+").Lit(1), Empty()),
		),
		If(Id("owner").Op("==").Lit("").Op("||").Id("name").Op("==").Lit("")).Block(
			Return(Lit("")),
		),
		Return(Id("owner").Op("+").Lit("/").Op("+").Id("name")),
	)
	f.Line()
}

// emitImageFunc writes a no-arg `func (Build) <ImageFunc>() error` that
// compiles the binary for the resolved arch(es) via BuildBinaries, then
// delegates packaging to the per-component package function. Repo and tag
// derive from the CI context when GITHUB_ACTIONS is set, otherwise the dev
// namespace and current version; IMAGE_REPO and IMAGE_TAG override either way.
// Arch comes from the ARCH env var or the local CPU arch. When
// called from AllImages the BuildBinaries call is a Go cache hit (AllImages
// pre-compiled the same package earlier); standalone it does the compile.
func emitImageFunc(f *File, funcName, displayName, binaryName, packageDir, packageFuncName, installerPkg, modulePath string) {
	f.Comment(fmt.Sprintf("%s builds and pushes a %s container image.", funcName, displayName))
	f.Func().Params(Id("Build")).Id(funcName).Params().Parens(Error()).Block(
		List(Id("workingDir"), Id("arch"), Err()).Op(":=").Id("getBuildVals").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get build values: %w"), Err())),
		),
		Line(),

		List(Id("imageRepo"), Id("imageTag"), Err()).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveImageCoordinates").Call(
			Id("workingDir"),
			Qual(installerPkg, "DevImageNamespace"),
			Qual(fmt.Sprintf("%s/internal/version", modulePath), "GetVersion").Call(),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve image coordinates: %w"), Err())),
		),
		Line(),

		Id("arches").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ParseArches").Call(Id("arch")),
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildBinaries",
		).Call(
			Line().Id("workingDir"),
			Line().Id("arches"),
			Line().Index().String().Values(Lit(packageDir)),
			Line().False(),
			Line(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf("failed to build %s binary: %%w", binaryName)),
				Err(),
			)),
		),
		Line(),

		Return(Id("Build").Values().Dot(packageFuncName).Call(Id("workingDir"), Id("imageRepo"), Id("imageTag"), Id("arch"))),
	)
	f.Line()
}

// emitImagePackageFunc writes a private `(Build).<packageFuncName>` method
// that takes a pre-built binary at bin/<arch>/<binaryName> and packages
// it into a container image via util.BuildImage. AllImages* call this
// directly after the upfront BuildBinaries to skip the redundant per-
// component compile that the public <ImageFunc> wrapper does.
func emitImagePackageFunc(f *File, packageFuncName, displayName, target, binaryName, imageName string) {
	f.Comment(fmt.Sprintf("%s packages a pre-built %s binary into a container image.", packageFuncName, displayName))
	f.Func().Params(Id("Build")).Id(packageFuncName).Params(
		Line().Id("workingDir").String(),
		Line().Id("imageRepo").String(),
		Line().Id("imageTag").String(),
		Line().Id("arch").String(),
		Line(),
	).Parens(Error()).Block(
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0",
			"BuildImage",
		).Call(
			Line().Id("workingDir"),
			Line().Lit("Dockerfile"),
			Line().Lit(target),
			Line().Id("arch"),
			Line().Lit(binaryName),
			Line().Lit("bin"),
			Line().Nil(),
			Line().Id("imageRepo"),
			Line().Lit(imageName),
			Line().Id("imageTag"),
			Line().True(),
			Line().False(),
			Line().Lit(""),
			Line(),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit(fmt.Sprintf(
				"failed to build and push %s image: %%w", binaryName,
			)), Err())),
		),
		Line(),

		Return(Nil()),
	)
	f.Line()
}

// emitPrebuildBlock writes the upfront BuildBinaries call used by AllImages.
// Declares workingDir and arch (from getBuildVals: the ARCH env var or the
// local CPU arch) in the caller's scope so the wrap helper can reference arch.
func emitPrebuildBlock(g *Group, components []componentSpec) {
	g.List(Id("workingDir"), Id("arch"), Id("err")).Op(":=").Id("getBuildVals").Call()
	g.If(Id("err").Op("!=").Nil()).Block(
		Return(Qual("fmt", "Errorf").Call(Lit("failed to get build values: %w"), Id("err"))),
	)
	g.Line()

	g.Comment("pre-compile every binary for every requested arch in one go build")
	g.Comment("per arch (arches run in parallel) so dependency compilation is")
	g.Comment("shared across components within an arch. Each per-image task")
	g.Comment("below then only packages the pre-built binary.")
	g.Id("arches").Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ParseArches").Call(Id("arch"))
	g.Line()

	g.Id("packageDirs").Op(":=").Index().String().ValuesFunc(func(v *Group) {
		for _, c := range components {
			v.Line().Lit(c.PackageDir)
		}
		v.Line()
	})
	g.Line()

	g.If(Err().Op(":=").Qual(
		"github.com/threeport/threeport/pkg/util/v0",
		"BuildBinaries",
	).Call(
		Line().Id("workingDir"),
		Line().Id("arches"),
		Line().Id("packageDirs"),
		Line().False(),
		Line(),
	).Op(";").Err().Op("!=").Nil()).Block(
		Return(Qual("fmt", "Errorf").Call(Lit("failed to pre-build binaries: %w"), Err())),
	)
	g.Line()
}

// emitWrapHelper writes a `wrap` closure into the AllImages* function
// bodies. It captures workingDir + repo + tag + arch from the surrounding
// scope and adapts each per-component package method (which has a
// uniform 4-string signature) into the func() error shape RunParallel
// expects, so the task list reads as `wrap(build.fooImagePackage)` per
// component instead of an inline closure per entry.
func emitWrapHelper(g *Group, repo, tag Code) {
	g.Id("wrap").Op(":=").Func().Params(
		Id("fn").Func().Params(String(), String(), String(), String()).Error(),
	).Func().Params().Error().Block(
		Return().Func().Params().Error().Block(
			Return().Id("fn").Call(Id("workingDir"), repo, tag, Id("arch")),
		),
	)
}

// emitCiEnvFunc writes a no-arg `func (Ci) Env() error` that prints the
// KEY=value lines the non-mage CI steps consume. The modulePath qualifies the
// embedded version package the module image tag derives from.
func emitCiEnvFunc(f *File, modulePath string) {
	f.Comment("Env prints KEY=value lines for the workflow to append to GITHUB_ENV. It emits")
	f.Comment("only the values that non-mage steps consume: the pinned threeport repo,")
	f.Comment("version, and ghcr namespace the gh release download and tptctl up steps read;")
	f.Comment("the module's own image tag the module install step reads; GOFLAGS, the")
	f.Comment("memory-derived go-build worker count the non-mage steps inherit; and")
	f.Comment("GORELEASER_PARALLELISM, a quarter of that worker count, the number of")
	f.Comment("whole-tree targets goreleaser builds at once since each links the full tree.")
	f.Comment("Values mage itself consumes (image repo, build-time image tag, image-build")
	f.Comment("parallelism) are self-derived at use and are not emitted here.")
	f.Func().Params(Id("Ci")).Id("Env").Params().Error().Block(
		List(Id("repo"), Id("namespace"), Id("ver"), Err()).Op(":=").Qual("github.com/threeport/threeport/pkg/util/v0", "ResolveThreeportPin").Call(),
		If(Err().Op("!=").Nil()).Block(
			Return(Err()),
		),
		Line(),

		List(Id("moduleTag"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/util/v0", "ResolveImageTag",
		).Call(Lit("."), Qual(fmt.Sprintf("%s/internal/version", modulePath), "GetVersion").Call()),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to resolve module image tag: %w"), Err())),
		),
		Line(),

		Qual("fmt", "Printf").Call(Lit("THREEPORT_REPO=%s\n"), Id("repo")),
		Qual("fmt", "Printf").Call(Lit("THREEPORT_IMAGE_TAG=%s\n"), Id("ver")),
		Qual("fmt", "Printf").Call(Lit("THREEPORT_IMAGE_NAMESPACE=%s\n"), Id("namespace")),
		Qual("fmt", "Printf").Call(Lit("MODULE_IMAGE_TAG=%s\n"), Id("moduleTag")),
		Qual("fmt", "Printf").Call(
			Lit("GOFLAGS=-p=%d\n"),
			Qual("github.com/threeport/threeport/pkg/util/v0", "BuildParallelism").Call(),
		),
		Qual("fmt", "Printf").Call(
			Lit("GORELEASER_PARALLELISM=%d\n"),
			Qual("github.com/threeport/threeport/pkg/util/v0", "ReleaseParallelism").Call(),
		),
		Return(Nil()),
	)
	f.Line()
}

// emitCiTeardownFunc writes a no-arg `func (Ci) Teardown() error` that removes
// what an integration job leaves behind. The body calls the generated Dev local
// registry target, which lives in the same generated file.
func emitCiTeardownFunc(f *File) {
	f.Comment("Teardown removes what an integration job leaves behind: the test control")
	f.Comment("plane and its kind cluster, the threeport client config, the local image")
	f.Comment("registry, and dangling docker data. It force-removes the cluster and config")
	f.Comment("directly rather than trusting tptctl down, which cannot clear its")
	f.Comment("control-plane entry once a failed test has left the cluster gone. Running")
	f.Comment("unconditionally keeps every subsequent run starting clean. Gated on the CI")
	f.Comment("env var so it never runs against a local environment.")
	f.Func().Params(Id("Ci")).Id("Teardown").Params().Error().Block(
		If(Qual("os", "Getenv").Call(Lit("CI")).Op("!=").Lit("true")).Block(
			Qual("fmt", "Println").Call(Lit("ci:teardown: not running in CI, skipping")),
			Return(Nil()),
		),
		Comment("best-effort graceful teardown of the control plane, then force-delete the"),
		Comment("kind cluster in case the graceful path failed"),
		Id("teardownStep").Call(Lit("tptctl"), Lit("down"), Lit("--name"), Lit("test")),
		Id("teardownStep").Call(Lit("kind"), Lit("delete"), Lit("cluster"), Lit("--name"), Lit("threeport-test")),
		Comment("force-remove the threeport client config; a failed run leaves tptctl down"),
		Comment("unable to clear its control-plane entry, which would block the next"),
		Comment("bring-up"),
		If(List(Id("home"), Err()).Op(":=").Qual("os", "UserHomeDir").Call().Op(";").Err().Op("==").Nil()).Block(
			Id("teardownStep").Call(
				Lit("rm"),
				Lit("-f"),
				Qual("path/filepath", "Join").Call(Id("home"), Lit(".threeport"), Lit("config.yaml")),
			),
		),
		Comment("remove the local image registry"),
		If(Err().Op(":=").Parens(Id("Dev").Values()).Dot("LocalRegistryDown").Call().Op(";").Err().Op("!=").Nil()).Block(
			Qual("fmt", "Fprintf").Call(Qual("os", "Stderr"), Lit("ci:teardown: remove local registry: %v\n"), Err()),
		),
		Comment("reclaim dangling images, stopped containers, and build cache"),
		Id("teardownStep").Call(Lit("docker"), Lit("system"), Lit("prune"), Lit("-f")),
		Return(Nil()),
	)
	f.Line()
}

// emitTeardownStepFunc writes the variadic `teardownStep` helper that runs a
// cleanup command best-effort, logging on failure so one failed command does
// not abort the rest of teardown.
func emitTeardownStepFunc(f *File) {
	f.Comment("teardownStep runs a cleanup command best-effort, logging on failure so one")
	f.Comment("failed command does not abort the rest of teardown.")
	f.Func().Id("teardownStep").Params(
		Id("name").String(),
		Id("args").Op("...").String(),
	).Block(
		If(
			List(Id("out"), Err()).Op(":=").Qual("os/exec", "Command").Call(
				Id("name"), Id("args").Op("..."),
			).Dot("CombinedOutput").Call().Op(";").Err().Op("!=").Nil(),
		).Block(
			Qual("fmt", "Fprintf").Call(
				Qual("os", "Stderr"),
				Lit("ci:teardown: %s %v failed: %v (%s)\n"),
				Id("name"),
				Id("args"),
				Err(),
				Id("out"),
			),
		),
	)
	f.Line()
}
