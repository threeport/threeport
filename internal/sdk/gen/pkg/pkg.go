package pkg

import (
	"fmt"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/gen/pkg/api"
	apiserver "github.com/threeport/threeport/internal/sdk/gen/pkg/api-server"
	"github.com/threeport/threeport/internal/sdk/gen/pkg/client"
	"github.com/threeport/threeport/internal/sdk/gen/pkg/config"
	"github.com/threeport/threeport/internal/sdk/gen/pkg/installer"
)

// GenPkg generates source code for pkg packages.
func GenPkg(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	////////////////////////////// pkg/api /////////////////////////////////////
	// generate API object constants and methods
	if err := api.GenApiObjectMethods(generator); err != nil {
		return fmt.Errorf("failed to generate API object methods: %w", err)
	}

	//////////////////////////// pkg/api-server ////////////////////////////////
	// generate API server routes
	if err := apiserver.GenRoutes(generator); err != nil {
		return fmt.Errorf("failed to generate API server routes: %w", err)
	}

	// generate function to add all generated routes in api-server package
	if err := apiserver.GenAddGenRoutes(generator); err != nil {
		return fmt.Errorf("failed to generate function to add API server generated routes: %w", err)
	}

	// generate function to add all custom routes in api-server package
	if err := apiserver.GenAddCustomRoutes(generator); err != nil {
		return fmt.Errorf("failed to generate function to add API server custom routes: %w", err)
	}

	// generate API server handlers
	if err := apiserver.GenHandlers(generator); err != nil {
		return fmt.Errorf("failed to generate API server handlers for API objects: %w", err)
	}

	// generate API handler wrapper for Threeport extensions
	if generator.Extension {
		if err := apiserver.GenHandlerWrapper(generator); err != nil {
			return fmt.Errorf("failed to generate API handler wrapper: %w", err)
		}
	}

	// add API object field validation and versions to API server
	if err := apiserver.GenObjValidationVersions(generator); err != nil {
		return fmt.Errorf("failed to generate API object validation, versions: %w", err)
	}

	// add database initialization and GORM logger methods
	if err := apiserver.GenDatabaseInit(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate database initialization: %w", err)
	}

	// tagged feilds vars for each API object
	if err := apiserver.GenObjectTaggedFields(generator); err != nil {
		return fmt.Errorf("failed to generate object tagged fields vars: %w", err)
	}

	// add the functions to add API object versions to the API server
	if err := apiserver.GenAddVersionsFuncs(generator); err != nil {
		return fmt.Errorf("failed to generate functions to add API object versions to API server: %w", err)
	}

	////////////////////////////// pkg/client //////////////////////////////////
	if err := client.GenClientLib(generator); err != nil {
		return fmt.Errorf("failed to generate API client library: %w", err)
	}

	// generate custom function to delete by object type and ID for
	// threeport/threeport only
	if !generator.Extension {
		if err := client.GenDeleteObjByTypeAndId(generator); err != nil {
			return fmt.Errorf("failed to generate custom delete function: %w", err)
		}
	}

	////////////////////////////// pkg/config //////////////////////////////////
	// TODO: remove generate.Extension if statement to apply to core threeport
	// as well.  Complete codegen for config package.
	if generator.Extension {
		if err := config.GenConfig(generator); err != nil {
			return fmt.Errorf("failed to generate config package: %w", err)
		}
	}

	//////////////////////////// pkg/installer /////////////////////////////////
	// install extension API and controller and register with an existing
	// Threeport control plane
	if generator.Extension {
		if err := installer.GenInstaller(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate installer package: %w", err)
		}
	}

	return nil
}
