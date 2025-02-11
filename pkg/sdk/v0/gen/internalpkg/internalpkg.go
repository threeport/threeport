package internalpkg

import (
	"fmt"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/internalpkg/controller"
	"github.com/threeport/threeport/pkg/sdk/v0/gen/internalpkg/version"
)

// GenInternalPkg generates source code for internal packages.
func GenInternalPkg(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// generate internal version package
	if err := version.GenVersionPackage(); err != nil {
		return fmt.Errorf("failed to generate internal version package: %w", err)
	}

	// generate controller internal package
	if err := controller.GenController(generator); err != nil {
		return fmt.Errorf("failed to generate controller internal package: %w", err)
	}

	// generate controller reconcilers
	if err := controller.GenReconcilers(generator); err != nil {
		return fmt.Errorf("failed to generate controller reconcilers: %w", err)
	}

	// generate reconciler operation functions
	if err := controller.GenReconcilerOperations(generator); err != nil {
		return fmt.Errorf("failed to generate reconciler operation functions: %w", err)
	}

	// generate NATS notification constants and functions for controllers
	if err := controller.GenNotifs(generator); err != nil {
		return fmt.Errorf("failed to generate notif constants and functions for controller: %w", err)
	}

	return nil
}
