package main

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
