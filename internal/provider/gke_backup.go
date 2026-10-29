package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/storage/v1"
)

// A production control plane on GKE backs its database up to a bucket in
// another region.  The bucket is created with the GCP API rather than in the
// Pulumi stack so that destroying the stack on teardown leaves the backups in
// place.
//
// The database reaches the bucket as its own Kubernetes service account,
// through Workload Identity Federation for GKE, with no GCP service account
// or key.  The bucket names that account by its UID rather than by its
// namespace and name: every cluster in a project shares one workload identity
// pool, so a namespace and name grant would let any cluster in the project
// that creates the same namespace and account read the backups, while a UID
// is assigned by the cluster that owns the account and cannot be claimed by
// another.
const (
	// backupBucketNameFormat is the bucket name, from the runtime instance
	// name and the project ID.  Bucket names are global, so the project is
	// part of it.
	backupBucketNameFormat = "%s-backups-%s"

	// backupBucketNameMaxLength is the longest bucket name GCS accepts.
	backupBucketNameMaxLength = 63

	// kubeServiceAccountPrincipalFormat is the Workload Identity Federation
	// principal of one Kubernetes service account, by project number,
	// project ID and the account's UID.
	kubeServiceAccountPrincipalFormat = "principal://iam.googleapis.com/projects/%d/locations/global/workloadIdentityPools/%s.svc.id.goog/kubernetes.serviceaccount.uid/%s"

	// kubeServiceAccountPrincipalMarker is part of every principal that
	// names a Kubernetes service account by UID, in any project's pool.
	kubeServiceAccountPrincipalMarker = ".svc.id.goog/kubernetes.serviceaccount.uid/"
)

// backupBucketRoles are the roles the database holds on the bucket: writing
// backups and reading them back to list and restore them.  Neither can delete
// an object or change the bucket.
var backupBucketRoles = []string{
	"roles/storage.objectCreator",
	"roles/storage.objectViewer",
}

// defaultBackupRegions pairs a GKE region with the region its backups go to
// when none is given: another region on the same continent, so a backup
// survives the loss of the cluster's region without leaving its geography.
var defaultBackupRegions = map[string]string{
	"us-central1":             "us-east1",
	"us-east1":                "us-central1",
	"us-east4":                "us-central1",
	"us-east5":                "us-central1",
	"us-south1":               "us-central1",
	"us-west1":                "us-west2",
	"us-west2":                "us-west1",
	"us-west3":                "us-west1",
	"us-west4":                "us-west1",
	"northamerica-northeast1": "northamerica-northeast2",
	"northamerica-northeast2": "northamerica-northeast1",
	"southamerica-east1":      "southamerica-west1",
	"southamerica-west1":      "southamerica-east1",
	"europe-west1":            "europe-west4",
	"europe-west2":            "europe-west1",
	"europe-west3":            "europe-west4",
	"europe-west4":            "europe-west1",
	"europe-north1":           "europe-west4",
	"asia-east1":              "asia-southeast1",
	"asia-southeast1":         "asia-east1",
	"asia-northeast1":         "asia-northeast2",
	"asia-northeast2":         "asia-northeast1",
	"asia-south1":             "asia-south2",
	"asia-south2":             "asia-south1",
	"australia-southeast1":    "australia-southeast2",
	"australia-southeast2":    "australia-southeast1",
}

// GkeBackupRegion returns the region a control plane's backups go to: the
// one given when there is one, and otherwise the default for the cluster's
// region.  Either way it has to differ from the cluster's region, since a
// backup in the same region does not survive that region's loss.
func GkeBackupRegion(clusterRegion, givenRegion string) (string, error) {
	backupRegion := givenRegion
	if backupRegion == "" {
		var ok bool
		backupRegion, ok = defaultBackupRegions[clusterRegion]
		if !ok {
			return "", fmt.Errorf(
				"no default backup region for GKE region %q, choose one in another region with --gcp-backup-region",
				clusterRegion,
			)
		}
	}

	if strings.EqualFold(backupRegion, clusterRegion) {
		return "", fmt.Errorf(
			"backup region %q is the cluster's own region, choose another so backups survive the loss of the cluster's region",
			backupRegion,
		)
	}

	return backupRegion, nil
}

// GkeBackupBucketName returns the name of the bucket a control plane's
// backups go to.  A name longer than GCS accepts is shortened and ends in a
// hash of the full name, so two long names do not shorten to the same bucket.
func GkeBackupBucketName(runtimeInstanceName, projectID string) string {
	name := strings.ToLower(fmt.Sprintf(backupBucketNameFormat, runtimeInstanceName, projectID))
	if len(name) <= backupBucketNameMaxLength {
		return name
	}

	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:8]
	prefix := strings.TrimRight(name[:backupBucketNameMaxLength-len(suffix)-1], "-")

	return prefix + "-" + suffix
}

// backupClientOpts returns the options for a GCP client the backup resources
// are managed with, carrying the explicit credentials when there are some.
func (i *KubernetesRuntimeInfraGKE) backupClientOpts(scope string) []option.ClientOption {
	opts := []option.ClientOption{option.WithScopes(scope)}
	if i.ServiceAccountCredentials != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(i.ServiceAccountCredentials)))
	}

	return opts
}

// provisionBackups creates the backup bucket.  Access to it is granted once
// the database's Kubernetes service account exists, with
// GrantDatabaseBackupAccess.
func (i *KubernetesRuntimeInfraGKE) provisionBackups() error {
	ctx := context.Background()
	storageClientOpts := i.backupClientOpts(storage.DevstorageFullControlScope)
	storageService, err := storage.NewService(ctx, storageClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create storage service client: %w", err)
	}
	crmClientOpts := i.backupClientOpts(cloudresourcemanager.CloudPlatformScope)
	crmService, err := cloudresourcemanager.NewService(ctx, crmClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	return i.provisionBackupsWith(storageService, crmService)
}

// provisionBackupsWith creates the backup bucket with the given clients.
func (i *KubernetesRuntimeInfraGKE) provisionBackupsWith(
	storageService *storage.Service,
	crmService *cloudresourcemanager.Service,
) error {
	backupRegion, err := GkeBackupRegion(i.Region, i.BackupRegion)
	if err != nil {
		return err
	}
	bucketName := GkeBackupBucketName(i.RuntimeInstanceName, i.ProjectID)
	projectNumber, err := i.projectNumber(crmService)
	if err != nil {
		return err
	}

	if err := i.ensureBackupBucket(storageService, bucketName, backupRegion, projectNumber); err != nil {
		return err
	}
	i.BackupBucket = bucketName

	return nil
}

// GrantDatabaseBackupAccess lets the database's Kubernetes service account,
// named by its UID, write and read backups in the backup bucket.
func (i *KubernetesRuntimeInfraGKE) GrantDatabaseBackupAccess(kubeServiceAccountUID string) error {
	ctx := context.Background()

	storageClientOpts := i.backupClientOpts(storage.DevstorageFullControlScope)
	storageService, err := storage.NewService(ctx, storageClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create storage service client: %w", err)
	}
	crmClientOpts := i.backupClientOpts(cloudresourcemanager.CloudPlatformScope)
	crmService, err := cloudresourcemanager.NewService(ctx, crmClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	return i.grantDatabaseBackupAccessWith(storageService, crmService, kubeServiceAccountUID)
}

// grantDatabaseBackupAccessWith grants the database access with the given
// clients.
func (i *KubernetesRuntimeInfraGKE) grantDatabaseBackupAccessWith(
	storageService *storage.Service,
	crmService *cloudresourcemanager.Service,
	kubeServiceAccountUID string,
) error {
	if i.BackupBucket == "" {
		return errors.New("no backup bucket was provisioned to grant the database access to")
	}
	if kubeServiceAccountUID == "" {
		return errors.New("the database's Kubernetes service account has no UID to grant access to")
	}

	projectNumber, err := i.projectNumber(crmService)
	if err != nil {
		return err
	}

	// the database's whole contents go to the bucket granted here, so it is
	// confirmed to be this project's before the grant, not trusted by name
	bucket, err := storageService.Buckets.Get(i.BackupBucket).Do()
	if err != nil {
		return fmt.Errorf("failed to check backup bucket %s: %w", i.BackupBucket, err)
	}
	if err := i.requireBucketInProject(bucket, projectNumber); err != nil {
		return err
	}

	member := fmt.Sprintf(kubeServiceAccountPrincipalFormat, projectNumber, i.ProjectID, kubeServiceAccountUID)

	return grantBucketRoles(storageService, i.BackupBucket, member, backupBucketRoles)
}

// projectNumber returns the number of the control plane's GCP project.
func (i *KubernetesRuntimeInfraGKE) projectNumber(crmService *cloudresourcemanager.Service) (int64, error) {
	project, err := crmService.Projects.Get(i.ProjectID).Do()
	if err != nil {
		return 0, fmt.Errorf("failed to get the number of project %s: %w", i.ProjectID, err)
	}

	return project.ProjectNumber, nil
}

// requireBucketInProject refuses a bucket that belongs to another project
// than the control plane's.
//
// Bucket names are global, and a backup bucket's is predictable, so anyone
// can create one by that name in a project of their own, give it this
// control plane's labels, and let the installer's identity manage it.  Its
// owner keeps their access when the database is granted its own, so a bucket
// accepted by its name and labels alone would hand them every backup.  The
// project a bucket is in is the one thing they cannot copy.
func (i *KubernetesRuntimeInfraGKE) requireBucketInProject(bucket *storage.Bucket, projectNumber int64) error {
	if projectNumber <= 0 || bucket.ProjectNumber != uint64(projectNumber) {
		return fmt.Errorf(
			"GCS bucket %s belongs to project number %d, not to project %s (%d), so database backups will not be written to it",
			bucket.Name, bucket.ProjectNumber, i.ProjectID, projectNumber,
		)
	}

	return nil
}

// ensureBackupBucket creates the backup bucket, or reuses it when an earlier
// install of the same control plane created it, so a reinstalled control
// plane keeps the backups taken before it.  A bucket of the same name in
// another project, one that this control plane did not create, or one in
// another region than asked for, is refused.
func (i *KubernetesRuntimeInfraGKE) ensureBackupBucket(
	storageService *storage.Service,
	bucketName string,
	backupRegion string,
	projectNumber int64,
) error {
	existing, err := storageService.Buckets.Get(bucketName).Do()
	if err == nil {
		if err := i.requireBucketInProject(existing, projectNumber); err != nil {
			return err
		}
		if !bucketOwnedBy(existing, i.RuntimeInstanceName) {
			return fmt.Errorf("GCS bucket %s already exists and was not created for %s", bucketName, i.RuntimeInstanceName)
		}
		if !strings.EqualFold(existing.Location, backupRegion) {
			return fmt.Errorf(
				"backup bucket %s already exists in %s, not in the requested backup region %s",
				bucketName, strings.ToLower(existing.Location), backupRegion,
			)
		}
		return nil
	}
	if !isGoogleAPINotFound(err) {
		return fmt.Errorf("failed to check for backup bucket %s: %w", bucketName, err)
	}

	_, err = storageService.Buckets.Insert(i.ProjectID, &storage.Bucket{
		Name:         bucketName,
		Location:     backupRegion,
		StorageClass: "STANDARD",
		Labels:       GcpResourceLabels(i.RuntimeInstanceName),
		IamConfiguration: &storage.BucketIamConfiguration{
			UniformBucketLevelAccess: &storage.BucketIamConfigurationUniformBucketLevelAccess{Enabled: true},
			PublicAccessPrevention:   "enforced",
		},
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to create backup bucket %s: %w", bucketName, err)
	}

	return nil
}

// grantBucketRoles adds member to each of roles on a bucket.
func grantBucketRoles(storageService *storage.Service, bucketName, member string, roles []string) error {
	policy, err := storageService.Buckets.GetIamPolicy(bucketName).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy of bucket %s: %w", bucketName, err)
	}

	changed := false
	for _, role := range roles {
		var binding *storage.PolicyBindings
		for _, b := range policy.Bindings {
			if b.Role == role {
				binding = b
				break
			}
		}
		if binding == nil {
			binding = &storage.PolicyBindings{Role: role}
			policy.Bindings = append(policy.Bindings, binding)
		}
		if !containsString(binding.Members, member) {
			binding.Members = append(binding.Members, member)
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if _, err := storageService.Buckets.SetIamPolicy(bucketName, policy).Do(); err != nil {
		return fmt.Errorf("failed to set IAM policy of bucket %s: %w", bucketName, err)
	}

	return nil
}

// removeBucketMembers removes every member matching remove from every role on
// a bucket.
func removeBucketMembers(storageService *storage.Service, bucketName string, remove func(string) bool) error {
	policy, err := storageService.Buckets.GetIamPolicy(bucketName).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy of bucket %s: %w", bucketName, err)
	}

	changed := false
	bindings := make([]*storage.PolicyBindings, 0, len(policy.Bindings))
	for _, binding := range policy.Bindings {
		members := make([]string, 0, len(binding.Members))
		for _, m := range binding.Members {
			if remove(m) {
				changed = true
				continue
			}
			members = append(members, m)
		}
		binding.Members = members
		if len(members) > 0 {
			bindings = append(bindings, binding)
		}
	}
	if !changed {
		return nil
	}
	policy.Bindings = bindings

	if _, err := storageService.Buckets.SetIamPolicy(bucketName, policy).Do(); err != nil {
		return fmt.Errorf("failed to set IAM policy of bucket %s: %w", bucketName, err)
	}

	return nil
}

// deleteBackupResources removes every Kubernetes service account's access to
// the backup bucket recorded for this control plane.  The bucket and the
// backups in it are kept unless DeleteBackups is set, since they may still be
// needed after the control plane is gone.  Without a recorded bucket there is
// nothing to remove, which is the case for every runtime but a production
// control plane's.
func (i *KubernetesRuntimeInfraGKE) deleteBackupResources(storageService *storage.Service) error {
	if i.BackupBucket == "" {
		return nil
	}

	bucket, err := storageService.Buckets.Get(i.BackupBucket).Do()
	if isGoogleAPINotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check for backup bucket %s: %w", i.BackupBucket, err)
	}
	if !bucketOwnedBy(bucket, i.RuntimeInstanceName) {
		return fmt.Errorf("GCS bucket %s was not created for %s, leaving it in place", i.BackupBucket, i.RuntimeInstanceName)
	}

	if i.DeleteBackups {
		return deleteBackupBucket(storageService, i.BackupBucket, i.RuntimeInstanceName)
	}

	// the cluster, and with it the account's UID, may already be gone, so
	// every account principal the bucket names goes
	return removeBucketMembers(storageService, i.BackupBucket, func(member string) bool {
		return strings.Contains(member, kubeServiceAccountPrincipalMarker)
	})
}

// deleteBackupBucket deletes a backup bucket and every backup in it.  A
// bucket this control plane did not create is left alone.
func deleteBackupBucket(storageService *storage.Service, bucketName, ownerName string) error {
	bucket, err := storageService.Buckets.Get(bucketName).Do()
	if isGoogleAPINotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to check for backup bucket %s: %w", bucketName, err)
	}
	if !bucketOwnedBy(bucket, ownerName) {
		return fmt.Errorf("GCS bucket %s was not created for %s, leaving it in place", bucketName, ownerName)
	}

	// a bucket can only be deleted once it is empty
	if err := storageService.Objects.List(bucketName).Pages(context.Background(), func(objects *storage.Objects) error {
		for _, object := range objects.Items {
			if err := storageService.Objects.Delete(bucketName, object.Name).Do(); err != nil && !isGoogleAPINotFound(err) {
				return fmt.Errorf("failed to delete backup object %s: %w", object.Name, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to empty backup bucket %s: %w", bucketName, err)
	}

	if err := storageService.Buckets.Delete(bucketName).Do(); err != nil && !isGoogleAPINotFound(err) {
		return fmt.Errorf("failed to delete backup bucket %s: %w", bucketName, err)
	}

	return nil
}

// bucketOwnedBy reports whether a bucket carries the labels this owner puts
// on the resources it creates.
func bucketOwnedBy(bucket *storage.Bucket, ownerName string) bool {
	if bucket == nil {
		return false
	}
	for key, value := range GcpResourceLabels(ownerName) {
		if bucket.Labels[key] != value {
			return false
		}
	}

	return true
}

// isGoogleAPINotFound reports whether err is a 404 from a Google API.
func isGoogleAPINotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == 404
}

// containsString reports whether s holds value.
func containsString(s []string, value string) bool {
	for _, v := range s {
		if v == value {
			return true
		}
	}

	return false
}
