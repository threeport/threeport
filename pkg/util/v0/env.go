package v0

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
)

// InstallDir returns the directory `go install` writes binaries to:
// $GOBIN if set, otherwise $GOPATH/bin. build.Default.GOPATH falls back
// to ~/go when $GOPATH is unset, so the result is always non-empty.
func InstallDir() string {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return gobin
	}
	return filepath.Join(build.Default.GOPATH, "bin")
}

// GetBuildVals returns the working directory and the arch list to
// build for. Arch comes from ARCH, comma-separated for multi-arch, or
// defaults to the local CPU architecture.
func GetBuildVals() (string, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get working directory: %w", err)
	}

	arch := os.Getenv("ARCH")
	if arch == "" {
		arch = runtime.GOARCH
	}

	return workingDir, arch, nil
}
