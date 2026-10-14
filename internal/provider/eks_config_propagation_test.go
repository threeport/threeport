package provider

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

// fillAwsConfig stands in for what CreateEKSControlPlaneInfra does to the
// configs its caller hands it.
func fillAwsConfig(target *aws.Config, loaded *aws.Config) {
	*target = *loaded
}

// rebindAwsConfig is the shape the code had before: assigning to the
// parameter, which rebinds a local copy of the pointer and leaves the
// caller's config untouched.
func rebindAwsConfig(target *aws.Config, loaded *aws.Config) {
	target = loaded
	_ = target
}

// TestAwsConfigMustBeWrittenThroughPointer documents why the EKS bootstrap
// writes through its config pointers instead of assigning to them.
//
// The caller passes a zero aws.Config for the bootstrap to fill in, and later
// steps read it back: the resource manager trust policy update, the cluster's
// OIDC issuer lookup, and the teardown that runs when a step fails.  Assigning
// to the parameter rebinds only the local pointer, so those steps got a config
// with no region and failed with "Invalid Configuration: Missing Region" -
// including the teardown, which then left IAM behind.
func TestAwsConfigMustBeWrittenThroughPointer(t *testing.T) {
	loaded := &aws.Config{Region: "us-east-1"}

	callerConfig := aws.Config{}
	fillAwsConfig(&callerConfig, loaded)
	assert.Equal(
		t,
		"us-east-1",
		callerConfig.Region,
		"writing through the pointer must reach the caller's config",
	)

	unreachedConfig := aws.Config{}
	rebindAwsConfig(&unreachedConfig, loaded)
	assert.Empty(
		t,
		unreachedConfig.Region,
		"rebinding the parameter leaves the caller holding an empty config",
	)
}
