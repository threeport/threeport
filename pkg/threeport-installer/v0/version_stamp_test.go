package v0

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/threeport/threeport/internal/version"
)

// stampedCommit is the commit the stamped test run links in.
const stampedCommit = "a1b2c3d"

// stampedChildEnv marks the stamped child run, so that a stamp that fails to
// take fails the child instead of starting another child.
const stampedChildEnv = "THREEPORT_TEST_STAMPED_CHILD"

// defaultImageTags returns the default image tag of every control plane
// component the installer defines, by component name.
func defaultImageTags() map[string]string {
	tags := map[string]string{
		ThreeportRestApi.Name: ThreeportRestApi.ImageTag,
		ThreeportAgent.Name:   ThreeportAgent.ImageTag,
		DatabaseMigrator.Name: DatabaseMigrator.ImageTag,
	}
	for _, controller := range ThreeportControllerList {
		tags[controller.Name] = controller.ImageTag
	}
	return tags
}

// TestDefaultImageTagsFollowVersion covers every component's default image
// tag being the version, so that a tptctl built with a Head commit defaults to
// the Head images without --control-plane-image-tag.
func TestDefaultImageTagsFollowVersion(t *testing.T) {
	want := version.GetVersion()

	for name, tag := range defaultImageTags() {
		if tag != want {
			t.Errorf("%s default ImageTag = %q, want the version %q", name, tag, want)
		}
	}
}

// TestDefaultImageTagsFollowStampedCommit covers the commit stamped at link
// time reaching these package-level defaults, which are evaluated when the
// package initializes. It runs itself as a child with the commit linked in,
// the way a Head build does, and checks the defaults there.
func TestDefaultImageTagsFollowStampedCommit(t *testing.T) {
	// in the child, the commit must be linked in: check the defaults
	if os.Getenv(stampedChildEnv) != "" {
		if version.Commit != stampedCommit {
			t.Fatalf("version.Commit = %q, want %q: the linker flag did not take", version.Commit, stampedCommit)
		}
		for name, tag := range defaultImageTags() {
			if !strings.HasSuffix(tag, "."+stampedCommit) || tag != version.GetVersion() {
				t.Errorf("%s default ImageTag = %q, want the version ending .%s", name, tag, stampedCommit)
			}
		}
		return
	}

	// in the parent, re-run this test with the commit linked in
	if testing.Short() {
		t.Skip("skipping the stamped build in short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	cmd := exec.Command(
		"go", "test", "-count=1", "-v",
		"-run", "^TestDefaultImageTagsFollowStampedCommit$",
		"-ldflags=-X=github.com/threeport/threeport/internal/version.Commit="+stampedCommit,
		".",
	)
	cmd.Env = append(os.Environ(), stampedChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stamped run failed: %v\n%s", err, out)
	}

	// the child must have run the check, not skipped it
	if !strings.Contains(string(out), "--- PASS: TestDefaultImageTagsFollowStampedCommit") {
		t.Fatalf("stamped run did not pass the check:\n%s", out)
	}
}
