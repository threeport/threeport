package controller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenControllerDockerfiles generates each controller's Dockerfiles.
func GenControllerDockerfiles(gen *gen.Generator) error {
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			// get content for each Dockerfile
			dockerfileMap := util.GetDockerfiles(objGroup.ControllerName, gen.GoVersion)

			// write each Dockerfile if it doesn't already exist
			for fileName, fileContent := range dockerfileMap {
				dockerfilePath := filepath.Join("cmd", objGroup.ControllerName, "image")
				if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
					return fmt.Errorf("failed to ensure %s Dockerfile directories exist: %w", objGroup.ControllerName, err)
				}

				// check if file exists - skip if it does
				dockerfileFile := filepath.Join(dockerfilePath, fileName)
				dockerfileExists := true
				if _, err := os.Stat(dockerfileFile); errors.Is(err, os.ErrNotExist) {
					dockerfileExists = false
				}
				if dockerfileExists {
					cli.Info(fmt.Sprintf(
						"%s %s already exists at %s - not overwritten",
						objGroup.ControllerName,
						fileName,
						dockerfileFile,
					))
					continue
				}

				// file doesn't exist - write it
				if err := os.WriteFile(dockerfileFile, []byte(fileContent), 0644); err != nil {
					return fmt.Errorf("failed to write %s Dockerfile to %s: %w", objGroup.ControllerName, dockerfileFile, err)
				}
				cli.Info(fmt.Sprintf("%s Dockerfile written to %s", objGroup.ControllerName, dockerfileFile))
			}
		}
	}

	return nil
}
