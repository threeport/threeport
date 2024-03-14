package versions

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
)

// AddVersions generates code for the function that adds version maps for all
// objects in the API.
func (gvc *GlobalVersionConfig) AddVersions() error {

	// put all the version function calls in a slice
	// since all versions will be in one version function, only add once if a
	// model has multiple versions
	for _, v := range gvc.Versions {
		f := NewFile("versions")
		f.HeaderComment("generated by 'threeport-sdk codegen api-version' - do not edit")
		var versionFuncs []string
		for _, n := range v.RouteNames {
			if !contains(versionFuncs, n) {
				versionFuncs = append(versionFuncs, n)
			}
		}

		versionFuncCalls := &Statement{}
		for _, vf := range versionFuncs {
			versionFuncCalls.Id(fmt.Sprintf("Add%sVersions", vf)).Call()
			versionFuncCalls.Line()
		}

		f.Func().Id("AddVersions").Params().Block(
			versionFuncCalls,
		)

		// write code to file
		routesFilepath := filepath.Join("..", "..", "pkg", "api-server", v.VersionName, "versions", "versions_gen.go")
		file, err := os.OpenFile(routesFilepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file to write generated code to add versions: %w", err)
		}
		defer file.Close()
		if err := f.Render(file); err != nil {
			return fmt.Errorf("failed to render generated source code to add versions: %w", err)
		}
		fmt.Println("code generation complete to add versions")
	}

	return nil
}
