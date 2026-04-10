package v0

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBuildBinary tests the BuildBinary function with a valid input.
func TestBuildBinary(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go not installed: %v", err)
	}

	// Create a real temporary Go project.
	rootDir := t.TempDir()

	err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte(`module example.com/test

go 1.22
`), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	err = os.MkdirAll(filepath.Join(rootDir, "cmd", "tptctl"), 0755)
	if err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "cmd", "tptctl", "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0644)
	if err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Call BuildBinary with valid inputs.
	err = BuildBinary(rootDir, "amd64", "test-binary", "cmd/tptctl/main.go", false)
	if err != nil {
		t.Errorf(`BuildBinary(rootDir, "amd64", "test-binary", "cmd/tptctl/main.go", false) failed: %v`, err)
	}
}

// TestBuildBinary_Failure tests the BuildBinary function with a failing command.
func TestBuildBinary_Failure(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go not installed: %v", err)
	}

	// Create a real temporary Go project.
	rootDir := t.TempDir()

	err := os.WriteFile(filepath.Join(rootDir, "go.mod"), []byte(`module example.com/test

go 1.22
`), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Call BuildBinary with an invalid main.go path and expect an error.
	err = BuildBinary(rootDir, "amd64", "test-binary", "cmd/main.go", false)
	if err == nil {
		t.Errorf(`BuildBinary(rootDir, "amd64", "test-binary", "cmd/main.go", false) expected error, got nil`)
	}
}

// TestBuildImage tests the BuildImage function with valid inputs.
func TestBuildImage(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker not installed: %v", err)
	}
	if err := exec.Command("docker", "buildx", "version").Run(); err != nil {
		t.Fatalf("docker buildx not available: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not running")
	}

	// Create a real temporary Docker build context.
	rootDir := t.TempDir()

	err := os.MkdirAll(filepath.Join(rootDir, "cmd", "rest-api", "image"), 0755)
	if err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "cmd", "rest-api", "image", "hello.txt"), []byte("hello\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write hello.txt: %v", err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "cmd", "rest-api", "image", "Dockerfile"), []byte(`FROM scratch
COPY hello.txt /hello.txt
`), 0644)
	if err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	dockerfilePath := filepath.Join(rootDir, "cmd", "rest-api", "image", "Dockerfile")
	err = BuildImage(rootDir, dockerfilePath, "amd64", "test-repo", "test-image", "latest", false, false, "")
	if err != nil {
		t.Errorf(`BuildImage failed: %v`, err)
	}
}

// TestBuildImage_Failure tests the BuildImage function with a failing command.
func TestBuildImage_Failure(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker not installed: %v", err)
	}

	// Create a real temporary directory with no Dockerfile.
	rootDir := t.TempDir()

	// Call BuildImage and expect an error.
	err := BuildImage(rootDir, "Dockerfile", "amd64", "test-repo", "test-image", "latest", false, false, "")
	if err == nil {
		t.Errorf(`BuildImage(rootDir, "Dockerfile", "amd64", "test-repo", "test-image", "latest", false, false, "") expected error, got nil`)
	}
}
