package cmd

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

func GetPathFromGoModule() (string, error) {
	goModFilePath, exists := os.LookupEnv(GoModEnvVar)
	if !exists {
		return "", fmt.Errorf("failed to find env var %s set for extension codegen", GoModEnvVar)
	}

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
