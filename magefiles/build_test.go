package main

import (
	"go/build"
	"path/filepath"
	"testing"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestParallelFromEnvDefaultsToDoubleBuildParallelism covers an empty
// PARALLEL_IMAGE_BUILD resolving to twice the compile worker count.
func TestParallelFromEnvDefaultsToDoubleBuildParallelism(t *testing.T) {
	// set an empty image-build override
	t.Setenv("PARALLEL_IMAGE_BUILD", "")
	want := util.BuildParallelism() * 2
	// assert twice the compile worker count
	if got := parallelFromEnv(); got != want {
		t.Errorf("parallelFromEnv() = %d, want %d (2x BuildParallelism)", got, want)
	}
}

// TestParallelFromEnvParsesValidCount covers a positive integer override.
func TestParallelFromEnvParsesValidCount(t *testing.T) {
	// set a valid worker count
	t.Setenv("PARALLEL_IMAGE_BUILD", "4")
	// assert the override is used as-is
	if got := parallelFromEnv(); got != 4 {
		t.Errorf("parallelFromEnv() = %d, want 4", got)
	}
}

// TestParallelFromEnvFloorsInvalidAndNonPositive rejects a non-numeric or
// non-positive PARALLEL_IMAGE_BUILD by flooring the worker count to 1.
func TestParallelFromEnvFloorsInvalidAndNonPositive(t *testing.T) {
	// try values that cannot serve as a worker count
	for _, in := range []string{"abc", "0", "-3"} {
		t.Run(in, func(t *testing.T) {
			// apply the invalid override
			t.Setenv("PARALLEL_IMAGE_BUILD", in)
			// assert the worker count floors to 1
			if got := parallelFromEnv(); got != 1 {
				t.Errorf("parallelFromEnv(%q) = %d, want 1", in, got)
			}
		})
	}
}

// TestInstallDirPrefersGobin covers GOBIN winning over GOPATH/bin.
func TestInstallDirPrefersGobin(t *testing.T) {
	// set a GOBIN override
	t.Setenv("GOBIN", "/custom/gobin")
	// assert GOBIN is the install directory
	if got := installDir(); got != "/custom/gobin" {
		t.Errorf("installDir() = %q, want /custom/gobin", got)
	}
}

// TestInstallDirFallsBackToGopathBin covers an empty GOBIN falling back
// to GOPATH/bin.
func TestInstallDirFallsBackToGopathBin(t *testing.T) {
	// clear GOBIN so the GOPATH/bin fallback applies
	t.Setenv("GOBIN", "")
	// resolve the install directory
	got := installDir()
	want := filepath.Join(build.Default.GOPATH, "bin")
	// assert the GOPATH/bin fallback
	if got != want {
		t.Errorf("installDir() = %q, want %q", got, want)
	}
	// assert GOPATH/bin is non-empty via go/build's home go default
	if got == "" {
		t.Errorf("installDir() returned empty, want a non-empty GOPATH/bin")
	}
}

// TestEnvOrReturnsSetValue covers a set value returning with surrounding
// whitespace trimmed.
func TestEnvOrReturnsSetValue(t *testing.T) {
	// set a padded value on a throwaway key
	t.Setenv("TPT_TEST_ENVOR", "  hello  ")
	// assert surrounding whitespace is trimmed
	if got := envOr("TPT_TEST_ENVOR", "fallback"); got != "hello" {
		t.Errorf("envOr() = %q, want %q", got, "hello")
	}
}

// TestEnvOrFallsBackOnEmptyOrWhitespace covers unset, empty, and
// whitespace-only values falling back to the default.
func TestEnvOrFallsBackOnEmptyOrWhitespace(t *testing.T) {
	// try unset, empty, and whitespace-only
	cases := []struct {
		name string
		set  bool
		val  string
	}{
		// leave the var unset
		{"unset", false, ""},
		// set the var empty
		{"empty", true, ""},
		// set the var to whitespace only
		{"whitespace only", true, "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				// apply the empty or whitespace value
				t.Setenv("TPT_TEST_ENVOR", c.val)
			}
			// assert the default is used
			if got := envOr("TPT_TEST_ENVOR", "fallback"); got != "fallback" {
				t.Errorf("envOr() = %q, want %q", got, "fallback")
			}
		})
	}
}
