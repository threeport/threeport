package v0

import (
	"errors"
	"fmt"

	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// ErrNoBackupBucket is returned when a restore has no bucket to restore from.
var ErrNoBackupBucket = errors.New("no backup bucket")

// RestoreBackupBucket returns the bucket a control plane's database is
// restored from: the one given, or otherwise the one recorded when the
// control plane was installed.
func RestoreBackupBucket(controlPlane *ControlPlane, givenBucket string) (string, error) {
	if givenBucket != "" {
		return givenBucket, nil
	}
	if controlPlane.Provider != v0.KubernetesRuntimeInfraProviderGKE {
		return "", fmt.Errorf(
			"%w: control plane %s runs on %s, and database backups are provisioned only for production control planes on %s",
			ErrNoBackupBucket, controlPlane.Name, controlPlane.Provider, v0.KubernetesRuntimeInfraProviderGKE,
		)
	}
	if controlPlane.GKEProviderConfig.BackupBucket == "" {
		return "", fmt.Errorf(
			"%w: none is recorded for control plane %s; pass the bucket with --bucket",
			ErrNoBackupBucket, controlPlane.Name,
		)
	}

	return controlPlane.GKEProviderConfig.BackupBucket, nil
}

// RestoreControlPlaneDatabase replaces a GKE control plane's database with
// one of its backups from sourceBucket.  The fresh full backup the restore
// takes goes to the bucket the control plane's scheduled backups go to,
// which is the one recorded at install, whichever bucket the backup came
// from.  It reaches the cluster through GKE rather than through the
// Threeport API, since a database that needs restoring may have taken the
// API down with it.
func RestoreControlPlaneDatabase(controlPlane *ControlPlane, sourceBucket, backup string) error {
	if controlPlane.Provider != v0.KubernetesRuntimeInfraProviderGKE {
		return fmt.Errorf(
			"restoring a backup is supported for control planes on %s, and %s runs on %s",
			v0.KubernetesRuntimeInfraProviderGKE, controlPlane.Name, controlPlane.Provider,
		)
	}

	kubeClient, err := gkeControlPlaneKubeClient(controlPlane)
	if err != nil {
		return err
	}

	cpi := threeport.NewInstaller()
	cpi.Opts.Namespace = threeport.ControlPlaneNamespace

	return cpi.RestoreDatabase(kubeClient, sourceBucket, controlPlane.GKEProviderConfig.BackupBucket, backup)
}

// RequireGkeLocation refuses an empty GCP project or region, which would
// otherwise be filled from the caller's current Google defaults and could
// name another cluster.
func RequireGkeLocation(projectID, region string) error {
	if projectID == "" || region == "" {
		return fmt.Errorf("the GCP project %q and region %q must both be known", projectID, region)
	}

	return nil
}

// gkeControlPlaneKubeClient returns a client for the GKE cluster a control
// plane runs on, authenticated as the caller's own Google credentials.
func gkeControlPlaneKubeClient(controlPlane *ControlPlane) (dynamic.Interface, error) {
	infra := (&provider.KubernetesRuntimeInfraGKE{
		PulumiWorkspace: provider.PulumiWorkspace{
			RuntimeInstanceName: provider.ThreeportRuntimeName(controlPlane.Name),
		},
		ProjectID: controlPlane.GKEProviderConfig.GcpProjectId,
		Region:    controlPlane.GKEProviderConfig.GcpRegion,
	}).InitPulumiProject()

	// the cluster is found by project and region, which are left to the
	// caller's current Google defaults when empty, so a config from an
	// install that did not record them reads them from the cluster's own
	// Pulumi stack instead
	if infra.ProjectID == "" || infra.Region == "" {
		if err := infra.LoadConfigFromStack(); err != nil {
			return nil, fmt.Errorf(
				"control plane %s records no GCP project and region, and they could not be read from its Pulumi stack: %w",
				controlPlane.Name, err,
			)
		}
	}
	if err := RequireGkeLocation(infra.ProjectID, infra.Region); err != nil {
		return nil, fmt.Errorf("cannot find the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	connection, err := infra.GetConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	kubeClient, _, err := kube.GetClient(
		&v0.KubernetesRuntimeInstance{
			ThreeportControlPlaneHost: util.Ptr(true),
			APIEndpoint:               &connection.APIEndpoint,
			CACertificate:             &connection.CACertificate,
			ConnectionToken:           &connection.Token,
			ConnectionTokenExpiration: &connection.TokenExpiration,
		},
		false,
		nil,
		"",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get a client for the GKE cluster of control plane %s: %w", controlPlane.Name, err)
	}

	return kubeClient, nil
}
