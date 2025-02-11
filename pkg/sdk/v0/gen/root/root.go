package root

import (
	"fmt"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// GenRoot generates source code at root of project.
func GenRoot(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// generate magefile
	if err := GenMagefile(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate Magefile at project root: %w", err)
	}

	return nil
}
