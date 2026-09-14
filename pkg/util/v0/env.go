package v0

import (
	"go/build"
	"os"
	"path/filepath"
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
