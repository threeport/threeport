package v0

// these are constants used throughout the threeport codebase, and may in
// some cases be placed here to avoid import cycles if placed elsewhere
const (
	// this is used in internal/provider and pkg/threeport-installer to configure
	// the AWS role session name used for cross-account access to AWS resources
	AwsResourceManagerRoleSessionName = "threeport-control-plane"

	// used to query GetProviderRegionForLocation to determine the AWS region
	AwsProvider = "aws"
)
