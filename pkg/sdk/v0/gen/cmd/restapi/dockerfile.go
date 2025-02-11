package restapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenRestApiDockerfile generates the REST API's Dockerfiles and writes them if
// they don't already exist.
func GenRestApiDockerfile(gen *gen.Generator) error {
	// get content for each Dockerfile
	dockerfileMap := util.GetDockerfiles("rest-api", gen.GoVersion)

	// write each Dockerfile if it doesn't already exist
	for fileName, fileContent := range dockerfileMap {
		dockerfilePath := filepath.Join("cmd", "rest-api", "image")
		if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
			return fmt.Errorf("failed to ensure REST API Dockerfile directories exist: %w", err)
		}

		// check if file exists - return without error if it does
		dockerfileFile := filepath.Join(dockerfilePath, fileName)
		dockerfileExists := true
		if _, err := os.Stat(dockerfileFile); errors.Is(err, os.ErrNotExist) {
			dockerfileExists = false
		}
		if dockerfileExists {
			cli.Info(fmt.Sprintf("REST API Dockerfile already exists at %s - not overwritten", dockerfileFile))
			continue
		}

		// file doesn't exist - write it
		if err := os.WriteFile(dockerfileFile, []byte(fileContent), 0644); err != nil {
			return fmt.Errorf("failed to write REST API Dockerfile to %s: %w", dockerfileFile, err)
		}
		cli.Info(fmt.Sprintf("REST API Dockerfile written to %s", dockerfileFile))
	}

	return nil
}
