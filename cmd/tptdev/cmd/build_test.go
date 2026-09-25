package cmd

import (
	"testing"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestImageBuildTarget covers the dockerfile target each component builds.
func TestImageBuildTarget(t *testing.T) {
	// compare each component to the target the release uses
	cases := []struct {
		name   string
		target string
	}{
		{installer.ThreeportTerraformControllerName, "release-terraform"},
		{installer.ThreeportOciControllerName, "release-pulumi"},
		{installer.ThreeportGcpControllerName, "release-pulumi"},
		{installer.ThreeportHelmWorkloadControllerName, "release-helm"},
		{"rest-api", "release"},
	}
	for _, tc := range cases {
		if got := imageBuildTarget(tc.name); got != tc.target {
			t.Errorf("imageBuildTarget(%s) = %s, want %s", tc.name, got, tc.target)
		}
	}
}
