package v0

import (
	"go/build"
	"path/filepath"
	"testing"
)

// TestInstallDirPrefersGobin covers GOBIN winning over GOPATH/bin.
func TestInstallDirPrefersGobin(t *testing.T) {
	// set a GOBIN override
	t.Setenv("GOBIN", "/custom/gobin")
	// assert GOBIN is the install directory
	if got := InstallDir(); got != "/custom/gobin" {
		t.Errorf("InstallDir() = %q, want /custom/gobin", got)
	}
}

// TestInstallDirFallsBackToGopathBin covers an empty GOBIN falling back
// to GOPATH/bin.
func TestInstallDirFallsBackToGopathBin(t *testing.T) {
	// clear GOBIN so the GOPATH/bin fallback applies
	t.Setenv("GOBIN", "")
	// resolve the install directory
	got := InstallDir()
	want := filepath.Join(build.Default.GOPATH, "bin")
	// assert the GOPATH/bin fallback
	if got != want {
		t.Errorf("InstallDir() = %q, want %q", got, want)
	}
	// assert GOPATH/bin is non-empty via go/build's home go default
	if got == "" {
		t.Errorf("InstallDir() returned empty, want a non-empty GOPATH/bin")
	}
}

func TestImageBuildParallelismParsesValidCount(t *testing.T) {
	t.Setenv("PARALLEL_IMAGE_BUILD", "4")
	if got := ImageBuildParallelism(); got != 4 {
		t.Errorf("ImageBuildParallelism() = %d, want 4", got)
	}
}

func TestImageBuildParallelismFloorsInvalidAndNonPositive(t *testing.T) {
	for _, in := range []string{"abc", "0", "-3"} {
		t.Run(in, func(t *testing.T) {
			t.Setenv("PARALLEL_IMAGE_BUILD", in)
			if got := ImageBuildParallelism(); got != 1 {
				t.Errorf("ImageBuildParallelism(%q) = %d, want 1", in, got)
			}
		})
	}
}
