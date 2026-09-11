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

	// alias the version package the module CI targets read
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

	namespaces := []string{"Build", "Test", "Install", "Dev", "Package"}
	for _, ns := range namespaces {
		f.Comment(fmt.Sprintf(
			"%s provides a type for methods that implement %s targets.", ns, strcase.ToLowerCamel(ns),
		))
		f.Type().Id(ns).Qual("github.com/magefile/mage/mg", "Namespace")
		f.Line()
	}

	f.Comment("Ci provides a type for methods that emit values for CI workflow steps.")
	f.Type().Id("Ci").Qual("github.com/magefile/mage/mg", "Namespace")
	f.Line()
	moduleVersion := ""
	if gen.Module {
		moduleVersion = fmt.Sprintf("%s/internal/version", gen.ModulePath)
	}
	emitCiEnvFunc(f, moduleVersion)
	emitCiTeardownFunc(f)

	// emit test targets
	emitTestUnitFunc(f)
	emitTestIntegrationFunc(f)

	emitInstallDirFunc(f)

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

	// parse PARALLEL_IMAGE_BUILD, self-compute when unset
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

	// look up an env var with a fallback default
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

// emitBinFunc writes a (Build).<funcName> method that compiles the
// named binary for ARCH, defaulting to the local CPU architecture.
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

// emitTestUnitFunc writes Test.Unit, which runs go test -race over
// pkg, internal, cmd, and magefiles.
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

// emitTestIntegrationFunc writes Test.Integration, which runs the
// integration tests under test/integration.
func emitTestIntegrationFunc(f *File) {
	f.Comment("Integration runs integration tests against an existing Threeport control plane.")
	f.Func().Params(Id("Test")).Id("Integration").Params().Error().Block(
		If(Err().Op(":=").Qual(
			"github.com/threeport/threeport/pkg/cli/v0", "UnmetPrerequisites",
		).Call(
			Lit("integration test"),
			Qual("github.com/threeport/threeport/pkg/cli/v0", "ControlPlaneConfigProblems").Call(),
		).Op(";").Err().Op("!=").Nil()).Block(
			Return(Err()),
		),
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

// emitInstallDirFunc writes installDir so generated install targets
// have a destination function to call.
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

// emitImageFunc writes a (Build).<funcName> method that compiles the
// named binary then packages it via the matching package method.
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

// emitPrebuildBlock writes AllImages' upfront compile and declares
// workingDir and arch in the caller so wrap can pass them through.
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

// emitCiEnvFunc writes Ci.Env as a call to util.WriteCIEnv.
func emitCiEnvFunc(f *File, versionPkg string) {
	f.Comment("Env prints KEY=value lines for a workflow GITHUB_ENV file.")
	f.Func().Params(Id("Ci")).Id("Env").Params().Error().Block(
		func() Code {
			if versionPkg == "" {
				return Return(Qual("github.com/threeport/threeport/pkg/util/v0", "WriteCIEnv").Call(Lit("")))
			}
			return Return(Qual("github.com/threeport/threeport/pkg/util/v0", "WriteCIEnv").Call(
				Qual(versionPkg, "GetVersion").Call(),
			))
		}(),
	)
	f.Line()
}

// emitCiTeardownFunc writes Ci.Teardown as a call to util.TeardownCILeftovers.
func emitCiTeardownFunc(f *File) {
	f.Comment("Teardown removes leftover kind clusters, containers, networks, and volumes.")
	f.Func().Params(Id("Ci")).Id("Teardown").Params().Error().Block(
		Return(Qual("github.com/threeport/threeport/pkg/util/v0", "TeardownCILeftovers").Call(
			Lit("./bin/tptctl"),
			Lit("mage-test"),
			Func().Params().Error().Block(
				Return(Parens(Id("Dev").Values()).Dot("LocalRegistryDown").Call()),
			),
		)),
	)
	f.Line()
}
