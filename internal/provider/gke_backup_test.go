package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/storage/v1"
)

// fakeGCP stands in for the Cloud Storage and IAM APIs, holding buckets,
// objects, service accounts and their IAM policies in memory.
type fakeGCP struct {
	mu sync.Mutex

	buckets         map[string]*storage.Bucket
	bucketPolicies  map[string]*storage.Policy
	objects         map[string][]string
	accounts        map[string]*iam.ServiceAccount
	accountPolicies map[string]*iam.Policy

	// how many more times a bucket policy naming a service account, or a
	// service account's own policy, is refused as if the account were not
	// yet known, the way GCP refuses one just created
	bucketPolicyNotPropagated  int
	accountPolicyNotPropagated int

	bucketsCreated  []string
	bucketsDeleted  []string
	objectsDeleted  []string
	accountsDeleted []string
}

func newFakeGCP() *fakeGCP {
	return &fakeGCP{
		buckets:         map[string]*storage.Bucket{},
		bucketPolicies:  map[string]*storage.Policy{},
		objects:         map[string][]string{},
		accounts:        map[string]*iam.ServiceAccount{},
		accountPolicies: map[string]*iam.Policy{},
	}
}

var (
	bucketPath        = regexp.MustCompile(`^/storage/v1/b/([^/]+)$`)
	bucketIamPath     = regexp.MustCompile(`^/storage/v1/b/([^/]+)/iam$`)
	objectsPath       = regexp.MustCompile(`^/storage/v1/b/([^/]+)/o$`)
	objectPath        = regexp.MustCompile(`^/storage/v1/b/([^/]+)/o/(.+)$`)
	accountsPath      = regexp.MustCompile(`^/v1/projects/([^/]+)/serviceAccounts$`)
	accountPath       = regexp.MustCompile(`^/v1/projects/[^/]+/serviceAccounts/([^/:]+)$`)
	accountPolicyPath = regexp.MustCompile(`^/v1/projects/[^/]+/serviceAccounts/([^/:]+):(getIamPolicy|setIamPolicy)$`)
)

func (f *fakeGCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	reply := func(v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	notFound := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": {"code": 404, "message": "Not Found"}}`))
	}
	path := r.URL.Path

	switch {
	case r.Method == http.MethodPost && path == "/storage/v1/b":
		var bucket storage.Bucket
		_ = json.Unmarshal(body, &bucket)
		bucket.Location = strings.ToUpper(bucket.Location)
		f.buckets[bucket.Name] = &bucket
		f.bucketPolicies[bucket.Name] = &storage.Policy{}
		f.bucketsCreated = append(f.bucketsCreated, bucket.Name)
		reply(&bucket)

	case bucketIamPath.MatchString(path):
		name := bucketIamPath.FindStringSubmatch(path)[1]
		if _, ok := f.buckets[name]; !ok {
			notFound()
			return
		}
		if r.Method == http.MethodPut && f.bucketPolicyNotPropagated > 0 {
			f.bucketPolicyNotPropagated--
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "Service account threeport-bkp-threeport-prod@acme-prod.iam.gserviceaccount.com does not exist."}}`))
			return
		}
		if r.Method == http.MethodPut {
			var policy storage.Policy
			_ = json.Unmarshal(body, &policy)
			f.bucketPolicies[name] = &policy
		}
		reply(f.bucketPolicies[name])

	case objectsPath.MatchString(path):
		name := objectsPath.FindStringSubmatch(path)[1]
		items := []*storage.Object{}
		for _, object := range f.objects[name] {
			items = append(items, &storage.Object{Name: object})
		}
		reply(&storage.Objects{Items: items})

	case objectPath.MatchString(path) && r.Method == http.MethodDelete:
		match := objectPath.FindStringSubmatch(path)
		f.objectsDeleted = append(f.objectsDeleted, match[2])
		remaining := []string{}
		for _, object := range f.objects[match[1]] {
			if object != match[2] {
				remaining = append(remaining, object)
			}
		}
		f.objects[match[1]] = remaining

	case bucketPath.MatchString(path):
		name := bucketPath.FindStringSubmatch(path)[1]
		bucket, ok := f.buckets[name]
		if !ok {
			notFound()
			return
		}
		if r.Method == http.MethodDelete {
			if len(f.objects[name]) > 0 {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error": {"code": 409, "message": "bucket not empty"}}`))
				return
			}
			delete(f.buckets, name)
			f.bucketsDeleted = append(f.bucketsDeleted, name)
			return
		}
		reply(bucket)

	case r.Method == http.MethodPost && accountsPath.MatchString(path):
		project := accountsPath.FindStringSubmatch(path)[1]
		var request iam.CreateServiceAccountRequest
		_ = json.Unmarshal(body, &request)
		account := request.ServiceAccount
		account.Email = request.AccountId + "@" + project + ".iam.gserviceaccount.com"
		f.accounts[account.Email] = account
		f.accountPolicies[account.Email] = &iam.Policy{}
		reply(account)

	case accountPolicyPath.MatchString(path):
		match := accountPolicyPath.FindStringSubmatch(path)
		if _, ok := f.accounts[match[1]]; !ok || f.accountPolicyNotPropagated > 0 {
			if ok {
				f.accountPolicyNotPropagated--
			}
			notFound()
			return
		}
		if match[2] == "setIamPolicy" {
			var request iam.SetIamPolicyRequest
			_ = json.Unmarshal(body, &request)
			f.accountPolicies[match[1]] = request.Policy
		}
		reply(f.accountPolicies[match[1]])

	case accountPath.MatchString(path):
		email := accountPath.FindStringSubmatch(path)[1]
		account, ok := f.accounts[email]
		if !ok {
			notFound()
			return
		}
		if r.Method == http.MethodDelete {
			delete(f.accounts, email)
			f.accountsDeleted = append(f.accountsDeleted, email)
			reply(map[string]interface{}{})
			return
		}
		reply(account)

	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// clients returns storage and IAM clients that talk to the fake.
func (f *fakeGCP) clients(t *testing.T) (*storage.Service, *iam.Service) {
	t.Helper()

	server := httptest.NewServer(f)
	t.Cleanup(server.Close)

	ctx := context.Background()
	storageService, err := storage.NewService(ctx, option.WithEndpoint(server.URL+"/storage/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)
	iamService, err := iam.NewService(ctx, option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
	require.NoError(t, err)

	return storageService, iamService
}

// bucketMembers returns each role on a bucket and the members holding it.
func (f *fakeGCP) bucketMembers(bucket string) map[string][]string {
	members := map[string][]string{}
	for _, binding := range f.bucketPolicies[bucket].Bindings {
		members[binding.Role] = binding.Members
	}

	return members
}

// testBackupInfra returns GKE infra for a production control plane.
func testBackupInfra() *KubernetesRuntimeInfraGKE {
	return &KubernetesRuntimeInfraGKE{
		PulumiWorkspace:  PulumiWorkspace{RuntimeInstanceName: "threeport-prod"},
		ProjectID:        "acme-prod",
		Region:           "us-central1",
		ProvisionBackups: true,
		BackupRegion:     "us-east1",
	}
}

// TestBackupsGoToAnotherRegion covers the issue's requirement that backups
// survive the loss of the cluster's region: the backup region is never the
// cluster's own, whether it is given or chosen by default, and the default
// stays on the cluster's continent.
func TestBackupsGoToAnotherRegion(t *testing.T) {
	for clusterRegion, backupRegion := range defaultBackupRegions {
		got, err := GkeBackupRegion(clusterRegion, "")
		require.NoError(t, err, clusterRegion)
		assert.Equal(t, backupRegion, got)
		assert.NotEqual(t, clusterRegion, got)
		assert.Equal(t,
			strings.Split(clusterRegion, "-")[0], strings.Split(got, "-")[0],
			"%s backs up to %s, on another continent", clusterRegion, got,
		)
	}

	got, err := GkeBackupRegion("us-central1", "us-west1")
	require.NoError(t, err)
	assert.Equal(t, "us-west1", got, "a given backup region is used as given")

	_, err = GkeBackupRegion("us-central1", "us-central1")
	assert.Error(t, err, "a backup in the cluster's own region is refused")

	_, err = GkeBackupRegion("me-central1", "")
	assert.ErrorContains(t, err, "--gcp-backup-region", "a region without a default asks for one")
}

// TestBackupBucketNamesAreValidAndDistinct covers the bucket name GCS
// requires: at most 63 lowercase characters, letters, digits and hyphens,
// starting and ending with a letter or digit.  Names that would be too long
// are shortened without two of them colliding.
func TestBackupBucketNamesAreValidAndDistinct(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

	short := GkeBackupBucketName("threeport-prod", "acme-prod")
	assert.Equal(t, "threeport-prod-backups-acme-prod", short)

	long := strings.Repeat("a", 30)
	first := GkeBackupBucketName("threeport-"+long+"1", "project-with-a-long-identifier")
	second := GkeBackupBucketName("threeport-"+long+"2", "project-with-a-long-identifier")
	for _, name := range []string{short, first, second} {
		assert.Regexp(t, valid, name)
	}
	assert.NotEqual(t, first, second)
}

// TestProvisionBackups covers what a production install creates: a private
// bucket in the backup region, labelled as this control plane's, and a
// service account that can write and read backups in that bucket and do
// nothing else, which the database's Kubernetes service account acts as.
func TestProvisionBackups(t *testing.T) {
	fake := newFakeGCP()
	storageService, iamService := fake.clients(t)
	infra := testBackupInfra()

	require.NoError(t, infra.provisionBackupsWith(storageService, iamService))

	bucketName := "threeport-prod-backups-acme-prod"
	bucket := fake.buckets[bucketName]
	require.NotNil(t, bucket, "no backup bucket was created")
	assert.Equal(t, "US-EAST1", bucket.Location)
	assert.True(t, bucketOwnedBy(bucket, infra.RuntimeInstanceName))
	assert.True(t, bucket.IamConfiguration.UniformBucketLevelAccess.Enabled)
	assert.Equal(t, "enforced", bucket.IamConfiguration.PublicAccessPrevention)

	email := "threeport-bkp-threeport-prod@acme-prod.iam.gserviceaccount.com"
	account := fake.accounts[email]
	require.NotNil(t, account, "no backup service account was created")
	assert.True(t, serviceAccountOwnedBy(account, infra.RuntimeInstanceName))

	member := "serviceAccount:" + email
	roles := []string{}
	for role, members := range fake.bucketMembers(bucketName) {
		if containsString(members, member) {
			roles = append(roles, role)
		}
	}
	assert.ElementsMatch(t, backupBucketRoles, roles, "the backup account holds other roles on the bucket than writing and reading objects")

	var workloadIdentityUsers []string
	for _, binding := range fake.accountPolicies[email].Bindings {
		if binding.Role == "roles/iam.workloadIdentityUser" {
			workloadIdentityUsers = binding.Members
		}
	}
	assert.Equal(t, []string{"serviceAccount:acme-prod.svc.id.goog[threeport-control-plane/crdb]"}, workloadIdentityUsers)

	assert.Equal(t, bucketName, infra.BackupBucket)
	assert.Equal(t, email, infra.BackupServiceAccountEmail)
}

// TestBackupRolesCannotDeleteBackups covers the least privilege the issue
// asks for: the backup account can write and read objects, but neither
// delete one nor administer the bucket, so a compromised database cannot
// destroy the backups taken of it.
func TestBackupRolesCannotDeleteBackups(t *testing.T) {
	for _, role := range backupBucketRoles {
		assert.NotContains(t, role, "Admin", role)
		assert.NotEqual(t, "roles/storage.objectUser", role, "objectUser can delete objects")
	}
}

// TestProvisionBackupsAgainKeepsTheBucket covers a reinstall of the same
// control plane: it reuses the bucket, and with it the backups taken before,
// and grants nothing twice.
func TestProvisionBackupsAgainKeepsTheBucket(t *testing.T) {
	fake := newFakeGCP()
	storageService, iamService := fake.clients(t)

	require.NoError(t, testBackupInfra().provisionBackupsWith(storageService, iamService))
	fake.objects["threeport-prod-backups-acme-prod"] = []string{"2026/10/20-050726.51/BACKUP_MANIFEST"}
	require.NoError(t, testBackupInfra().provisionBackupsWith(storageService, iamService))

	assert.Len(t, fake.bucketsCreated, 1)
	assert.NotEmpty(t, fake.objects["threeport-prod-backups-acme-prod"])
	for role, members := range fake.bucketMembers("threeport-prod-backups-acme-prod") {
		assert.Len(t, members, 1, role)
	}
}

// TestProvisionBackupsRefusesABucketItDidNotCreate covers a bucket name
// already taken by something else, and the control plane's own bucket in
// another region than now asked for: neither is written to.
func TestProvisionBackupsRefusesABucketItDidNotCreate(t *testing.T) {
	t.Run("not ours", func(t *testing.T) {
		fake := newFakeGCP()
		fake.buckets["threeport-prod-backups-acme-prod"] = &storage.Bucket{Name: "threeport-prod-backups-acme-prod", Location: "US-EAST1"}
		storageService, iamService := fake.clients(t)

		assert.ErrorContains(t, testBackupInfra().provisionBackupsWith(storageService, iamService), "not created for")
		assert.Empty(t, fake.accounts)
	})

	t.Run("other region", func(t *testing.T) {
		fake := newFakeGCP()
		storageService, iamService := fake.clients(t)
		require.NoError(t, testBackupInfra().provisionBackupsWith(storageService, iamService))

		moved := testBackupInfra()
		moved.BackupRegion = "us-west1"
		assert.ErrorContains(t, moved.provisionBackupsWith(storageService, iamService), "already exists in us-east1")
	})
}

// TestTeardownKeepsBackupsByDefault covers the issue's teardown rule: the
// backup account and its access go with the control plane, but the bucket
// and the backups in it stay unless the operator asks for them to be deleted.
func TestTeardownKeepsBackupsByDefault(t *testing.T) {
	fake := newFakeGCP()
	storageService, iamService := fake.clients(t)
	infra := testBackupInfra()
	require.NoError(t, infra.provisionBackupsWith(storageService, iamService))
	bucketName := infra.BackupBucket
	fake.objects[bucketName] = []string{"2026/10/20-050726.51/BACKUP_MANIFEST"}

	require.NoError(t, testBackupInfra().deleteBackupResources(storageService, iamService))

	assert.Equal(t, []string{infra.BackupServiceAccountEmail}, fake.accountsDeleted)
	assert.Empty(t, fake.bucketMembers(bucketName), "the deleted account keeps access to the bucket")
	assert.Contains(t, fake.buckets, bucketName, "the bucket was deleted without being asked to")
	assert.Empty(t, fake.objectsDeleted)
}

// TestTeardownDeletesBackupsWhenAsked covers the opt-in: every backup is
// removed, then the bucket.
func TestTeardownDeletesBackupsWhenAsked(t *testing.T) {
	fake := newFakeGCP()
	storageService, iamService := fake.clients(t)
	infra := testBackupInfra()
	require.NoError(t, infra.provisionBackupsWith(storageService, iamService))
	bucketName := infra.BackupBucket
	fake.objects[bucketName] = []string{"a/BACKUP_MANIFEST", "a/data/1.sst"}

	deleting := testBackupInfra()
	deleting.DeleteBackups = true
	require.NoError(t, deleting.deleteBackupResources(storageService, iamService))

	assert.ElementsMatch(t, []string{"a/BACKUP_MANIFEST", "a/data/1.sst"}, fake.objectsDeleted)
	assert.Equal(t, []string{bucketName}, fake.bucketsDeleted)
}

// TestTeardownLeavesWhatItDidNotCreate covers a bucket or account of the
// same name that this control plane did not create: neither is touched, even
// when deleting backups was asked for.
func TestTeardownLeavesWhatItDidNotCreate(t *testing.T) {
	fake := newFakeGCP()
	bucketName := "threeport-prod-backups-acme-prod"
	fake.buckets[bucketName] = &storage.Bucket{Name: bucketName}
	fake.bucketPolicies[bucketName] = &storage.Policy{}
	fake.objects[bucketName] = []string{"someone-elses-data"}
	storageService, iamService := fake.clients(t)

	deleting := testBackupInfra()
	deleting.DeleteBackups = true
	assert.Error(t, deleting.deleteBackupResources(storageService, iamService))
	assert.Contains(t, fake.buckets, bucketName)
	assert.Empty(t, fake.objectsDeleted)

	email := "threeport-bkp-threeport-prod@acme-prod.iam.gserviceaccount.com"
	fake.accounts[email] = &iam.ServiceAccount{Email: email, Description: "not threeport's"}
	assert.Error(t, testBackupInfra().deleteBackupResources(storageService, iamService))
	assert.Contains(t, fake.accounts, email)
}

// TestTeardownWithoutBackupsDoesNothing covers a development control plane,
// or a child runtime the controller deletes: there is no backup account, and
// with deleting backups not asked for, no storage call is needed.
func TestTeardownWithoutBackupsDoesNothing(t *testing.T) {
	fake := newFakeGCP()
	storageService, iamService := fake.clients(t)

	require.NoError(t, testBackupInfra().deleteBackupResources(storageService, iamService))
	assert.Empty(t, fake.accountsDeleted)
	assert.Empty(t, fake.bucketsDeleted)
}

// TestProvisionBackupsWaitsForTheNewServiceAccount covers what a live
// install hit: right after the backup service account is created, Cloud
// Storage refuses a policy naming it and IAM does not find it, until it has
// propagated.  Granting it access is retried rather than failing the install.
func TestProvisionBackupsWaitsForTheNewServiceAccount(t *testing.T) {
	delay := serviceAccountPropagationDelay
	serviceAccountPropagationDelay = 0
	t.Cleanup(func() { serviceAccountPropagationDelay = delay })

	fake := newFakeGCP()
	fake.bucketPolicyNotPropagated = 3
	fake.accountPolicyNotPropagated = 2
	storageService, iamService := fake.clients(t)

	infra := testBackupInfra()
	require.NoError(t, infra.provisionBackupsWith(storageService, iamService))
	assert.Zero(t, fake.bucketPolicyNotPropagated)
	assert.Zero(t, fake.accountPolicyNotPropagated)
	assert.NotEmpty(t, fake.bucketMembers(infra.BackupBucket)["roles/storage.objectCreator"])
}

// TestProvisionBackupsGivesUpOnAnAccountThatNeverAppears covers the bound on
// that retry: an account that never becomes known fails the install.
func TestProvisionBackupsGivesUpOnAnAccountThatNeverAppears(t *testing.T) {
	delay := serviceAccountPropagationDelay
	serviceAccountPropagationDelay = 0
	t.Cleanup(func() { serviceAccountPropagationDelay = delay })

	fake := newFakeGCP()
	fake.bucketPolicyNotPropagated = serviceAccountPropagationAttempts + 1
	storageService, iamService := fake.clients(t)

	assert.ErrorContains(t, testBackupInfra().provisionBackupsWith(storageService, iamService), "did not become available")
}

// TestOtherFailuresAreNotRetried covers the errors the retry leaves alone: a
// failure that is not about a service account's propagation is returned at
// once.
func TestOtherFailuresAreNotRetried(t *testing.T) {
	calls := 0
	err := retryWhileServiceAccountPropagates(func() error {
		calls++
		return &googleapi.Error{Code: 403, Message: "permission denied"}
	})
	assert.Error(t, err)
	assert.Equal(t, 1, calls)
}
