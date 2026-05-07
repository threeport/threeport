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

// hookSpec describes one GORM lifecycle hook the generator emits and its
// matching user-facing extension method and core dispatcher.
type hookSpec struct {
	gormName string // exported: BeforeCreate, AfterCreate, ...
	userName string // unexported: beforeCreate, afterCreate, ...
	coreName string // ProcessCoreTaggedFieldsBeforeCreate, ...
	phase    string // before-create, after-create, ...
	verb     string // created, updated, deleted
}

// hooks is the canonical ordered list of GORM lifecycle hooks the generator
// emits for every API type.
var hooks = []hookSpec{
	{"BeforeCreate", "beforeCreate", "ProcessCoreTaggedFieldsBeforeCreate", "before-create", "created"},
	{"BeforeUpdate", "beforeUpdate", "ProcessCoreTaggedFieldsBeforeUpdate", "before-update", "updated"},
	{"BeforeDelete", "beforeDelete", "ProcessCoreTaggedFieldsBeforeDelete", "before-delete", "deleted"},
	{"AfterCreate", "afterCreate", "ProcessCoreTaggedFieldsAfterCreate", "after-create", "created"},
	{"AfterUpdate", "afterUpdate", "ProcessCoreTaggedFieldsAfterUpdate", "after-update", "updated"},
	{"AfterDelete", "afterDelete", "ProcessCoreTaggedFieldsAfterDelete", "after-delete", "deleted"},
}

// GenValidationHooks emits per-group GORM hook boilerplate and scaffolds a
// companion file of empty extension-point stubs for hand-written logic.
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
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)
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

		for _, h := range hooks {
			f.Comment(fmt.Sprintf(
				"%s is the GORM %s hook for %s.",
				h.gormName, h.phase, typeName,
			))
			f.Func().Params(
				Id(receiver).Op("*").Id(typeName),
			).Id(h.gormName).Params(
				Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
			).Error().Block(
				If(
					Err().Op(":=").Id(receiver).Dot(h.userName).Call(Id("tx")),
					Err().Op("!=").Nil(),
				).Block(
					Return(Err()),
				),
				Return(processCall(h.coreName)),
			)
			f.Line()
		}
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
// <group>_validate.go of empty extension-point stubs if no such file exists.
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
	f.HeaderComment(sdk.HeaderCommentGenMod)
	f.ImportAlias("gorm.io/gorm", "gorm")

	for _, apiObj := range objGroup.ApiObjects {
		typeName := apiObj.TypeName
		receiver := strings.ToLower(string(typeName[0]))

		for _, h := range hooks {
			tense := "before"
			if strings.HasPrefix(h.gormName, "After") {
				tense = "after"
			}
			f.Comment(fmt.Sprintf(
				"%s runs %s the %s is %s.",
				h.userName, tense, typeName, h.verb,
			))
			f.Func().Params(
				Id(receiver).Op("*").Id(typeName),
			).Id(h.userName).Params(
				Id("tx").Op("*").Qual("gorm.io/gorm", "DB"),
			).Error().Block(
				Return(Nil()),
			)
			f.Line()
		}
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
