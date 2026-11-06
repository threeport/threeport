package pulumitest

import (
	"os"
	"os/exec"
	"testing"
)

// RequireCLIEnv names the variable that turns a missing pulumi CLI from a
// skip into a failure. CI sets it, so a job that stops installing the CLI
// fails instead of quietly skipping every test that needs a real backend.
const RequireCLIEnv = "THREEPORT_REQUIRE_PULUMI"

// RequireCLI skips the test when the pulumi CLI is not on PATH, or fails it
// when RequireCLIEnv is set to 1.
func RequireCLI(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("pulumi"); err == nil {
		return
	}
	if os.Getenv(RequireCLIEnv) == "1" {
		t.Fatalf("pulumi CLI not found on PATH and %s=1", RequireCLIEnv)
	}
	t.Skip("pulumi CLI not found on PATH; skipping test that needs a real pulumi backend")
}
