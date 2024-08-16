package v0

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BuildBinary builds the go binary for a threeport control plane component.
func BuildBinary(
	threeportPath string,
	arch string,
	binName string,
	mainPath string,
	noCache bool,
) error {
	// construct build arguments
	buildArgs := []string{"build"}

	// append build flags
	buildArgs = append(buildArgs, "-gcflags=\\\"all=-N -l\\\"") // escape quotes and escape char for shell

	// append no cache flag if specified
	if noCache {
		buildArgs = append(buildArgs, "-a")
	}

	// append output flag
	buildArgs = append(buildArgs, "-o")

	// append binary name
	buildArgs = append(buildArgs, "bin/"+binName)

	// append main.go filepath
	buildArgs = append(buildArgs, mainPath)

	fmt.Printf("go %s \n", strings.Join(buildArgs, " "))

	// construct build command
	cmd := exec.Command("go", buildArgs...)
	cmd.Env = os.Environ()
	goEnv := []string{
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=" + arch,
	}
	cmd.Env = append(cmd.Env, goEnv...)
	cmd.Dir = threeportPath

	// start build command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build %s: %v\noutput:\n%s", binName, err, string(output))
	}

	return nil
}
