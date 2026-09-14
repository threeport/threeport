package v0

import (
	"go/build"
	"path/filepath"
	"runtime"
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

// TestGetBuildValsUsesArchEnv covers ARCH winning over GOARCH.
func TestGetBuildValsUsesArchEnv(t *testing.T) {
	// set a comma-separated ARCH override
	t.Setenv("ARCH", "amd64,arm64")
	// resolve cwd and arch
	_, arch, err := GetBuildVals()
	if err != nil {
		t.Fatalf("GetBuildVals() error = %v", err)
	}
	// assert ARCH is returned unchanged
	if arch != "amd64,arm64" {
		t.Errorf("GetBuildVals() arch = %q, want amd64,arm64", arch)
	}
}

// TestGetBuildValsDefaultsArchToGOARCH covers an empty ARCH falling
// back to runtime.GOARCH.
func TestGetBuildValsDefaultsArchToGOARCH(t *testing.T) {
	// clear ARCH so the GOARCH fallback applies
	t.Setenv("ARCH", "")
	// resolve cwd and arch
	dir, arch, err := GetBuildVals()
	if err != nil {
		t.Fatalf("GetBuildVals() error = %v", err)
	}
	// assert cwd is non-empty
	if dir == "" {
		t.Errorf("GetBuildVals() dir empty, want cwd")
	}
	// assert arch is the local CPU architecture
	if arch != runtime.GOARCH {
		t.Errorf("GetBuildVals() arch = %q, want %q", arch, runtime.GOARCH)
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
