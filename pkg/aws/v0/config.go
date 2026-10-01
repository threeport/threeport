package v0

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// AssumeRoleSessionDuration is how long the credentials from an assumed role
// are good for.
const AssumeRoleSessionDuration = time.Hour

// LoadAwsConfig loads an AWS config from the environment or a shared config
// profile, overriding the region when one is given.
func LoadAwsConfig(configProfile, region string) (*aws.Config, error) {
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		awsConfigOptions(configProfile, region)...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &awsConfig, nil
}

// LoadAwsConfigFromApiKeys returns an AWS config built from static API keys.
// When a role ARN is given the config assumes that role, with an external ID
// if the role requires one.
func LoadAwsConfigFromApiKeys(
	accessKeyId,
	secretAccessKey,
	region,
	roleArn,
	externalId string,
) (*aws.Config, error) {
	configOptions := awsConfigOptions("", region)
	if accessKeyId != "" && secretAccessKey != "" {
		configOptions = append(
			configOptions,
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(accessKeyId, secretAccessKey, ""),
			),
		)
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background(), configOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from static API keys: %w", err)
	}
	if roleArn == "" {
		return &awsConfig, nil
	}

	return assumeRole(awsConfig, configOptions, roleArn, externalId, 0)
}

// AssumeRole returns an AWS config holding temporary credentials for a role,
// obtained with the credentials in the config it is given.
func AssumeRole(awsConfig aws.Config, roleArn, region string) (*aws.Config, error) {
	return assumeRole(
		awsConfig,
		awsConfigOptions("", region),
		roleArn,
		"",
		AssumeRoleSessionDuration,
	)
}

// assumeRole loads a config whose credentials come from assuming a role.  A
// zero session duration leaves the SDK's default in place.
func assumeRole(
	awsConfig aws.Config,
	configOptions []func(*config.LoadOptions) error,
	roleArn string,
	externalId string,
	sessionDuration time.Duration,
) (*aws.Config, error) {
	assumeRoleProvider := stscreds.NewAssumeRoleProvider(
		sts.NewFromConfig(awsConfig),
		roleArn,
		func(assumeRoleOptions *stscreds.AssumeRoleOptions) {
			if externalId != "" {
				assumeRoleOptions.ExternalID = aws.String(externalId)
			}
			if sessionDuration != 0 {
				assumeRoleOptions.Duration = sessionDuration
			}
		},
	)

	assumedConfig, err := config.LoadDefaultConfig(
		context.Background(),
		append(configOptions, config.WithCredentialsProvider(assumeRoleProvider))...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for assumed role %s: %w", roleArn, err)
	}

	return &assumedConfig, nil
}

// awsConfigOptions returns the load options shared by every way of building a
// config.
func awsConfigOptions(configProfile, region string) []func(*config.LoadOptions) error {
	configOptions := []func(*config.LoadOptions) error{
		// a shared config profile may itself assume a role that requires MFA,
		// in which case the SDK needs somewhere to ask for the code
		config.WithAssumeRoleCredentialOptions(
			func(assumeRoleOptions *stscreds.AssumeRoleOptions) {
				assumeRoleOptions.TokenProvider = stscreds.StdinTokenProvider
			},
		),
	}
	if configProfile != "" {
		configOptions = append(configOptions, config.WithSharedConfigProfile(configProfile))
	}
	if region != "" {
		configOptions = append(configOptions, config.WithRegion(region))
	}

	return configOptions
}

// AwsProviderCredentials are the credentials an out-of-process tool such as
// the Pulumi AWS provider authenticates with.
//
// It holds the long-lived credentials and the role to assume rather than the
// already-assumed ones, because a tool running in its own process cannot be
// handed a refreshing credential chain - only values.  Assumed-role
// credentials are good for an hour, and provisioning an EKS cluster and its
// node group routinely takes longer than that, so freezing them produces a
// half-built cluster partway through.  Given the role instead, the tool
// assumes it itself and renews as it goes.
type AwsProviderCredentials struct {
	// AccessKeyId and SecretAccessKey are the long-lived credentials.  Both
	// empty means the tool resolves credentials from its own environment,
	// which is what a control plane running in EKS wants: there the pod is
	// authenticated through IRSA, and that identity refreshes on its own.
	AccessKeyId     string
	SecretAccessKey string

	// SessionToken is set when the credentials above are themselves
	// temporary.  They expire on their own schedule, which bounds how long an
	// operation using them can run.
	SessionToken string

	// Profile is a shared config profile the tool resolves credentials from,
	// used instead of the fields above when the credentials come from the
	// local AWS configuration rather than from stored API keys.  Naming the
	// profile rather than passing values is what lets the tool refresh an SSO
	// or credential_process session, which otherwise expires mid-operation
	// the same way an assumed role does.
	Profile string

	// AssumeRoleArn is the role the tool assumes, empty to use the
	// credentials directly.
	AssumeRoleArn string

	// ExternalId is required by roles whose trust policy asks for one.
	ExternalId string
}

// ProviderCredentialsFromProfile returns credentials that point an
// out-of-process tool at a shared config profile and a role to assume.  This
// is the bootstrap path, where credentials come from the local AWS
// configuration rather than from stored API keys.
//
// The profile is named rather than resolved here on purpose.  Resolving it
// would produce values frozen at that moment, and a profile backed by SSO or
// by a credential_process hands out credentials that live minutes - far less
// than provisioning a cluster takes.  Reading the profile itself, the tool
// renews them as it goes.
func ProviderCredentialsFromProfile(profile, assumeRoleArn string) AwsProviderCredentials {
	return AwsProviderCredentials{
		Profile:       profile,
		AssumeRoleArn: assumeRoleArn,
	}
}

// IamTags returns tags for an IAM resource, identifying it by name alongside
// any caller-supplied tags.
func IamTags(name string, customTags map[string]string) []types.Tag {
	tags := []types.Tag{{Key: aws.String("Name"), Value: aws.String(name)}}
	for key, value := range customTags {
		tags = append(tags, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}

	return tags
}
