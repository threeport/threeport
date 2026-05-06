package pkg

import (
	"fmt"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/api"
	apiserver "github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/api-server"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/client"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/config"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/pkg/installer"
)

// GenPkg generates source code for pkg packages.
func GenPkg(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	////////////////////////////// pkg/api /////////////////////////////////////
	// generate API object constants and methods
	if err := api.GenApiObjectMethods(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API object methods: %w", err)
	}

	// generate methods that set objects' DB table names
	if err := api.GenTableNames(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate APi object table name methods: %w", err)
	}

	// generate GORM validation/encryption hooks and scaffold custom validation
	if err := api.GenValidationHooks(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API object validation hooks: %w", err)
	}

	//////////////////////////// pkg/api-server ////////////////////////////////
	// generate API server routes
	if err := apiserver.GenRoutes(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API server routes: %w", err)
	}

	// generate function to add all generated routes in api-server package
	if err := apiserver.GenAddGenRoutes(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate function to add API server generated routes: %w", err)
	}

	// generate function to add all custom routes in api-server package
	if err := apiserver.GenAddCustomRoutes(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate function to add API server custom routes: %w", err)
	}

	// generate API server handlers
	if err := apiserver.GenHandlers(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API server handlers for API objects: %w", err)
	}

	// generate API handler wrapper for Threeport extensions
	if generator.Module {
		if err := apiserver.GenHandlerWrapper(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate API handler wrapper: %w", err)
		}
	}

	// add API object field validation and versions to API server
	if err := apiserver.GenObjValidationVersions(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API object validation, versions: %w", err)
	}

	// add database initialization and GORM logger methods
	if err := apiserver.GenDatabaseInit(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate database initialization: %w", err)
	}

	// tagged feilds vars for each API object
	if err := apiserver.GenObjectTaggedFields(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate object tagged fields vars: %w", err)
	}

	// add the functions to add API object versions to the API server
	if err := apiserver.GenAddVersionsFuncs(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate functions to add API object versions to API server: %w", err)
	}

	// the module registration is different for core threeport and extension modules:
	// * core threeport registers directly with the database
	// * extension modules register with the Threeport API server as the objects used for
	//   registration are core Threeport objects and the dynamic routes must be added for
	//   extensions to proxy connection from core Threeport API to extension module API.
	if generator.Module {
		// add the module registration function
		if err := apiserver.GenModuleRegistration(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module registration function: %w", err)
		}
	} else {
		// add the module registration function for core threeport
		if err := apiserver.GenCoreModuleRegistration(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate module registration function: %w", err)
		}
	}

	////////////////////////////// pkg/client //////////////////////////////////
	if err := client.GenClientLib(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate API client library: %w", err)
	}

	// generate custom function to delete by object type and ID for
	// threeport/threeport only
	if !generator.Module {
		if err := client.GenDeleteObjByTypeAndId(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate custom delete function: %w", err)
		}
	}

	////////////////////////////// pkg/config //////////////////////////////////
	// generate config abstractions
	if err := config.GenConfig(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate config package: %w", err)
	}

	//////////////////////////// pkg/installer /////////////////////////////////
	// install extension API and controller and register with an existing
	// Threeport control plane
	if generator.Module {
		if err := installer.GenInstaller(generator, sdkConfig); err != nil {
			return fmt.Errorf("failed to generate installer package: %w", err)
		}
	}

	return nil
}
