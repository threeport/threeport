package v0

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// gcpProviderKeepsServiceAccount reports whether a GcpProvider was set up to
// leave its GCP service account alone, because it is managed outside Threeport.
// A provider with no setting predates the field and is cleaned up.
func gcpProviderKeepsServiceAccount(gcpProvider v0.GcpProvider) bool {
	return gcpProvider.SyncServiceAccount != nil && !*gcpProvider.SyncServiceAccount
}

// gcpProvidersToCleanUp names the GCP service accounts behind a set of
// GcpProviders. A provider with no name or project cannot have had one created,
// and one created with SyncServiceAccount false has a service account that is
// not Threeport's to remove.
func gcpProvidersToCleanUp(providers []v0.GcpProvider) []PendingGcpCleanup {
	var pending []PendingGcpCleanup
	for _, gcpProvider := range providers {
		if gcpProvider.Name == nil || gcpProvider.ProjectID == nil {
			continue
		}
		if gcpProviderKeepsServiceAccount(gcpProvider) {
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

// listPendingGcpCleanup lists the GcpProviders whose GCP service accounts
// teardown must clean up.
//
// A server that answers not found has no GcpProvider route - it predates GCP
// support - so no provider can exist and there is nothing to record. Any other
// failure stops teardown rather than being skipped: the providers live in the
// database that teardown is about to destroy, so a listing that was skipped is
// a service account, with its key and project roles, that nothing will ever find
// again. The way past a control plane that cannot be listed is --infra-only,
// which does not depend on it.
func listPendingGcpCleanup(
	list func() (*[]v0.GcpProvider, error),
) ([]PendingGcpCleanup, error) {
	providers, err := list()
	if errors.Is(err, client_lib.ErrObjectNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf(
			"failed to list gcp providers before teardown, so their GCP service accounts cannot be cleaned up: %w\n"+
				"to tear down without cleaning them up, re-run with --infra-only and remove any threeport-svc-* service accounts in GCP by hand",
			err,
		)
	}

	for _, gcpProvider := range *providers {
		if gcpProvider.Name != nil && gcpProviderKeepsServiceAccount(gcpProvider) {
			Info(fmt.Sprintf(
				"leaving the GCP service account for gcp provider %s in place: it was created with SyncServiceAccount false",
				*gcpProvider.Name,
			))
		}
	}

	return gcpProvidersToCleanUp(*providers), nil
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
	pending, err := listPendingGcpCleanup(func() (*[]v0.GcpProvider, error) {
		return client.GetGcpProviders(apiClient, apiEndpoint)
	})
	if err != nil {
		return err
	}
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

// markInfraDestroyed records on the control plane that its infrastructure, and
// every provider-specific step after it, is done. What is left - the GCP service
// accounts of its providers - needs neither the cluster nor the API, so a repeat
// of teardown can go straight to it. Without the mark a repeat would try to
// reach the API that is already gone.
func markInfraDestroyed(controlPlane *ControlPlane) error {
	if _, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.InfraDestroyed = true
	}); err != nil {
		return fmt.Errorf("failed to record that infrastructure was destroyed: %w", err)
	}

	return nil
}

// markGkeCleanupPending records that a GKE cluster is gone but its GCP service
// account is not, so the next teardown can retry the removal. It does nothing
// for any other error.
func markGkeCleanupPending(controlPlane *ControlPlane, deleteErr error) error {
	var cleanupErr *provider.GcpCleanupError
	if !errors.As(deleteErr, &cleanupErr) {
		return nil
	}

	_, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
		c.InfraDestroyed = true
		c.GKEProviderConfig.ServiceAccountCleanupPending = true
		if c.GKEProviderConfig.GcpProjectId == "" {
			c.GKEProviderConfig.GcpProjectId = cleanupErr.ProjectID
		}
	})
	if err != nil {
		return fmt.Errorf("failed to record that infrastructure was destroyed: %w", err)
	}

	return nil
}

// resumeTeardownCleanup finishes a teardown whose infrastructure is already
// destroyed: the GKE service account if its removal failed, then the service
// accounts of the control plane's gcp providers. deleteBackups is whether this
// run was asked to delete the database backups, as the first one may have been.
func resumeTeardownCleanup(controlPlane *ControlPlane, deleteBackups bool) error {
	if controlPlane.Provider == v0.KubernetesRuntimeInfraProviderGKE &&
		controlPlane.GKEProviderConfig.ServiceAccountCleanupPending {
		if err := resumeGkeCleanup(controlPlane, deleteBackups); err != nil {
			return err
		}
		if _, err := controlPlane.UpdateThreeportConfigInstance(func(c *ControlPlane) {
			c.GKEProviderConfig.ServiceAccountCleanupPending = false
		}); err != nil {
			return fmt.Errorf("failed to update threeport config: %w", err)
		}
	}

	return cleanUpPendingGcpProviders(controlPlane)
}

// resumeGkeInfra describes the GKE runtime whose cluster a previous teardown
// destroyed, with what its cleanup needs. The backup bucket is part of that: the
// service account is removed first, so a failure removing the database's access
// to its bucket, or the backups, leaves the retry to do it - and without the
// bucket the retry would skip it, report success and drop the config entry.
func resumeGkeInfra(controlPlane *ControlPlane, deleteBackups bool) *provider.KubernetesRuntimeInfraGKE {
	return &provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: provider.ThreeportRuntimeName(controlPlane.Name),
		},
		ProjectID:     controlPlane.GKEProviderConfig.GcpProjectId,
		Region:        controlPlane.GKEProviderConfig.GcpRegion,
		BackupBucket:  controlPlane.GKEProviderConfig.BackupBucket,
		DeleteBackups: deleteBackups,
	}
}

// resumeGkeCleanup removes the GCP service account and backup bucket access of a
// GKE runtime whose cluster a previous teardown destroyed.
func resumeGkeCleanup(controlPlane *ControlPlane, deleteBackups bool) error {
	infra := resumeGkeInfra(controlPlane, deleteBackups)
	if infra.ProjectID == "" {
		return fmt.Errorf(
			"no GCP project recorded for control plane %s; set GKEProviderConfig.GcpProjectId in the threeport config to the project the cluster was in",
			controlPlane.Name,
		)
	}
	if err := infra.InitPulumiProject().CleanUpGCPResources(); err != nil {
		return fmt.Errorf("failed to clean up GCP resources: %w", err)
	}

	return nil
}
