package v0

import (
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
