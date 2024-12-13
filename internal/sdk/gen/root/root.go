package root

import (
	"fmt"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
)

// GenRoot generates source code at root of project.
func GenRoot(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	// generate magefile
	if err := GenMagefile(generator, sdkConfig); err != nil {
		return fmt.Errorf("failed to generate Magefile at project root: %w", err)
	}

	return nil
}
