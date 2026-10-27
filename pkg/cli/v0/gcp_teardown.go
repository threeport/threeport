package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// gcpProvidersToCleanUp names the GCP service accounts behind a set of
// GcpProviders. A provider with no name or project cannot have had one created.
func gcpProvidersToCleanUp(providers []v0.GcpProvider) []PendingGcpCleanup {
	var pending []PendingGcpCleanup
	for _, gcpProvider := range providers {
		if gcpProvider.Name == nil || gcpProvider.ProjectID == nil {
			continue
		}
		pending = append(pending, PendingGcpCleanup{
			Name:      *gcpProvider.Name,
			ProjectID: *gcpProvider.ProjectID,
		})
	}

	return pending
}

// runGcpCleanups removes each service account with cleanup and returns the ones
// that remain. An account that is not Threeport's is dropped with a warning:
// nothing here is entitled to remove it, so keeping it would block teardown
// forever. Any other failure keeps its entry for the next attempt.
func runGcpCleanups(
	pending []PendingGcpCleanup,
	cleanup func(projectID, name string) error,
	warn func(string),
) ([]PendingGcpCleanup, error) {
	var remaining []PendingGcpCleanup
	var errs []error
	for _, p := range pending {
		err := cleanup(p.ProjectID, p.Name)
		switch {
		case err == nil:
		case errors.Is(err, provider.ErrGcpServiceAccountNotOwned):
			warn(fmt.Sprintf("leaving GCP service account for provider %s in place: %v", p.Name, err))
		default:
			remaining = append(remaining, p)
			errs = append(errs, fmt.Errorf("provider %s in project %s: %w", p.Name, p.ProjectID, err))
		}
	}

	return remaining, errors.Join(errs...)
}

// recordPendingGcpCleanup lists the control plane's GcpProviders and saves what
// teardown must clean up to the local config. It runs before the control plane's
// database is destroyed, because afterwards nothing says which providers there
// were.
func recordPendingGcpCleanup(
	apiClient *http.Client,
	apiEndpoint string,
	controlPlane *ControlPlane,
) error {
	providers, err := client.GetGcpProviders(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to list gcp providers before teardown: %w", err)
	}

	pending := gcpProvidersToCleanUp(*providers)
	if len(pending) == 0 {
		return nil
	}

	if _, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.PendingGcpCleanup = mergePendingGcpCleanup(c.PendingGcpCleanup, pending)
	}); err != nil {
		return fmt.Errorf("failed to record gcp providers to clean up: %w", err)
	}

	return nil
}

// mergePendingGcpCleanup adds the entries of extra that existing lacks.
func mergePendingGcpCleanup(existing, extra []PendingGcpCleanup) []PendingGcpCleanup {
	merged := append([]PendingGcpCleanup(nil), existing...)
	for _, e := range extra {
		found := false
		for _, m := range merged {
			if m == e {
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, e)
		}
	}

	return merged
}

// cleanUpPendingGcpProviders removes the GCP service accounts recorded for the
// control plane and updates the config to hold only those that remain.
func cleanUpPendingGcpProviders(controlPlane *ControlPlane) error {
	if len(controlPlane.PendingGcpCleanup) == 0 {
		return nil
	}

	Info("Cleaning up GCP service accounts for gcp providers")
	remaining, cleanupErr := runGcpCleanups(
		controlPlane.PendingGcpCleanup,
		provider.DeleteGCPServiceAccountWithKey,
		Warning,
	)
	if _, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.PendingGcpCleanup = remaining
	}); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("failed to update threeport config: %w", err))
	}
	if cleanupErr != nil {
		return fmt.Errorf(
			"failed to clean up GCP service accounts, run teardown again to retry: %w",
			cleanupErr,
		)
	}

	return nil
}

// markGkeInfraDestroyed records on the control plane that its GKE cluster is
// gone but its GCP service account is not, so the next teardown can resume at
// the cleanup. It does nothing for any other error.
func markGkeInfraDestroyed(controlPlane *ControlPlane, deleteErr error) error {
	var cleanupErr *provider.GcpCleanupError
	if !errors.As(deleteErr, &cleanupErr) {
		return nil
	}

	_, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.GKEProviderConfig.InfraDestroyed = true
		if c.GKEProviderConfig.GcpProjectId == "" {
			c.GKEProviderConfig.GcpProjectId = cleanupErr.ProjectID
		}
	})
	if err != nil {
		return fmt.Errorf("failed to record that infrastructure was destroyed: %w", err)
	}

	return nil
}

// resumeGkeCleanup removes the GCP service account of a GKE runtime whose
// cluster a previous teardown destroyed.
func resumeGkeCleanup(controlPlane *ControlPlane) error {
	infra := &provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: provider.ThreeportRuntimeName(controlPlane.Name),
		},
		ProjectID: controlPlane.GKEProviderConfig.GcpProjectId,
		Region:    controlPlane.GKEProviderConfig.GcpRegion,
	}
	if infra.ProjectID == "" {
		return fmt.Errorf(
			"no GCP project recorded for control plane %s; set GKEProviderConfig.GcpProjectId in the threeport config to the project the cluster was in",
			controlPlane.Name,
		)
	}
	if err := infra.InitPulumiProject().CleanUpGCPResources(); err != nil {
		return fmt.Errorf("failed to clean up GCP service account: %w", err)
	}

	return nil
}
