package controller

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenControllerDockerfiles generates each controller's Dockerfile.
func GenControllerDockerfiles(gen *gen.Generator) error {
	for _, objGroup := range gen.ApiObjectGroups {
		if len(objGroup.ReconciledObjects) > 0 {
			dockerfileString := util.CreateDockerfile(objGroup.ControllerName, gen.GoVersion)

			dockerfilePath := filepath.Join("cmd", objGroup.ControllerName, "image")
			if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
				return fmt.Errorf("failed to ensure %s Dockerfile directories exist: %w", objGroup.ControllerName, err)
			}

			dockerfileFile := filepath.Join(dockerfilePath, "Dockerfile")
			if err := os.WriteFile(dockerfileFile, []byte(dockerfileString), 0644); err != nil {
				return fmt.Errorf("failed to write %s Dockerfile to %s: %w", objGroup.ControllerName, dockerfileFile, err)
			}
			cli.Info(fmt.Sprintf("%s Dockerfile written to %s", objGroup.ControllerName, dockerfileFile))
		}
	}

	return nil
}
