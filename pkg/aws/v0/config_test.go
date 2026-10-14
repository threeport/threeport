package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSharedConfigFile writes an AWS shared config file holding two profiles
// and points the SDK at it for the duration of the test.
func writeSharedConfigFile(t *testing.T) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(configPath, []byte(`[default]
region = us-east-1

[profile other-account]
region = eu-west-1
`), 0600))

	t.Setenv("AWS_CONFIG_FILE", configPath)
	// keep anything in the environment from deciding the region instead
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
}

// TestLoadAwsConfigUsesProfileRegion checks a named shared config profile is
// honored.  tptctl passes the profile from --aws-config-profile, and loading
// the wrong one points the whole operation at the wrong account.
func TestLoadAwsConfigUsesProfileRegion(t *testing.T) {
	writeSharedConfigFile(t)

	awsConfig, err := LoadAwsConfig("other-account", "")
	require.NoError(t, err)
	assert.Equal(t, "eu-west-1", awsConfig.Region)
}

// TestLoadAwsConfigRegionOverridesProfile checks an explicit region wins over
// the profile's.
func TestLoadAwsConfigRegionOverridesProfile(t *testing.T) {
	writeSharedConfigFile(t)

	awsConfig, err := LoadAwsConfig("other-account", "us-west-2")
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", awsConfig.Region)
}

// TestLoadAwsConfigFromApiKeys checks the static credentials and region reach
// the config.  These are the decrypted keys off an AwsProvider, and falling
// back to ambient credentials instead would act as the wrong identity.
func TestLoadAwsConfigFromApiKeys(t *testing.T) {
	writeSharedConfigFile(t)

	awsConfig, err := LoadAwsConfigFromApiKeys(
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"us-west-2",
		"",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", awsConfig.Region)

	retrievedCredentials, err := awsConfig.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", retrievedCredentials.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", retrievedCredentials.SecretAccessKey)
}

// TestLoadAwsConfigFromApiKeysWithRole checks that supplying a role ARN
// replaces the static credentials with ones obtained by assuming the role.
// Leaving the static pair in place would silently act as the API key's own
// identity instead of the role's.
func TestLoadAwsConfigFromApiKeysWithRole(t *testing.T) {
	writeSharedConfigFile(t)

	awsConfig, err := LoadAwsConfigFromApiKeys(
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"us-west-2",
		"arn:aws:iam::123456789012:role/some-role",
		"external-id",
	)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", awsConfig.Region)

	assert.True(
		t,
		aws.IsCredentialsProvider(awsConfig.Credentials, &stscreds.AssumeRoleProvider{}),
		"credentials must come from assuming the role",
	)
	assert.False(
		t,
		aws.IsCredentialsProvider(awsConfig.Credentials, credentials.StaticCredentialsProvider{}),
		"the static API keys must not be the config's credentials",
	)
}

// TestAssumeRole checks the returned config's credentials come from the role
// rather than from the config the role was assumed with.
func TestAssumeRole(t *testing.T) {
	writeSharedConfigFile(t)

	userConfig, err := LoadAwsConfigFromApiKeys(
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"us-east-1",
		"",
		"",
	)
	require.NoError(t, err)

	roleConfig, err := AssumeRole(
		*userConfig,
		"arn:aws:iam::123456789012:role/resource-manager",
		"us-west-2",
	)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", roleConfig.Region)
	assert.True(
		t,
		aws.IsCredentialsProvider(roleConfig.Credentials, &stscreds.AssumeRoleProvider{}),
		"credentials must come from assuming the role",
	)
}

// TestIamTags checks IAM resources are tagged with their name alongside any
// caller-supplied tags.
func TestIamTags(t *testing.T) {
	tags := map[string]string{}
	for _, tag := range IamTags("some-role", map[string]string{"ProvisionedBy": "threeport"}) {
		tags[*tag.Key] = *tag.Value
	}

	assert.Equal(t, map[string]string{
		"Name":          "some-role",
		"ProvisionedBy": "threeport",
	}, tags)

	nameOnly := IamTags("some-role", map[string]string{})
	require.Len(t, nameOnly, 1)
	assert.Equal(t, "Name", *nameOnly[0].Key)
}
