package v0

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	tpaws "github.com/threeport/threeport/pkg/aws/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// ErrNoBackupBucket is returned when a restore has no bucket to restore from.
var ErrNoBackupBucket = errors.New("no backup bucket")

// BackupWriters is the caller's choice for the writers after a backup returns.
// The zero value leaves the writers stopped.
type BackupWriters bool

const (
	// ResumeWriters brings the stopped writers back up before the call returns.
	ResumeWriters BackupWriters = true

	// LeaveWritersStopped leaves the writers down and returns the counts.
	LeaveWritersStopped BackupWriters = false
)

// RestoreBackupBucket returns the bucket a control plane's database is
// restored from: the one given, or otherwise the one recorded when the
// control plane was installed.
func RestoreBackupBucket(controlPlane *ControlPlane, givenBucket string) (string, error) {
	if givenBucket != "" {
		return givenBucket, nil
	}
	// use the bucket recorded for this provider
	bucket, supported := recordedBackupBucket(controlPlane)
	if !supported {
		return "", fmt.Errorf(
			"%w: database backups are not provisioned for control planes on %s",
			ErrNoBackupBucket, controlPlane.Provider,
		)
	}
	if bucket == "" {
		return "", fmt.Errorf(
			"%w: none is recorded for control plane %s; pass the bucket with --bucket",
			ErrNoBackupBucket, controlPlane.Name,
		)
	}

	return bucket, nil
}

// ScheduledBackupBucket returns the bucket a control plane's scheduled
// backups go to, recorded when it was installed, or empty when it has none.
// An install that sets it up again, such as a reinstall, passes it on so
// the database keeps creating its backup schedule.
func ScheduledBackupBucket(controlPlane *ControlPlane) string {
	// an unsupported provider records no bucket
	bucket, _ := recordedBackupBucket(controlPlane)
	return bucket
}

// recordedBackupBucket returns the backup bucket recorded for a control
// plane and whether its provider provisions database backups.
func recordedBackupBucket(controlPlane *ControlPlane) (string, bool) {
	// return the bucket this provider records
	switch controlPlane.Provider {
	case v0.KubernetesRuntimeInfraProviderGKE:
		return gkeRecordedBackupBucket(controlPlane), true
	default:
		return "", false
	}
}

// controlPlaneKubeClient returns a client for the cluster a control plane runs on.
// The caller authenticates with that provider's own credentials.
func controlPlaneKubeClient(controlPlane *ControlPlane) (dynamic.Interface, error) {
	// use the client for this provider
	switch controlPlane.Provider {
	case v0.KubernetesRuntimeInfraProviderGKE:
		return gkeControlPlaneKubeClient(controlPlane)
	case v0.KubernetesRuntimeInfraProviderEKS:
		return eksControlPlaneKubeClient(controlPlane)
	case v0.KubernetesRuntimeInfraProviderOKE:
		return okeControlPlaneKubeClient(controlPlane)
	default:
		return nil, fmt.Errorf(
			"a database backup is not supported for control planes on %s",
			controlPlane.Provider,
		)
	}
}

// eksControlPlaneKubeClient returns a client for the EKS cluster, refreshing the token on each use.
func eksControlPlaneKubeClient(controlPlane *ControlPlane) (dynamic.Interface, error) {
	// load the caller's AWS credentials
	cfg := controlPlane.EKSProviderConfig
	awsConfig, err := tpaws.LoadAwsConfig(controlPlane.EKSProviderConfig.AwsConfigProfile, cfg.AwsRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	infra := &provider.KubernetesRuntimeInfraEKS{
		PulumiWorkspace: provider.PulumiWorkspace{RuntimeInstanceName: controlPlane.Name},
		AwsConfig:       awsConfig,
		AwsAccountID:    cfg.AwsAccountID,
		Region:          cfg.AwsRegion,
	}

	// mint a fresh token whenever the client needs one
	connection, err := infra.GetConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to EKS: %w", err)
	}
	return refreshingKubeClient(connection, func() (string, error) {
		refreshed, err := infra.GetConnection()
		if err != nil {
			return "", err
		}
		return refreshed.Token, nil
	})
}

// okeControlPlaneKubeClient returns a client for the OKE cluster, refreshing the token on each use.
func okeControlPlaneKubeClient(controlPlane *ControlPlane) (dynamic.Interface, error) {
	// load the caller's OCI credentials
	cfg := controlPlane.OKEProviderConfig
	infra := &provider.KubernetesRuntimeInfraOKE{
		PulumiWorkspace: provider.PulumiWorkspace{RuntimeInstanceName: controlPlane.Name},
		Region:          cfg.OciRegion,
		CompartmentOCID: cfg.OciCompartmentOcid,
	}
	if err := infra.LoadOCIConfig(cfg.OciRegion, cfg.OciConfigProfile, cfg.OciCompartmentOcid); err != nil {
		return nil, fmt.Errorf("failed to load OCI config: %w", err)
	}

	// mint a fresh token whenever the client needs one
	connection, err := infra.GetConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OKE: %w", err)
	}
	return refreshingKubeClient(connection, func() (string, error) {
		refreshed, err := infra.GetConnection()
		if err != nil {
			return "", err
		}
		return refreshed.Token, nil
	})
}

// RestoreControlPlaneDatabase restores a control plane database from sourceBucket.
// The fresh backup goes to the recorded schedule bucket, and the call uses the provider credentials because the API can be down.
func RestoreControlPlaneDatabase(controlPlane *ControlPlane, sourceBucket, backup, namespace, fromNamespace string, stopControllers bool) error {
	// a provider with no database backup is refused before the cluster is touched
	if controlPlane.Provider != v0.KubernetesRuntimeInfraProviderGKE {
		return fmt.Errorf(
			"a database backup is not supported for control planes on %s",
			controlPlane.Provider,
		)
	}

	// reach the cluster for this provider, since the API can be down
	kubeClient, err := controlPlaneKubeClient(controlPlane)
	if err != nil {
		return err
	}

	return RestoreControlPlaneDatabaseWithClient(
		controlPlane, sourceBucket, backup, namespace, fromNamespace, stopControllers, kubeClient,
	)
}

// RestoreControlPlaneDatabaseWithClient restores a database with a cluster client the caller already has.
func RestoreControlPlaneDatabaseWithClient(
	controlPlane *ControlPlane,
	sourceBucket, backup, namespace, fromNamespace string,
	stopControllers bool,
	kubeClient dynamic.Interface,
) error {
	// honor a namespace override, the source collection, and whether controllers stay scaled down
	cpi := threeport.NewInstaller()
	cpi.Opts.Namespace = threeport.ControlPlaneNamespace
	if namespace != "" {
		cpi.Opts.Namespace = namespace
	}
	cpi.Opts.FromNamespace = fromNamespace
	cpi.Opts.RestoreControllersStopped = stopControllers

	// use the bucket this provider records for scheduled backups
	scheduleBucket, _ := recordedBackupBucket(controlPlane)

	// replace the database from the named backup
	return cpi.RestoreDatabase(kubeClient, sourceBucket, scheduleBucket, backup)
}

// BackupControlPlane scales registered modules and the control plane down, then backs up the database.
// Module names are read from the API before that API is scaled down.
func BackupControlPlane(
	controlPlane *ControlPlane,
	namespace, givenBucket string,
	apiClient *http.Client,
	apiEndpoint string,
	writers BackupWriters,
) error {
	// resolve the bucket before any deployment is scaled down
	bucket, err := RestoreBackupBucket(controlPlane, givenBucket)
	if err != nil {
		return err
	}

	// reach the cluster for this provider before the API is scaled down
	kubeClient, err := controlPlaneKubeClient(controlPlane)
	if err != nil {
		return err
	}

	// back up, then resume the writers when the caller asked
	_, _, err = BackupControlPlaneWithClient(
		namespace, bucket, apiClient, apiEndpoint, kubeClient, writers,
	)
	return err
}

// BackupControlPlaneWithClient backs up a control plane with a cluster client the caller already has.
// It returns the writer counts it stopped, and brings them back up when writers says to.
func BackupControlPlaneWithClient(
	namespace, bucket string,
	apiClient *http.Client,
	apiEndpoint string,
	kubeClient dynamic.Interface,
	writers BackupWriters,
) (map[string]int64, []threeport.ModuleDeploymentScale, error) {
	cpi := threeport.NewInstaller()
	if namespace != "" {
		cpi.Opts.Namespace = namespace
	}
	replicas, scales, err := cpi.StopWritersAndBackup(kubeClient, apiClient, apiEndpoint, bucket)
	if err != nil && replicas == nil && scales == nil {
		return nil, nil, err
	}
	return finishBackup(cpi, kubeClient, replicas, scales, writers, err)
}

// finishBackup returns the stopped counts, bringing the writers back up when asked.
func finishBackup(
	cpi *threeport.ControlPlaneInstaller,
	kubeClient dynamic.Interface,
	replicas map[string]int64,
	modules []threeport.ModuleDeploymentScale,
	writers BackupWriters,
	backupErr error,
) (map[string]int64, []threeport.ModuleDeploymentScale, error) {
	if writers != ResumeWriters {
		return replicas, modules, backupErr
	}

	// bring the writers back up
	if err := cpi.ResumeWriters(kubeClient, replicas, modules); err != nil {
		if backupErr != nil {
			return replicas, modules, fmt.Errorf("%w (resuming writers: %v)", backupErr, err)
		}
		return replicas, modules, err
	}
	fmt.Println("Info: writers resumed")
	return replicas, modules, backupErr
}

// refreshingKubeClient returns a client for the cluster a connection names
// that asks tokenGenerator for a token on every request, rather than keeping
// the one the connection was made with until it expires.
func refreshingKubeClient(connection *kube.KubeConnectionInfo, tokenGenerator func() (string, error)) (dynamic.Interface, error) {
	kubeClient, _, err := kube.GetClientWithTokenRefresh(
		&v0.KubernetesRuntimeInstance{
			APIEndpoint:   &connection.APIEndpoint,
			CACertificate: &connection.CACertificate,
		},
		tokenGenerator,
	)

	return kubeClient, err
}

// RestoringNamedControlPlane reports whether namespace is the control plane named by name.
// An empty namespace is the default control plane namespace.
func RestoringNamedControlPlane(name, namespace string) bool {
	// compare the namespace to the control plane name
	if namespace == "" ||
		namespace == threeport.ControlPlaneNamespace {
		return true
	}
	return namespace == name
}

// ControlPlaneForEndpoint returns the configured control plane that serves apiEndpoint.
func ControlPlaneForEndpoint(cfg *ThreeportConfig, apiEndpoint string) *ControlPlane {
	if cfg == nil {
		return nil
	}
	for i := range cfg.ControlPlanes {
		if cfg.ControlPlanes[i].APIServer == apiEndpoint {
			return &cfg.ControlPlanes[i]
		}
	}
	return nil
}

// FillClusterLocation copies the GKE project, region, cluster, and backup bucket
// from the control plane that serves apiEndpoint when the entry records none.
func FillClusterLocation(cfg *ThreeportConfig, apiEndpoint string, entry *ControlPlane) {
	// keep a location the entry already records
	if entry == nil ||
		entry.GKEProviderConfig.GcpProjectId != "" ||
		entry.GKEProviderConfig.GcpRegion != "" ||
		entry.GKEProviderConfig.ClusterName != "" {
		return
	}

	// copy the location from the control plane that served the API
	host := ControlPlaneForEndpoint(cfg, apiEndpoint)
	if host == nil ||
		host.Name == entry.Name {
		return
	}
	if entry.Provider == "" {
		entry.Provider = host.Provider
	}
	entry.GKEProviderConfig = host.GKEProviderConfig
}

// FillEncryptionKey reads the encryption key into entry when the entry has none.
func FillEncryptionKey(
	cfg *ThreeportConfig,
	apiEndpoint string,
	entry *ControlPlane,
	namespace string,
) error {
	// keep a key the entry already records
	if entry == nil ||
		entry.EncryptionKey != "" {
		return nil
	}
	if namespace == "" {
		namespace = entry.Name
	}

	// read the key from the cluster that can reach the namespace
	reader := entry
	if reader.GKEProviderConfig.GcpProjectId == "" {
		if host := ControlPlaneForEndpoint(cfg, apiEndpoint); host != nil {
			reader = host
		}
	}
	key, err := EncryptionKeyInNamespace(reader, namespace)
	if err != nil {
		return fmt.Errorf(
			"failed to read the encryption key for control plane %s: %w",
			entry.Name,
			err,
		)
	}
	entry.EncryptionKey = key
	return nil
}

// EncryptionKeyInNamespace reads the encryption key secret in namespace.
func EncryptionKeyInNamespace(controlPlane *ControlPlane, namespace string) (string, error) {
	// open the cluster that holds the control plane
	kubeClient, err := controlPlaneKubeClient(controlPlane)
	if err != nil {
		return "", err
	}

	return encryptionKeyFromClient(kubeClient, namespace)
}

// encryptionKeyFromClient reads the encryption key secret through kubeClient.
func encryptionKeyFromClient(kubeClient dynamic.Interface, namespace string) (string, error) {
	// read the encryption key secret
	secret, err := kubeClient.Resource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}).Namespace(namespace).Get(context.Background(), threeport.EncryptionKeySecret, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to load encryption key secret: %w", err)
	}

	// decode the key the API stores
	keyB64, _, err := unstructured.NestedString(secret.Object, "data", encryption.KeyEnvVar)
	if err != nil {
		return "", fmt.Errorf("failed to load encryption key secret: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("failed to load encryption key secret: %w", err)
	}
	return string(key), nil
}

// RecordRestoredParent stores the encryption key for the restored control plane and makes it current.
func RecordRestoredParent(cfg *ThreeportConfig, name, namespace string) error {
	// load the entry written when the control plane was installed
	entry, err := cfg.GetControlPlaneConfig(name)
	if err != nil {
		return err
	}
	if namespace == "" {
		namespace = threeport.ControlPlaneNamespace
	}

	// store the key the restored database uses and make this control plane current
	if err := FillEncryptionKey(
		cfg,
		entry.APIServer,
		entry,
		namespace,
	); err != nil {
		return err
	}
	return UpdateThreeportConfig(cfg, entry)
}
