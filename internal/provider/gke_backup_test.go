package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/cloudresourcemanager/v1"
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
	projectPath       = regexp.MustCompile(`^/v1/projects/([^/:]+)$`)
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
		// a bucket belongs to the project it is created in
		if r.URL.Query().Get("project") == "acme-prod" {
			bucket.ProjectNumber = testProjectNumber
		}
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

	case r.Method == http.MethodGet && projectPath.MatchString(path):
		reply(map[string]interface{}{
			"projectId":     projectPath.FindStringSubmatch(path)[1],
			"projectNumber": strconv.FormatUint(testProjectNumber, 10),
		})

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
		if _, ok := f.accounts[match[1]]; !ok {
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

// clients returns storage and Cloud Resource Manager clients that talk to
// the fake.
func (f *fakeGCP) clients(t *testing.T) (*storage.Service, *cloudresourcemanager.Service) {
	t.Helper()

	server := httptest.NewServer(f)
	t.Cleanup(server.Close)

	ctx := context.Background()
	storageService, err := storage.NewService(ctx, option.WithEndpoint(server.URL+"/storage/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
	require.NoError(t, err)

	return storageService, crmService
}

// bucketMembers returns each role on a bucket and the members holding it.
func (f *fakeGCP) bucketMembers(bucket string) map[string][]string {
	members := map[string][]string{}
	for _, binding := range f.bucketPolicies[bucket].Bindings {
		members[binding.Role] = binding.Members
	}

	return members
}

// testProjectNumber is the number of the test control plane's project.
const testProjectNumber = uint64(485872316478)

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

// testUID is the UID of the database's Kubernetes service account.
const testUID = "6b7a3e5c-1f2d-4c8b-9a0e-2d4f6a8c0e1b"

// testPrincipal is the principal the bucket grants the database access as.
const testPrincipal = "principal://iam.googleapis.com/projects/485872316478/locations/global/workloadIdentityPools/acme-prod.svc.id.goog/kubernetes.serviceaccount.uid/" + testUID

// provisionAndGrant provisions the backups of testBackupInfra and grants the
// database access, as an install does.
func provisionAndGrant(t *testing.T, storageService *storage.Service, crmService *cloudresourcemanager.Service) *KubernetesRuntimeInfraGKE {
	t.Helper()

	infra := testBackupInfra()
	require.NoError(t, infra.provisionBackupsWith(storageService, crmService))
	require.NoError(t, infra.grantDatabaseBackupAccessWith(storageService, crmService, testUID))

	return infra
}

// TestProvisionBackups covers what a production install creates: a private
// bucket in the backup region, labelled as this control plane's, that the
// database's Kubernetes service account can write and read backups in and do
// nothing else.
func TestProvisionBackups(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)

	infra := provisionAndGrant(t, storageService, crmService)

	bucketName := "threeport-prod-backups-acme-prod"
	assert.Equal(t, bucketName, infra.BackupBucket)
	bucket := fake.buckets[bucketName]
	require.NotNil(t, bucket, "no backup bucket was created")
	assert.Equal(t, "US-EAST1", bucket.Location)
	assert.True(t, bucketOwnedBy(bucket, infra.RuntimeInstanceName))
	assert.True(t, bucket.IamConfiguration.UniformBucketLevelAccess.Enabled)
	assert.Equal(t, "enforced", bucket.IamConfiguration.PublicAccessPrevention)

	roles := []string{}
	for role, members := range fake.bucketMembers(bucketName) {
		assert.Equal(t, []string{testPrincipal}, members, "role %s names someone other than the database", role)
		roles = append(roles, role)
	}
	assert.ElementsMatch(t, backupBucketRoles, roles)

	assert.Empty(t, fake.accounts, "a GCP service account was created")
}

// TestOtherClustersCannotReadBackups covers who the bucket lets in.  Every
// cluster in a project shares one workload identity pool, so a grant to the
// database's namespace and account name would also let in any other cluster
// that creates the same namespace and account.  The grant names the account
// by the UID its own cluster gave it instead.
func TestOtherClustersCannotReadBackups(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)

	provisionAndGrant(t, storageService, crmService)

	for _, members := range fake.bucketMembers("threeport-prod-backups-acme-prod") {
		for _, member := range members {
			assert.Contains(t, member, "/kubernetes.serviceaccount.uid/"+testUID)
			assert.NotContains(t, member, "/subject/ns/")
			assert.NotContains(t, member, "svc.id.goog[")
			assert.NotContains(t, member, "principalSet://")
		}
	}
}

// TestBackupRolesCannotDeleteBackups covers the least privilege the issue
// asks for: the database can write and read objects, but neither delete one
// nor administer the bucket, so a compromised database cannot destroy the
// backups taken of it.
func TestBackupRolesCannotDeleteBackups(t *testing.T) {
	for _, role := range backupBucketRoles {
		assert.NotContains(t, role, "Admin", role)
		assert.NotEqual(t, "roles/storage.objectUser", role, "objectUser can delete objects")
	}
}

// TestGrantNeedsABucketAndAUID covers a grant with nothing to grant: no
// bucket was provisioned, or the account has no UID.
func TestGrantNeedsABucketAndAUID(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)

	assert.Error(t, testBackupInfra().grantDatabaseBackupAccessWith(storageService, crmService, testUID))

	infra := testBackupInfra()
	require.NoError(t, infra.provisionBackupsWith(storageService, crmService))
	assert.Error(t, infra.grantDatabaseBackupAccessWith(storageService, crmService, ""))
}

// TestProvisionBackupsAgainKeepsTheBucket covers a reinstall of the same
// control plane: it reuses the bucket, and with it the backups taken before,
// and grants nothing twice.
func TestProvisionBackupsAgainKeepsTheBucket(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)

	provisionAndGrant(t, storageService, crmService)
	fake.objects["threeport-prod-backups-acme-prod"] = []string{"2026/10/20-050726.51/BACKUP_MANIFEST"}
	provisionAndGrant(t, storageService, crmService)

	assert.Len(t, fake.bucketsCreated, 1)
	assert.NotEmpty(t, fake.objects["threeport-prod-backups-acme-prod"])
	for role, members := range fake.bucketMembers("threeport-prod-backups-acme-prod") {
		assert.Len(t, members, 1, role)
	}
}

// TestProvisionBackupsRefusesABucketItDidNotCreate covers a bucket name
// already taken by something else, and the control plane's own bucket in
// another region than now asked for: neither is used.
func TestProvisionBackupsRefusesABucketItDidNotCreate(t *testing.T) {
	t.Run("not ours", func(t *testing.T) {
		fake := newFakeGCP()
		fake.buckets["threeport-prod-backups-acme-prod"] = &storage.Bucket{
			Name: "threeport-prod-backups-acme-prod", Location: "US-EAST1", ProjectNumber: testProjectNumber,
		}
		storageService, crmService := fake.clients(t)

		assert.ErrorContains(t, testBackupInfra().provisionBackupsWith(storageService, crmService), "not created for")
	})

	t.Run("other region", func(t *testing.T) {
		fake := newFakeGCP()
		storageService, crmService := fake.clients(t)
		require.NoError(t, testBackupInfra().provisionBackupsWith(storageService, crmService))

		moved := testBackupInfra()
		moved.BackupRegion = "us-west1"
		assert.ErrorContains(t, moved.provisionBackupsWith(storageService, crmService), "already exists in us-east1")
	})
}

// tearingDown returns infra for tearing the test control plane down, with
// its bucket recorded as the config records it.
func tearingDown(deleteBackups bool) *KubernetesRuntimeInfraGKE {
	infra := testBackupInfra()
	infra.BackupBucket = "threeport-prod-backups-acme-prod"
	infra.DeleteBackups = deleteBackups

	return infra
}

// TestTeardownKeepsBackupsByDefault covers the issue's teardown rule: the
// database's access goes with the control plane, but the bucket and the
// backups in it stay unless the operator asks for them to be deleted.
func TestTeardownKeepsBackupsByDefault(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)
	provisionAndGrant(t, storageService, crmService)
	bucketName := "threeport-prod-backups-acme-prod"
	fake.objects[bucketName] = []string{"2026/10/20-050726.51/BACKUP_MANIFEST"}

	// someone else's access to the bucket is not threeport's to remove
	fake.bucketPolicies[bucketName].Bindings = append(fake.bucketPolicies[bucketName].Bindings,
		&storage.PolicyBindings{Role: "roles/storage.objectViewer", Members: []string{"group:auditors@acme.example"}})

	require.NoError(t, tearingDown(false).deleteBackupResources(storageService))

	assert.Equal(t, map[string][]string{"roles/storage.objectViewer": {"group:auditors@acme.example"}}, fake.bucketMembers(bucketName))
	assert.Contains(t, fake.buckets, bucketName, "the bucket was deleted without being asked to")
	assert.Empty(t, fake.objectsDeleted)
}

// TestTeardownDeletesBackupsWhenAsked covers the opt-in: every backup is
// removed, then the bucket.
func TestTeardownDeletesBackupsWhenAsked(t *testing.T) {
	fake := newFakeGCP()
	storageService, crmService := fake.clients(t)
	provisionAndGrant(t, storageService, crmService)
	bucketName := "threeport-prod-backups-acme-prod"
	fake.objects[bucketName] = []string{"a/BACKUP_MANIFEST", "a/data/1.sst"}

	require.NoError(t, tearingDown(true).deleteBackupResources(storageService))

	assert.ElementsMatch(t, []string{"a/BACKUP_MANIFEST", "a/data/1.sst"}, fake.objectsDeleted)
	assert.Equal(t, []string{bucketName}, fake.bucketsDeleted)
}

// TestTeardownLeavesWhatItDidNotCreate covers a recorded bucket this control
// plane did not create: it is not touched, even when deleting backups was
// asked for.
func TestTeardownLeavesWhatItDidNotCreate(t *testing.T) {
	fake := newFakeGCP()
	bucketName := "threeport-prod-backups-acme-prod"
	fake.buckets[bucketName] = &storage.Bucket{Name: bucketName}
	fake.bucketPolicies[bucketName] = &storage.Policy{}
	fake.objects[bucketName] = []string{"someone-elses-data"}
	storageService, _ := fake.clients(t)

	assert.Error(t, tearingDown(true).deleteBackupResources(storageService))
	assert.Contains(t, fake.buckets, bucketName)
	assert.Empty(t, fake.objectsDeleted)
}

// TestTeardownWithoutABucketDoesNothing covers a development control plane,
// or a child runtime the controller deletes: no bucket is recorded, so
// nothing is looked up, even with deleting backups asked for.
func TestTeardownWithoutABucketDoesNothing(t *testing.T) {
	infra := testBackupInfra()
	infra.DeleteBackups = true

	// a client that fails every call shows none is made
	storageService, err := storage.NewService(context.Background(),
		option.WithEndpoint("http://127.0.0.1:1/storage/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)
	require.NoError(t, infra.deleteBackupResources(storageService))
}

// TestABucketInAnotherProjectIsNeverUsed covers a backup bucket's name taken
// by someone else.  Bucket names are global and this one is predictable, so
// anyone can create it in a project of their own, copy the control plane's
// labels onto it, and let the installer manage its policy.  They keep their
// own access when the database is granted its, so writing backups there would
// hand them the database.  Neither provisioning the bucket nor granting the
// database access to it accepts one that is not in the control plane's
// project.
func TestABucketInAnotherProjectIsNeverUsed(t *testing.T) {
	bucketName := "threeport-prod-backups-acme-prod"
	squatted := func(fake *fakeGCP) {
		fake.buckets[bucketName] = &storage.Bucket{
			Name:          bucketName,
			Location:      "US-EAST1",
			Labels:        GcpResourceLabels("threeport-prod"),
			ProjectNumber: 111111111111,
		}
		fake.bucketPolicies[bucketName] = &storage.Policy{Bindings: []*storage.PolicyBindings{
			{Role: "roles/storage.objectViewer", Members: []string{"user:someone-else@example.com"}},
		}}
	}

	t.Run("provisioning", func(t *testing.T) {
		fake := newFakeGCP()
		squatted(fake)
		storageService, crmService := fake.clients(t)

		infra := testBackupInfra()
		assert.ErrorContains(t, infra.provisionBackupsWith(storageService, crmService), "not to project acme-prod")
		assert.Empty(t, infra.BackupBucket, "a bucket in another project was recorded as the backup bucket")
	})

	t.Run("granting", func(t *testing.T) {
		fake := newFakeGCP()
		squatted(fake)
		storageService, crmService := fake.clients(t)

		// as a config recording the bucket name would have it
		infra := testBackupInfra()
		infra.BackupBucket = bucketName
		assert.ErrorContains(t, infra.grantDatabaseBackupAccessWith(storageService, crmService, testUID), "not to project acme-prod")
		for role, members := range fake.bucketMembers(bucketName) {
			assert.NotContains(t, members, testPrincipal, "the database was granted %s on a bucket in another project", role)
		}
	})
}
