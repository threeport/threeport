package v0

import (
	"fmt"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// these are constants used throughout the threeport codebase, and may in
// some cases be placed here to avoid import cycles if placed elsewhere
const (
	// this is used in internal/provider and pkg/threeport-installer to configure
	// the AWS role session name used for cross-account access to AWS resources
	AwsResourceManagerRoleSessionName = "threeport-control-plane"

	// used to query GetProviderRegionForLocation to determine the AWS region
	AwsProvider = "aws"

	// namespace used by the gateway system
	GatewaySystemNamespace = "nukleros-gateway-system"
)

// GetCloudProviderForInfraProvider returns the cloud provider for a given
// infrastructure provider.
func GetCloudProviderForInfraProvider(input string) (string, error) {
	switch input {
	case v0.KubernetesRuntimeInfraProviderEKS:
		return AwsProvider, nil
	case v0.KubernetesRuntimeInfraProviderKind:
		return AwsProvider, nil // default to AWS values for testing purposes
	default:
		return "", fmt.Errorf("failed to get provider, infra provider %s not supported", input)
	}
}
