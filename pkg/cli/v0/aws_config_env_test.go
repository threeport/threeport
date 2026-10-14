package v0

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// TestAwsConfigProfile covers what --aws-config-env selects.
//
// The flag is documented as retrieving credentials from environment variables,
// and a named profile defeats that twice over: the SDK prefers the profile's
// credentials over the environment's, and a profile that is not on disk - the
// ordinary case in CI - fails the load outright rather than falling back.
// Since --aws-config-profile defaults to "default", one is always named unless
// the flag clears it.
func TestAwsConfigProfile(t *testing.T) {
	assert.Equal(
		t, "", awsConfigProfile(threeport.Options{AwsConfigEnv: true, AwsConfigProfile: "default"}),
		"--aws-config-env has to leave the profile unnamed or it does nothing",
	)
	assert.Equal(
		t, "some-profile", awsConfigProfile(threeport.Options{AwsConfigProfile: "some-profile"}),
		"without the flag the named profile is used",
	)
	assert.Equal(
		t, "", awsConfigProfile(threeport.Options{AwsConfigEnv: true, AwsConfigProfile: "some-profile"}),
		"the flag wins over an explicitly named profile, which is what asking for it means",
	)
}

// TestAwsConfigProfileIsTheOnlyReaderOfTheRawFlag covers the invariant the
// accessor exists to hold.
//
// Clearing the profile for LoadAwsConfig alone was not enough: the Pulumi AWS
// provider is handed a profile of its own, and the teardown reads one back out
// of the saved EKS provider config. Both still read the raw field, so IAM setup
// succeeded on environment credentials and Pulumi then failed to load a profile
// that was not there. The bug is a site reading Opts.AwsConfigProfile directly,
// and that is what this looks for - a behavioural test would have to reach AWS
// to see it.
func TestAwsConfigProfileIsTheOnlyReaderOfTheRawFlag(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	require.NoError(t, err)

	for _, name := range sources {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		require.NoError(t, err)

		for number, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, ".AwsConfigProfile") {
				continue
			}
			// the accessor itself, and the teardown, which reads the resolved
			// value back off the stored control plane config rather than the flag
			trimmed := strings.TrimSpace(line)
			switch {
			// the accessor itself
			case strings.Contains(line, "opts.AwsConfigProfile"):
				continue
			// the teardown reads the resolved value back off the stored
			// control plane config, not the flag
			case strings.Contains(line, "EKSProviderConfig.AwsConfigProfile"):
				continue
			// setting the flag from CLI args is how it gets there
			case strings.Contains(trimmed, "AwsConfigProfile = "):
				continue
			}
			t.Errorf(
				"%s:%d reads the raw flag instead of awsConfigProfile(): %s",
				name, number+1, trimmed,
			)
		}
	}
}
