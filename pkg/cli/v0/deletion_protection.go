package v0

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// teardownProtectionCheckTimeout bounds each call tptctl down --infra-only
// makes to read the cluster's deletion protection, so a control plane that is
// gone doesn't hold up the teardown.
var teardownProtectionCheckTimeout = 30 * time.Second

// checkTeardownDeletionProtection refuses to tear down the control plane's
// cluster while the EKS, GKE or OKE definition it was created from has
// DeletionProtection set.  instance is the control plane's runtime instance,
// read from its API; with infraOnly it is nil and read here, each call bounded
// by teardownProtectionCheckTimeout.
//
// With infraOnly, a control plane that can't be reached, or doesn't answer in
// time, may be gone, and the teardown is how a cluster without a working
// control plane is removed: the check is skipped, and skipped says why.  An
// API that answers with an error is running, so the check can't be skipped,
// and that is an error.
func checkTeardownDeletionProtection(
	apiClient *http.Client,
	apiEndpoint string,
	infraProvider string,
	instance *v0.KubernetesRuntimeInstance,
	infraOnly bool,
) (skipped error, err error) {
	if infraProvider == v0.KubernetesRuntimeInfraProviderKind {
		return nil, nil
	}

	checkClient := apiClient
	if infraOnly {
		boundedClient := *apiClient
		boundedClient.Timeout = teardownProtectionCheckTimeout
		checkClient = &boundedClient

		instance, err = client.GetThreeportControlPlaneKubernetesRuntimeInstance(checkClient, apiEndpoint)
		if err != nil {
			if apiUnreachable(err) {
				return err, nil
			}
			return nil, fmt.Errorf(
				"refusing to tear down the control plane's cluster: the Threeport API answered with an error, "+
					"so the cluster's deletion protection can't be checked. Fix the problem and run the command again: %w",
				err,
			)
		}
	}

	if err := clusterDeletionProtection(checkClient, apiEndpoint, infraProvider, instance); err != nil {
		var protected *v0.DeletionProtectedError
		if errors.As(err, &protected) {
			return nil, fmt.Errorf("refusing to tear down the control plane's cluster: %w, then run tptctl down again", err)
		}
		if infraOnly && apiUnreachable(err) {
			return err, nil
		}
		return nil, fmt.Errorf("refusing to tear down the control plane's cluster: failed to check its deletion protection: %w", err)
	}

	return nil, nil
}

// clusterDeletionProtection returns a *v0.DeletionProtectedError while the
// EKS, GKE or OKE definition of a runtime instance's cluster has
// DeletionProtection set.  A runtime with no provider instance recorded, as
// one whose install failed part way, has nothing that protects it.
func clusterDeletionProtection(
	apiClient *http.Client,
	apiEndpoint string,
	infraProvider string,
	instance *v0.KubernetesRuntimeInstance,
) error {
	var definition any
	var err error
	switch infraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		var providerInstance *v0.AwsEksKubernetesRuntimeInstance
		if providerInstance, err = client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient, apiEndpoint, *instance.ID); err == nil {
			definition, err = client.GetAwsEksKubernetesRuntimeDefinitionByID(apiClient, apiEndpoint, *providerInstance.AwsEksKubernetesRuntimeDefinitionID)
		}
	case v0.KubernetesRuntimeInfraProviderGKE:
		var providerInstance *v0.GcpGkeKubernetesRuntimeInstance
		if providerInstance, err = client.GetGcpGkeKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient, apiEndpoint, *instance.ID); err == nil {
			definition, err = client.GetGcpGkeKubernetesRuntimeDefinitionByID(apiClient, apiEndpoint, *providerInstance.GcpGkeKubernetesRuntimeDefinitionID)
		}
	case v0.KubernetesRuntimeInfraProviderOKE:
		var providerInstance *v0.OciOkeKubernetesRuntimeInstance
		if providerInstance, err = client.GetOciOkeKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient, apiEndpoint, *instance.ID); err == nil {
			definition, err = client.GetOciOkeKubernetesRuntimeDefinitionByID(apiClient, apiEndpoint, *providerInstance.OciOkeKubernetesRuntimeDefinitionID)
		}
	default:
		return nil
	}
	if err != nil {
		if errors.Is(err, client_lib.ErrObjectNotFound) {
			return nil
		}
		return err
	}

	return v0.ProviderDefinitionDeletionProtection(definition)
}

// apiUnreachable reports whether a call to the Threeport API failed without
// an answer: the connection failed or timed out.
func apiUnreachable(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr)
}
