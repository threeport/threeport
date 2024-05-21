package restapi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenRestApiDockerfile generates the REST API's Dockerfile.
func GenRestApiDockerfile(gen *gen.Generator) error {
	dockerfileString := util.CreateDockerfile("rest-api", gen.GoVersion)

	dockerfilePath := filepath.Join("cmd", "rest-api", "image")
	if err := os.MkdirAll(dockerfilePath, 0755); err != nil {
		return fmt.Errorf("failed to ensure REST API Dockerfile directories exist: %w", err)
	}

	dockerfileFile := filepath.Join(dockerfilePath, "Dockerfile")
	if err := os.WriteFile(dockerfileFile, []byte(dockerfileString), 0644); err != nil {
		return fmt.Errorf("failed to write REST API Dockerfile to %s: %w", dockerfileFile, err)
	}
	cli.Info(fmt.Sprintf("REST API Dockerfile written to %s", dockerfileFile))

	return nil
}
