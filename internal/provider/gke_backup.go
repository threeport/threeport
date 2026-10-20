package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/storage/v1"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// The resources a production control plane on GKE backs its database up with:
// a bucket in another region and a service account that can write to it and
// nothing else.  They are created with the GCP API rather than in the Pulumi
// stack so that destroying the stack on teardown leaves the backups in place.
const (
	// backupBucketNameFormat is the bucket name, from the runtime instance
	// name and the project ID.  Bucket names are global, so the project is
	// part of it.
	backupBucketNameFormat = "%s-backups-%s"

	// backupBucketNameMaxLength is the longest bucket name GCS accepts.
	backupBucketNameMaxLength = 63

	// backupServiceAccountNameFormat is the backup service account ID, from
	// the runtime instance name.  It is as long as serviceAccountNameFormat,
	// so a runtime name that fits one fits the other.
	backupServiceAccountNameFormat = "threeport-bkp-%s"

	// backupServiceAccountDisplayFormat is the backup service account's
	// display name.
	backupServiceAccountDisplayFormat = "Threeport database backups for %s"
)

// backupBucketRoles are the roles the backup service account holds on the
// bucket: writing backups and reading them back to list and restore them.
// Neither can delete an object or change the bucket.
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

// backupServiceAccountID returns the backup service account's ID.
func (i *KubernetesRuntimeInfraGKE) backupServiceAccountID() string {
	return formatServiceAccountID(backupServiceAccountNameFormat, i.RuntimeInstanceName)
}

// expectedBackupServiceAccountEmail returns the backup service account's
// email, derived from its ID.
func (i *KubernetesRuntimeInfraGKE) expectedBackupServiceAccountEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", i.backupServiceAccountID(), i.ProjectID)
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

// provisionBackups creates the backup bucket and service account, grants the
// account access to the bucket and lets the database's Kubernetes service
// account act as it.  It runs after the cluster is created, since the
// workload identity pool a binding names exists only once there is a cluster.
func (i *KubernetesRuntimeInfraGKE) provisionBackups() error {
	ctx := context.Background()

	storageClientOpts := i.backupClientOpts(storage.DevstorageFullControlScope)
	storageService, err := storage.NewService(ctx, storageClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create storage service client: %w", err)
	}
	iamClientOpts := i.backupClientOpts(iam.CloudPlatformScope)
	iamService, err := iam.NewService(ctx, iamClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	return i.provisionBackupsWith(storageService, iamService)
}

// provisionBackupsWith provisions the backup resources with the given clients.
func (i *KubernetesRuntimeInfraGKE) provisionBackupsWith(
	storageService *storage.Service,
	iamService *iam.Service,
) error {
	backupRegion, err := GkeBackupRegion(i.Region, i.BackupRegion)
	if err != nil {
		return err
	}
	bucketName := GkeBackupBucketName(i.RuntimeInstanceName, i.ProjectID)

	if err := i.ensureBackupBucket(storageService, bucketName, backupRegion); err != nil {
		return err
	}

	account, existed, err := createServiceAccountForProject(
		iamService,
		i.ProjectID,
		i.backupServiceAccountID(),
		fmt.Sprintf(backupServiceAccountDisplayFormat, i.RuntimeInstanceName),
		backupServiceAccountDescription(i.RuntimeInstanceName),
	)
	if err != nil {
		return fmt.Errorf("failed to create backup service account: %w", err)
	}
	if err := refuseUnownedExistingGCPServiceAccount(account, existed, i.RuntimeInstanceName); err != nil {
		return err
	}

	// a service account just created is not yet known to every GCP API, so
	// granting it access is retried until it is
	if err := retryWhileServiceAccountPropagates(func() error {
		return grantBucketRoles(storageService, bucketName, "serviceAccount:"+account.Email, backupBucketRoles)
	}); err != nil {
		return err
	}

	if err := retryWhileServiceAccountPropagates(func() error {
		return addWorkloadIdentityMembers(
			iamService,
			i.ProjectID,
			account.Email,
			[]string{databaseWorkloadIdentityMember(i.ProjectID)},
		)
	}); err != nil {
		return fmt.Errorf("failed to let the database act as the backup service account: %w", err)
	}

	i.BackupBucket = bucketName
	i.BackupServiceAccountEmail = account.Email

	return nil
}

// serviceAccountPropagationAttempts and serviceAccountPropagationDelay bound
// how long access for a new service account is retried: two minutes.
var (
	serviceAccountPropagationAttempts = 24
	serviceAccountPropagationDelay    = 5 * time.Second
)

// retryWhileServiceAccountPropagates runs f until it succeeds, retrying while
// the failure says a service account does not exist yet.  Any other failure
// is returned at once.
func retryWhileServiceAccountPropagates(f func() error) error {
	var err error
	for attempt := 1; attempt <= serviceAccountPropagationAttempts; attempt++ {
		err = f()
		if err == nil || !isServiceAccountNotPropagated(err) {
			return err
		}
		if attempt < serviceAccountPropagationAttempts {
			time.Sleep(serviceAccountPropagationDelay)
		}
	}

	return fmt.Errorf("service account did not become available: %w", err)
}

// isServiceAccountNotPropagated reports whether err is a Google API saying a
// service account does not exist: a 404 for the account itself, or the 400
// Cloud Storage returns for a policy that names it.
func isServiceAccountNotPropagated(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}

	return apiErr.Code == 404 ||
		(apiErr.Code == 400 && strings.Contains(apiErr.Message, "does not exist"))
}

// ensureBackupBucket creates the backup bucket, or reuses it when an earlier
// install of the same control plane created it, so a reinstalled control
// plane keeps the backups taken before it.  A bucket of the same name that
// this control plane did not create, or one in another region than asked
// for, is refused.
func (i *KubernetesRuntimeInfraGKE) ensureBackupBucket(
	storageService *storage.Service,
	bucketName string,
	backupRegion string,
) error {
	existing, err := storageService.Buckets.Get(bucketName).Do()
	if err == nil {
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

// removeBucketMember removes member from every role on a bucket.
func removeBucketMember(storageService *storage.Service, bucketName, member string) error {
	policy, err := storageService.Buckets.GetIamPolicy(bucketName).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy of bucket %s: %w", bucketName, err)
	}

	changed := false
	bindings := make([]*storage.PolicyBindings, 0, len(policy.Bindings))
	for _, binding := range policy.Bindings {
		members := make([]string, 0, len(binding.Members))
		for _, m := range binding.Members {
			if m == member {
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

// addWorkloadIdentityMembers lets each member act as a GCP service account
// through workload identity.
func addWorkloadIdentityMembers(iamService *iam.Service, projectID, serviceAccountEmail string, members []string) error {
	resource := fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, serviceAccountEmail)
	policy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(resource).Do()
	if err != nil {
		return fmt.Errorf("failed to get service account IAM policy: %w", err)
	}

	var binding *iam.Binding
	for _, b := range policy.Bindings {
		if b.Role == "roles/iam.workloadIdentityUser" {
			binding = b
			break
		}
	}
	if binding == nil {
		binding = &iam.Binding{Role: "roles/iam.workloadIdentityUser"}
		policy.Bindings = append(policy.Bindings, binding)
	}

	changed := false
	for _, member := range members {
		if !containsString(binding.Members, member) {
			binding.Members = append(binding.Members, member)
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if _, err := iamService.Projects.ServiceAccounts.SetIamPolicy(
		resource,
		&iam.SetIamPolicyRequest{Policy: policy},
	).Do(); err != nil {
		return fmt.Errorf("failed to set service account IAM policy: %w", err)
	}

	return nil
}

// databaseWorkloadIdentityMember is the workload identity principal of the
// database's Kubernetes service account.
func databaseWorkloadIdentityMember(projectID string) string {
	return fmt.Sprintf(
		"serviceAccount:%s[%s/%s]",
		fmt.Sprintf(workloadIdentityPoolFormat, projectID),
		installer.ControlPlaneNamespace,
		installer.DatabaseServiceAccountName,
	)
}

// deleteBackupResources removes the backup service account and its access to
// the bucket.  The bucket and the backups in it are kept unless DeleteBackups
// is set, since they may still be needed after the control plane is gone.
func (i *KubernetesRuntimeInfraGKE) deleteBackupResources(
	storageService *storage.Service,
	iamService *iam.Service,
) error {
	bucketName := GkeBackupBucketName(i.RuntimeInstanceName, i.ProjectID)
	email := i.expectedBackupServiceAccountEmail()

	account, found, err := getServiceAccountForProject(iamService, i.ProjectID, email)
	if err != nil {
		return fmt.Errorf("failed to check for backup service account: %w", err)
	}
	if found {
		if !serviceAccountOwnedBy(account, i.RuntimeInstanceName) {
			return fmt.Errorf("backup service account %s was not created for %s, leaving it in place", email, i.RuntimeInstanceName)
		}

		// the bucket outlives the account, so its access goes first
		if err := removeBucketMember(storageService, bucketName, "serviceAccount:"+email); err != nil && !isGoogleAPINotFound(err) {
			return err
		}
		if err := deleteServiceAccountForProject(iamService, i.ProjectID, email); err != nil {
			return fmt.Errorf("failed to delete backup service account: %w", err)
		}
	}

	if !i.DeleteBackups {
		return nil
	}

	return deleteBackupBucket(storageService, bucketName, i.RuntimeInstanceName)
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

// backupServiceAccountDescription is the IAM description of a backup service
// account, recording which control plane it belongs to.
func backupServiceAccountDescription(runtimeInstanceName string) string {
	return fmt.Sprintf(
		"Writes database backups for Threeport instance %s; %s",
		runtimeInstanceName,
		GcpOwnershipDescription(runtimeInstanceName),
	)
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
