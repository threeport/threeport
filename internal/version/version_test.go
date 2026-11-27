package version

import (
	"strings"
	"testing"
)

// setVersionAndCommit sets Version and Commit for a test and restores them
// when the test ends.
func setVersionAndCommit(t *testing.T, version, commit string) {
	t.Helper()

	previousVersion, previousCommit := Version, Commit
	t.Cleanup(func() { Version, Commit = previousVersion, previousCommit })
	Version, Commit = version, commit
}

// TestGetVersionWithoutCommitReturnsVersionFile covers RC and OSR builds,
// which never set a commit, reporting version.txt unchanged.
func TestGetVersionWithoutCommitReturnsVersionFile(t *testing.T) {
	for _, contents := range []string{"v0.7.0-rc.6", "v0.7.0-rc.6\n"} {
		setVersionAndCommit(t, contents, "")

		if got := GetVersion(); got != "v0.7.0-rc.6" {
			t.Errorf("GetVersion() with version.txt %q = %q, want v0.7.0-rc.6", contents, got)
		}
	}
}

// TestGetVersionWithCommitAppendsCommit covers a Head build reporting
// <version>.<commit>.
func TestGetVersionWithCommitAppendsCommit(t *testing.T) {
	for _, contents := range []string{"v0.7.0-rc.6", "v0.7.0-rc.6\n"} {
		setVersionAndCommit(t, contents, "a1b2c3d")

		if got := GetVersion(); got != "v0.7.0-rc.6.a1b2c3d" {
			t.Errorf("GetVersion() with version.txt %q = %q, want v0.7.0-rc.6.a1b2c3d", contents, got)
		}
	}
}

// TestVersionFileHasNoStrayWhitespace covers version.txt holding one token
// with at most a trailing newline. GetVersion trims one newline, HeadVersion
// trims all surrounding whitespace and head-images.yml strips every space, so
// they only agree on a file like that.
func TestVersionFileHasNoStrayWhitespace(t *testing.T) {
	// assert the embedded file has a version
	trimmed := strings.TrimSpace(Version)
	if trimmed == "" {
		t.Fatal("version.txt is empty")
	}

	// assert the only whitespace is one optional trailing newline
	if trimmed != strings.TrimSuffix(Version, "\n") {
		t.Errorf("version.txt %q has whitespace other than one trailing newline", Version)
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		t.Errorf("version.txt %q contains whitespace inside the version", Version)
	}
}
