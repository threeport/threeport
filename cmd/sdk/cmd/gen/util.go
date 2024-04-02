package gen

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

var modulePath string

const threeportGoPath string = "github.com/threeport/threeport"

// Determine whether the sdk is being run from an extension
func isExtension() (bool, string, error) {
	modPath, err := getPathFromGoModule()
	if err != nil {
		return false, "", fmt.Errorf("could not get go mod path: %w", err)
	}

	if modPath == threeportGoPath {
		return false, modPath, nil
	}

	return true, modPath, nil
}

func getPathFromGoModule() (string, error) {
	goModFilePath := "./go.mod"
	goModBytes, err := os.ReadFile(goModFilePath)
	if err != nil {
		return "", fmt.Errorf("could not read go mod file from provided path %s: %w", goModFilePath, err)
	}

	f, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return "", fmt.Errorf("could not parse go mod file: %w", err)
	}

	return f.Module.Mod.Path, nil
}
