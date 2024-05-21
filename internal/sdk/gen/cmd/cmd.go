package cmd

import (
	"fmt"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/cli"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/controller"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/restapi"
)

// GenCmd generates source code for cmd packages.
func GenCmd(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// generate REST API main package
	if err := restapi.GenRestApiMain(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate REST API main package: %w", err)
	}

	// generate REST API Dockerfile
	if err := restapi.GenRestApiDockerfile(generator); err != nil {
		return fmt.Errorf("failed to generate REST API Dockerfile: %w", err)
	}

	// generate JetStream init in REST API util package
	if err := restapi.GenUtilJetstream(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate NATS JetStream initialization code in REST API util package: %w", err)
	}

	// generate version route in REST API util package
	if err := restapi.GenUtilVersion(generator); err != nil {
		return fmt.Errorf("failed to generate version route in REST API util package: %w", err)
	}

	// generate controller main packages
	if err := controller.GenControllerMain(generator); err != nil {
		return fmt.Errorf("failed to generate controller main packages: %w", err)
	}

	// generate controller Dockerfiles
	if err := controller.GenControllerDockerfiles(generator); err != nil {
		return fmt.Errorf("failed to generate controller Dockerfiles: %w", err)
	}

	// generate CLI commands
	// TODO
	if err := cli.GenCliCommands(generator); err != nil {
		return fmt.Errorf("failed to generate CLI commands: %w", err)
	}

	return nil
}
