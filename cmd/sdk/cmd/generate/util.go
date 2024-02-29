/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package gen

import (
	"fmt"
	"os"

	"golang.org/x/mod/modfile"
)

// flag used to indicate whether the command is being run for an extension
var extension bool

const GoModEnvVar string = "GO_MOD_FILE_PATH"

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
