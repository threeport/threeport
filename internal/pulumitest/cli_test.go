package pulumitest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// outcomeTB records whether RequireCLI skipped or failed the test, and stops
// the helper the way the testing package would.
type outcomeTB struct {
	testing.TB
	skipped bool
	failed  bool
}

func (o *outcomeTB) Helper() {}

func (o *outcomeTB) Skip(...any) {
	o.skipped = true
	runtime.Goexit()
}

func (o *outcomeTB) Fatalf(string, ...any) {
	o.failed = true
	runtime.Goexit()
}

// requireCLI runs RequireCLI on its own goroutine so the Goexit that ends a
// skip or a failure ends that goroutine rather than this test.
func requireCLI(t *testing.T) *outcomeTB {
	o := &outcomeTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		RequireCLI(o)
	}()
	<-done
	return o
}

// TestAMissingCLIIsOnlyAFailureWhenCIAsksForIt covers #715: without the CLI,
// a contributor's run skips the tests that need it, and a run with
// THREEPORT_REQUIRE_PULUMI=1 fails them. With the CLI, both run.
func TestAMissingCLIIsOnlyAFailureWhenCIAsksForIt(t *testing.T) {
	withCLI := t.TempDir()
	cli := filepath.Join(withCLI, "pulumi")
	if runtime.GOOS == "windows" {
		cli += ".exe"
	}
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		require string
		skipped bool
		failed  bool
	}{
		{"no CLI on a contributor's machine", t.TempDir(), "", true, false},
		{"no CLI in CI", t.TempDir(), "1", false, true},
		{"no CLI with the variable set to something else", t.TempDir(), "true", true, false},
		{"the CLI on a contributor's machine", withCLI, "", false, false},
		{"the CLI in CI", withCLI, "1", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("PATH", c.path)
			t.Setenv(RequireCLIEnv, c.require)

			o := requireCLI(t)
			assert.Equal(t, c.skipped, o.skipped, "skipped")
			assert.Equal(t, c.failed, o.failed, "failed")
		})
	}
}
