package util

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/mod/modfile"
)

const threeportGoPath string = "github.com/threeport/threeport"

// GetPathFromGoModule gets the path from the module directive in go.mod.
func GetPathFromGoModule() (string, error) {
	goModFile, err := parseModFile()
	if err != nil {
		return "", fmt.Errorf("failed to parse Go mod file: %s", err)
	}

	return goModFile.Module.Mod.Path, nil
}

// GetMajorMinorVersionFromGoModule gets the go version from go.mod excluding
// the bugfix version, e.g. 1.22.
func GetMajorMinorVersionFromGoModule() (string, error) {
	goModFile, err := parseModFile()
	if err != nil {
		return "", fmt.Errorf("failed to parse Go mod file: %s", err)
	}

	parsedVersion := strings.Split(goModFile.Go.Version, ".")
	version := fmt.Sprintf("%s.%s", parsedVersion[0], parsedVersion[1])

	return version, nil
}

// IsExtension determines whether the sdk is being run from an extension.
func IsExtension() (bool, string, error) {
	modPath, err := GetPathFromGoModule()
	if err != nil {
		return false, "", fmt.Errorf("could not get go mod path: %w", err)
	}

	if modPath == threeportGoPath {
		return false, modPath, nil
	}

	return true, modPath, nil
}

// parseModFile parses the go module details from go.mod.
func parseModFile() (*modfile.File, error) {
	goModFilePath := "./go.mod"
	goModBytes, err := os.ReadFile(goModFilePath)
	if err != nil {
		return nil, fmt.Errorf("could not read go mod file from provided path %s: %w", goModFilePath, err)
	}

	goModFile, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse go mod file: %w", err)
	}

	return goModFile, nil
}
