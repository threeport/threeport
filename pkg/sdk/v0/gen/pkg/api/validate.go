package api

import (
	"fmt"
	"path/filepath"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenValidationHooks emits per-group GORM hook boilerplate and scaffolds a
// companion file of empty Validate* stubs for hand-written validation.
func GenValidationHooks(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	for _, objCollection := range generator.VersionedApiObjectCollections {
		for _, objGroup := range objCollection.VersionedApiObjectGroups {
			if err := emitValidateGen(generator, objCollection.Version, objGroup); err != nil {
				return fmt.Errorf("failed to emit %s_validate_gen.go: %w", objGroup.Name, err)
			}
			if err := emitValidateScaffoldIfMissing(generator, objCollection.Version, objGroup); err != nil {
				return fmt.Errorf("failed to scaffold %s_validate.go: %w", objGroup.Name, err)
			}
		}
	}
	return nil
}

// emitValidateGen writes the boilerplate <group>_validate_gen.go file.
func emitValidateGen(
	generator *gen.Generator,
	version string,
	objGroup gen.VersionedApiObjectGroup,
) error {
	f := NewFile(version)
	f.HeaderComment(util.HeaderCommentGenNoEdit)
	f.ImportAlias("gorm.io/gorm", "gorm")
	if generator.Module {
		f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpapi_v0")
	}

	for _, apiObj := range objGroup.ApiObjects {
		typeName := apiObj.TypeName
		receiver := strings.ToLower(string(typeName[0]))

		// resolve helper call: same-package for core, cross-package for modules
		processCall := func(funcName string) *Statement {
			if generator.Module {
				return Qual(
					"github.com/threeport/threeport/pkg/api/v0",
					funcName,
				).Call(Id("tx"), Id(receiver))
			}
			return Id(funcName).Call(Id("tx"), Id(receiver))
		}

		// BeforeCreate
		f.Comment(fmt.Sprintf(
			"BeforeCreate is the GORM create hook for %s.",
			typeName,
		))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("BeforeCreate").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			If(
				Err().Op(":=").Id(receiver).Dot("validateBeforeCreate").Call(Id("tx")),
				Err().Op("!=").Nil(),
			).Block(
				Return(Err()),
			),
			Return(processCall("ProcessCoreTaggedFieldsBeforeCreate")),
		)
		f.Line()

		// BeforeUpdate
		f.Comment(fmt.Sprintf(
			"BeforeUpdate is the GORM update hook for %s.",
			typeName,
		))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("BeforeUpdate").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			If(
				Err().Op(":=").Id(receiver).Dot("validateBeforeUpdate").Call(Id("tx")),
				Err().Op("!=").Nil(),
			).Block(
				Return(Err()),
			),
			Return(processCall("ProcessCoreTaggedFieldsBeforeUpdate")),
		)
		f.Line()

		// BeforeDelete
		f.Comment(fmt.Sprintf(
			"BeforeDelete is the GORM delete hook for %s.",
			typeName,
		))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("BeforeDelete").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			If(
				Err().Op(":=").Id(receiver).Dot("validateBeforeDelete").Call(Id("tx")),
				Err().Op("!=").Nil(),
			).Block(
				Return(Err()),
			),
			Return(processCall("ProcessCoreTaggedFieldsBeforeDelete")),
		)
		f.Line()
	}

	outputPath := filepath.Join(
		"pkg", "api", version,
		fmt.Sprintf("%s_validate_gen.go", strcase.ToSnake(objGroup.Name)),
	)
	if _, err := util.WriteCodeToFile(f, outputPath, true); err != nil {
		return err
	}
	cli.Info(fmt.Sprintf("source code for API object validation hooks written to %s", outputPath))
	return nil
}

// emitValidateScaffoldIfMissing writes a one-time scaffolded
// <group>_validate.go of empty Validate* stubs if no such file exists.
func emitValidateScaffoldIfMissing(
	generator *gen.Generator,
	version string,
	objGroup gen.VersionedApiObjectGroup,
) error {
	outputPath := filepath.Join(
		"pkg", "api", version,
		fmt.Sprintf("%s_validate.go", strcase.ToSnake(objGroup.Name)),
	)

	f := NewFile(version)
	f.HeaderComment(util.HeaderCommentGenMod)
	f.ImportAlias("gorm.io/gorm", "gorm")

	for _, apiObj := range objGroup.ApiObjects {
		typeName := apiObj.TypeName
		receiver := strings.ToLower(string(typeName[0]))

		// validateBeforeCreate
		f.Comment(fmt.Sprintf("validateBeforeCreate validates the %s before create.", typeName))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("validateBeforeCreate").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			Return(Nil()),
		)
		f.Line()

		// validateBeforeUpdate
		f.Comment(fmt.Sprintf("validateBeforeUpdate validates the %s before update.", typeName))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("validateBeforeUpdate").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			Return(Nil()),
		)
		f.Line()

		// validateBeforeDelete
		f.Comment(fmt.Sprintf("validateBeforeDelete validates the %s before delete.", typeName))
		f.Func().Params(
			Id(receiver).Op("*").Id(typeName),
		).Id("validateBeforeDelete").Params(
			Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
		).Error().Block(
			Return(Nil()),
		)
		f.Line()
	}

	written, err := util.WriteCodeToFile(f, outputPath, false)
	if err != nil {
		return err
	}
	if written {
		cli.Info(fmt.Sprintf("source code for API object custom validation written to %s", outputPath))
	} else {
		cli.Info(fmt.Sprintf(
			"source code for API object custom validation already exists at %s - not overwritten",
			outputPath,
		))
	}
	return nil
}
