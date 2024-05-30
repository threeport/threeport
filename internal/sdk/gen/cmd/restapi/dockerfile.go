package restapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenRestApiDockerfile generates the REST API's Dockerfile and writes it if it
// doesn't already exist.
func GenRestApiDockerfile(gen *gen.Generator) error {
	dockerfileString := util.CreateDockerfile("rest-api", gen.GoVersion)

	dockerfilePath := filepath.Join("cmd", "rest-api", "image")
	if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
		return fmt.Errorf("failed to ensure REST API Dockerfile directories exist: %w", err)
	}

	// check if file exists - return without error if it does
	dockerfileFile := filepath.Join(dockerfilePath, "Dockerfile")
	dockerfileExists := true
	if _, err := os.Stat(dockerfileFile); errors.Is(err, os.ErrNotExist) {
		dockerfileExists = false
	}
	if dockerfileExists {
		cli.Info(fmt.Sprintf("REST API Dockerfile already exists at %s - not overwritten", dockerfileFile))
		return nil
	}

	// file doesn't exist - write it
	if err := os.WriteFile(dockerfileFile, []byte(dockerfileString), 0644); err != nil {
		return fmt.Errorf("failed to write REST API Dockerfile to %s: %w", dockerfileFile, err)
	}
	cli.Info(fmt.Sprintf("REST API Dockerfile written to %s", dockerfileFile))

	return nil
}
