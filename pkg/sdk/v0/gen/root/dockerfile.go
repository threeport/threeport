package root

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"slices"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

//go:embed Dockerfile
var dockerfileContent string

// GenDockerfile writes the minimal module Dockerfile to the project
// root if not already present. The embedded source is a stripped-down
// variant of threeport's canonical Dockerfile carrying only the
// release target a module image needs. The write is skipped when a
// Dockerfile already exists so threeport's own repo and any module
// that has customized its Dockerfile are left untouched.
func GenDockerfile(generator *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	const target = "Dockerfile"

	if slices.Contains(sdkConfig.ExcludeFiles, target) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", target))
		return nil
	}

	if _, err := os.Stat(target); err == nil {
		cli.Info(fmt.Sprintf("Dockerfile already exists at %s - not overwritten", target))
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat %s: %w", target, err)
	}

	if err := os.WriteFile(target, []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile to %s: %w", target, err)
	}
	cli.Info(fmt.Sprintf("Dockerfile written to %s", target))

	return nil
}
