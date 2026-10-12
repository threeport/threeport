package installer

import (
	"fmt"
	"path/filepath"
	"sort"

	. "github.com/dave/jennifer/jen"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenApiObjectGroupNames writes the sorted sdk-config API object group names to a generated file.
func GenApiObjectGroupNames(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// collect group names from the sdk config
	var names []string
	for _, group := range sdkConfig.ApiObjectGroups {
		if group == nil || group.Name == nil {
			continue
		}
		names = append(names, *group.Name)
	}
	sort.Strings(names)

	f := NewFile("v0")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	f.Var().Id("ApiObjectGroupNames").Op("=").Index().String().ValuesFunc(func(v *Group) {
		for _, name := range names {
			v.Line().Lit(name)
		}
		v.Line()
	})

	// modules write under pkg/installer; threeport-core writes under pkg/threeport-installer
	var genFilepath string
	if generator.Module {
		genFilepath = filepath.Join(
			"pkg",
			"installer",
			"v0",
			"api_object_groups_gen.go",
		)
	} else {
		genFilepath = filepath.Join(
			"pkg",
			"threeport-installer",
			"v0",
			"api_object_groups_gen.go",
		)
	}

	if _, err := util.WriteCodeToFile(f, genFilepath, true); err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	cli.Info(fmt.Sprintf("source code for API object group names written to %s", genFilepath))

	return nil
}
