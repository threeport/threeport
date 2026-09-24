package apiserver

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

// GenObjectLookup emits server-side helpers that resolve names to IDs and
// back for every core API type with a Name field. Used by the events-join
// handler to enrich responses without a per-type switch in the CLI.
// Generated only for threeport/threeport (not for modules); module dispatch
// is handled at runtime by GetModuleRouteForType().
func GenObjectLookup(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	for _, collection := range generator.VersionedApiObjectCollections {
		// collect types with a Name field for this version
		var namedTypes []string
		for _, group := range collection.VersionedApiObjectGroups {
			for _, obj := range group.ApiObjects {
				if obj.NameField && obj.TypeName != "" {
					namedTypes = append(namedTypes, obj.TypeName)
				}
			}
		}
		if len(namedTypes) == 0 {
			continue
		}
		slices.Sort(namedTypes)

		f := NewFile("v0")
		f.HeaderComment(sdk.HeaderCommentGenNoEdit)

		apiPkg := fmt.Sprintf("github.com/threeport/threeport/pkg/api/%s", collection.Version)

		// ErrUnknownCoreType sentinel
		f.Comment("ErrUnknownCoreType signals that the given object type is not")
		f.Comment("owned by core. Callers should dispatch to the owning module.")
		f.Var().Id("ErrUnknownCoreType").Op("=").Qual("errors", "New").Call(
			Lit("object type not owned by core"),
		)
		f.Line()

		// GetCoreObjectNamesByIDs
		f.Comment("GetCoreObjectNamesByIDs returns id->name for each id of the given")
		f.Comment("core object type, or ErrUnknownCoreType if the type isn't a core type.")
		f.Comment("includeDeleted=true bypasses the soft-delete filter to include removed rows.")
		f.Func().Id("GetCoreObjectNamesByIDs").Params(
			Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
			Id("objectType").String(),
			Id("ids").Index().Uint(),
			Id("includeDeleted").Bool(),
		).Params(
			Map(Uint()).String(),
			Error(),
		).Block(
			If(Len(Id("ids")).Op("==").Lit(0)).Block(
				Return(Map(Uint()).String().Values(), Nil()),
			),
			If(Id("includeDeleted")).Block(
				Id("db").Op("=").Id("db").Dot("Unscoped").Call(),
			),
			Id("out").Op(":=").Make(Map(Uint()).String(), Len(Id("ids"))),
			Switch(Id("objectType")).BlockFunc(func(g *Group) {
				for _, name := range namedTypes {
					g.Case(Lit(fmt.Sprintf("threeport.io/%s.%s", collection.Version, name))).Block(
						Var().Id("rows").Index().Qual(apiPkg, name),
						If(
							Id("err").Op(":=").Id("db").
								Dot("Model").Call(Op("&").Qual(apiPkg, name).Values()).
								Dot("Select").Call(Lit("id, name")).
								Dot("Where").Call(Lit("id IN ?"), Id("ids")).
								Dot("Find").Call(Op("&").Id("rows")).
								Dot("Error"),
							Id("err").Op("!=").Nil(),
						).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to look up %s names: %%w", name)),
								Id("err"),
							)),
						),
						For(List(Id("_"), Id("r")).Op(":=").Range().Id("rows")).Block(
							If(Id("r").Dot("ID").Op("!=").Nil().Op("&&").Id("r").Dot("Name").Op("!=").Nil()).Block(
								Id("out").Index(Op("*").Id("r").Dot("ID")).Op("=").Op("*").Id("r").Dot("Name"),
							),
						),
					)
					g.Line()
				}
				g.Default().Block(
					Return(Nil(), Id("ErrUnknownCoreType")),
				)
			}),
			Return(Id("out"), Nil()),
		)
		f.Line()

		// GetCoreObjectIDsByName
		f.Comment("GetCoreObjectIDsByName returns every ID with the given name for")
		f.Comment("the given core object type, or ErrUnknownCoreType if the type")
		f.Comment("isn't a core type. Empty result is returned as an empty slice")
		f.Comment("with no error.")
		f.Func().Id("GetCoreObjectIDsByName").Params(
			Id("db").Op("*").Qual("gorm.io/gorm", "DB"),
			Id("objectType").String(),
			Id("name").String(),
		).Params(
			Index().Uint(),
			Error(),
		).Block(
			Switch(Id("objectType")).BlockFunc(func(g *Group) {
				for _, name := range namedTypes {
					g.Case(Lit(fmt.Sprintf("threeport.io/%s.%s", collection.Version, name))).Block(
						Var().Id("rows").Index().Qual(apiPkg, name),
						If(
							Id("err").Op(":=").Id("db").
								Dot("Select").Call(Lit("id")).
								Dot("Where").Call(Lit("name = ?"), Id("name")).
								Dot("Find").Call(Op("&").Id("rows")).
								Dot("Error"),
							Id("err").Op("!=").Nil(),
						).Block(
							Return(Nil(), Qual("fmt", "Errorf").Call(
								Lit(fmt.Sprintf("failed to look up %s by name: %%w", name)),
								Id("err"),
							)),
						),
						Id("ids").Op(":=").Make(Index().Uint(), Lit(0), Len(Id("rows"))),
						For(List(Id("_"), Id("r")).Op(":=").Range().Id("rows")).Block(
							If(Id("r").Dot("ID").Op("!=").Nil()).Block(
								Id("ids").Op("=").Append(Id("ids"), Op("*").Id("r").Dot("ID")),
							),
						),
						Return(Id("ids"), Nil()),
					)
					g.Line()
				}
				g.Default().Block(
					Return(Nil(), Id("ErrUnknownCoreType")),
				)
			}),
		)
		f.Line()

		// write code to file if not excluded by SDK config.
		// lives in lib/v0 so it can be reused outside the handlers package
		// and so the broader utility functions that wrap it can also be
		// kept out of handlers/.
		genFilepath := filepath.Join(
			"pkg",
			"api-server",
			"lib",
			collection.Version,
			"object_lookup_gen.go",
		)
		if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
			cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
			continue
		}
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for object lookup helpers written to %s", genFilepath))
	}

	return nil
}
