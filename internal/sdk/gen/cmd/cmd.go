package cmd

import (
	"fmt"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/cli"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/controller"
	"github.com/threeport/threeport/internal/sdk/gen/cmd/dbmigrator"
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

	// generate DB migrator main package
	if err := dbmigrator.GenDbMigratorMain(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate DB migrator main package: %w", err)
	}

	// generate DB migrator Dockerfile
	if err := dbmigrator.GenDbMigratorDockerfile(generator); err != nil {
		return fmt.Errorf("failed to generate DB migrator Dockerfiles: %w", err)
	}

	// generate DB migrator migrations utils
	if err := dbmigrator.GenDbMigratorUtils(generator); err != nil {
		return fmt.Errorf("failed to generate DB migrator migration utils: %w", err)
	}

	// generate DB migrator initial DB migration
	if err := dbmigrator.GenDbMigratorMigration(generator); err != nil {
		return fmt.Errorf("failed to generate DB migrator migration: %w", err)
	}

	// generate controller main packages
	if err := controller.GenControllerMain(generator); err != nil {
		return fmt.Errorf("failed to generate controller main packages: %w", err)
	}

	// generate controller Dockerfiles
	if err := controller.GenControllerDockerfiles(generator); err != nil {
		return fmt.Errorf("failed to generate controller Dockerfiles: %w", err)
	}

	// generate module tptctl plugin main package
	if generator.Module {
		if err := cli.GenPluginMain(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module plugin main package: %w", err)
		}
	}

	// generate extension tptctl plugin plugin root command
	if generator.Module {
		if err := cli.GenPluginRootCmd(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module plugin root command: %w", err)
		}
	}

	// generate extension tptctl plugin install command
	if generator.Module {
		if err := cli.GenPluginInstallCmd(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module plugin install command: %w", err)
		}
	}

	// generate module tptctl plugin CRUD commands
	if generator.Module {
		if err := cli.GenPluginCrudCmds(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module plugin CRUD commands: %w", err)
		}
	}

	// generate standard commands for tptctl and module plugins
	if err := cli.GenCliCommands(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate tptctl commands: %w", err)
	}

	return nil
}
