package version

import (
	"strings"
	"testing"
)

// TestGetVersionPrefersReleaseVersion covers GetVersion returning a stamped
// release version ahead of the embedded one.
func TestGetVersionPrefersReleaseVersion(t *testing.T) {
	// restore the unstamped default after the test
	t.Cleanup(func() { ReleaseVersion = "" })
	// stand in for a release build's -X stamp
	ReleaseVersion = "v9.9.9-rc.1"
	// the stamped version wins over the embedded one
	if got := GetVersion(); got != "v9.9.9-rc.1" {
		t.Errorf("GetVersion = %q, want the stamped v9.9.9-rc.1", got)
	}
}

// TestGetVersionFallsBackToEmbedded covers GetVersion returning the embedded
// version with its trailing newline trimmed when no release version is set.
func TestGetVersionFallsBackToEmbedded(t *testing.T) {
	// leave the release version unstamped
	ReleaseVersion = ""
	// the embedded version stands, trimmed
	got := GetVersion()
	// reject an empty version
	if got == "" {
		t.Fatal("GetVersion is empty, want the embedded version")
	}
	// reject a trailing line break
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("GetVersion = %q, want no line break", got)
	}
	// match the trimmed embed
	if want := strings.TrimSuffix(Version, "\n"); got != want {
		t.Errorf("GetVersion = %q, want the embedded %q", got, want)
	}
}
