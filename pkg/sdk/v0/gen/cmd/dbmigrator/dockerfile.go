package dbmigrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
)

// GenDbMigratorDockerfile generates the database migrator's Dockerfiles and
// writes them if they don't already exist and are not excluded by SDK config.
func GenDbMigratorDockerfile(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// get content for each Dockerfile
	dockerfileMap := util.GetDockerfiles("database-migrator", gen.GoVersion)

	// write each Dockerfile if it doesn't already exist and not excluded
	for fileName, fileContent := range dockerfileMap {
		dockerfilePath := filepath.Join("cmd", "database-migrator", "image")
		if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
			return fmt.Errorf("failed to ensure database migrator Dockerfile directories exist: %w", err)
		}

		// check if file exists - return without error if it does
		dockerfileFile := filepath.Join(dockerfilePath, fileName)
		dockerfileExists := true
		if _, err := os.Stat(dockerfileFile); errors.Is(err, os.ErrNotExist) {
			dockerfileExists = false
		}
		if dockerfileExists {
			cli.Info(fmt.Sprintf("database migrator Dockerfile already exists at %s - not overwritten", dockerfileFile))
			continue
		}

		// check if file is excluded by SDK config
		if slices.Contains(sdkConfig.ExcludeFiles, dockerfileFile) {
			cli.Info(fmt.Sprintf("source code generation skipped for %s", dockerfileFile))
		} else {
			// file doesn't exist and not excluded - write it
			if err := os.WriteFile(dockerfileFile, []byte(fileContent), 0644); err != nil {
				return fmt.Errorf("failed to write database migrator Dockerfile to %s: %w", dockerfileFile, err)
			}
			cli.Info(fmt.Sprintf("database migrator Dockerfile written to %s", dockerfileFile))
		}
	}

	return nil
}
