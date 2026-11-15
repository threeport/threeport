package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// TestGenPluginInstallCmd_PreRunUsesLocalFunc is a regression test for a bug
// where the generated install command's PreRun was wired to the imported
// threeport/cmd/tptctl/cmd package's CommandPreRunFunc instead of the
// module's own local one (defined in the module's generated root.go).
//
// The imported function reads from that package's own, never-populated
// package-level cliArgs, since an extension module's plugin binary runs as
// a standalone subprocess rather than executing tptctl's own cobra tree in
// process (see cmd/tptctl/cmd/root.go's Execute(), which exec's the plugin).
// The practical effect: --threeport-config (and any other global/persistent
// flag) was silently ignored by every generated module's `install` command,
// which always fell back to the default ~/.threeport/config.yaml path.
//
// Found and originally patched by hand in django-threeport-module
// (threeport/django-threeport-module#7, fixed in #8) before being traced
// back to this generator.
func TestGenPluginInstallCmd_PreRunUsesLocalFunc(t *testing.T) {
	tmpDir := t.TempDir()

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWd); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})

	testGen := &gen.Generator{ModulePath: "example.com/testmodule"}
	testSdkConfig := &sdk.SdkConfig{ModuleName: "testmodule"}

	if err := GenPluginInstallCmd(testGen, testSdkConfig); err != nil {
		t.Fatalf("GenPluginInstallCmd returned an error: %v", err)
	}

	generatedPath := filepath.Join(tmpDir, "cmd", "testmodule", "cmd", "install.go")
	src, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("failed to read generated file %s: %v", generatedPath, err)
	}
	text := string(src)

	if strings.Contains(text, "tptctl_cmd.CommandPreRunFunc") {
		t.Errorf(
			"generated install.go wires PreRun to the imported tptctl_cmd.CommandPreRunFunc; " +
				"this reads from that package's own, never-populated cliArgs and silently ignores " +
				"--threeport-config in the plugin subprocess - PreRun should reference the module's own " +
				"local CommandPreRunFunc instead, matching every other generated command (see command.go)",
		)
	}
	// checked as its own exact assignment, not just "both substrings appear
	// somewhere in the file": PreRun: other.CommandPreRunFunc would satisfy
	// two independent Contains checks just as well as the correct,
	// unqualified assignment would, without actually fixing anything. The
	// whitespace after the colon isn't a single space in the real output -
	// gofmt aligns Dict keys in this struct literal against the longest one
	// (SilenceUsage:), so PreRun: renders with extra padding - hence \s+
	// rather than a literal single space.
	if !regexp.MustCompile(`PreRun:\s+CommandPreRunFunc,`).MatchString(text) {
		t.Fatalf("generated install.go does not wire PreRun to the unqualified local CommandPreRunFunc - generator output may have changed shape:\n%s", text)
	}

	// tptctl_cmd must remain imported and used for GetClientContext in Run -
	// only the PreRun wiring itself is wrong, not the import.
	if !strings.Contains(text, "tptctl_cmd") {
		t.Error("generated install.go no longer references tptctl_cmd at all - GetClientContext should still be called through it in Run")
	}
}
