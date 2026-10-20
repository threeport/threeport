package v0

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// normalizingClient hands every object it creates or updates to the fake
// client as it would arrive from the API server, with its numbers as int64.
// The fake client deep-copies what it is given and panics on the plain ints
// the install manifests are written with.
type normalizingClient struct{ dynamic.Interface }

func (c normalizingClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return normalizingResource{c.Interface.Resource(gvr)}
}

type normalizingResource struct {
	dynamic.NamespaceableResourceInterface
}

func (r normalizingResource) Namespace(namespace string) dynamic.ResourceInterface {
	return normalizingNamespacedResource{r.NamespaceableResourceInterface.Namespace(namespace)}
}

func (r normalizingResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.NamespaceableResourceInterface.Create(ctx, normalized(obj), opts, sub...)
}

func (r normalizingResource) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.NamespaceableResourceInterface.Update(ctx, normalized(obj), opts, sub...)
}

type normalizingNamespacedResource struct{ dynamic.ResourceInterface }

func (r normalizingNamespacedResource) Create(ctx context.Context, obj *unstructured.Unstructured, opts metav1.CreateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.ResourceInterface.Create(ctx, normalized(obj), opts, sub...)
}

func (r normalizingNamespacedResource) Update(ctx context.Context, obj *unstructured.Unstructured, opts metav1.UpdateOptions, sub ...string) (*unstructured.Unstructured, error) {
	return r.ResourceInterface.Update(ctx, normalized(obj), opts, sub...)
}

// normalized returns obj round-tripped through JSON.
func normalized(obj *unstructured.Unstructured) *unstructured.Unstructured {
	data, err := obj.MarshalJSON()
	if err != nil {
		panic(err)
	}
	out := &unstructured.Unstructured{}
	if err := out.UnmarshalJSON(data); err != nil {
		panic(err)
	}

	return out
}

// backupTestMapper returns a mapper for every kind the dependencies and API
// server installs create.
func backupTestMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "policy", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	for _, gvk := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "Secret"},
		{Version: "v1", Kind: "ConfigMap"},
		{Version: "v1", Kind: "Service"},
		{Version: "v1", Kind: "ServiceAccount"},
		{Version: "v1", Kind: "Pod"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "batch", Version: "v1", Kind: "Job"},
		{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	} {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)

	return mapper
}

// backupInstaller returns an installer for a production control plane on
// GKE whose backups were provisioned.
func backupInstaller() *ControlPlaneInstaller {
	return &ControlPlaneInstaller{Opts: Options{
		Name:                        ControlPlaneName,
		Namespace:                   ControlPlaneNamespace,
		RestApiInfo:                 ThreeportRestApi,
		DatabaseMigratorInfo:        DatabaseMigrator,
		AgentInfo:                   ThreeportAgent,
		AdditionalOptions:           map[string]interface{}{},
		CreateOrUpdateKubeResources: true,
		InfraProvider:               "gke",
		Tier:                        ControlPlaneTier(ControlPlaneTierProd),
		GcpBackupBucket:             "threeport-prod-backups-acme-prod",
	}}
}

// installDependencies installs the control plane dependencies into a fake
// cluster and returns it.
func installDependencies(t *testing.T, cpi *ControlPlaneInstaller) *dynamicfake.FakeDynamicClient {
	t.Helper()

	fakeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mapper := backupTestMapper()
	dbCreds, err := auth.GenerateDbCreds(cpi.Opts.Namespace)
	require.NoError(t, err)
	require.NoError(t, cpi.InstallThreeportControlPlaneDependencies(normalizingClient{fakeClient}, &mapper, "", dbCreds))

	return fakeClient
}

// getObject reads an object back from the fake cluster.
func getObject(t *testing.T, kubeClient dynamic.Interface, gvr schema.GroupVersionResource, name string) *unstructured.Unstructured {
	t.Helper()

	object, err := kubeClient.Resource(gvr).Namespace(ControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)

	return object
}

var statefulSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}

// TestTheDatabaseRunsAsItsOwnServiceAccount covers the identity the database
// writes its backups as: its own Kubernetes service account, which the
// backup bucket grants access to by UID.  It acts as no GCP service account,
// and gets no API token since it never calls the Kubernetes API.
func TestTheDatabaseRunsAsItsOwnServiceAccount(t *testing.T) {
	kubeClient := installDependencies(t, backupInstaller())

	statefulSet := getObject(t, kubeClient, statefulSetGVR, "crdb")
	serviceAccountName, _, err := unstructured.NestedString(statefulSet.Object, "spec", "template", "spec", "serviceAccountName")
	require.NoError(t, err)
	assert.Equal(t, DatabaseServiceAccountName, serviceAccountName)

	serviceAccount := getObject(t, kubeClient, serviceAccountGVR, DatabaseServiceAccountName)
	assert.Empty(t, serviceAccount.GetAnnotations(), "the database acts as a GCP service account")
	automount, found, err := unstructured.NestedBool(serviceAccount.Object, "automountServiceAccountToken")
	require.NoError(t, err)
	require.True(t, found, "the database service account leaves token automounting at the default")
	assert.False(t, automount)
}

// TestAReinstallKeepsTheDatabaseServiceAccount covers a reinstall: the
// database's service account is marked to survive it, since one recreated
// would get a new UID, which the backup bucket does not grant access to.
func TestAReinstallKeepsTheDatabaseServiceAccount(t *testing.T) {
	kubeClient := installDependencies(t, backupInstaller())

	serviceAccount := getObject(t, kubeClient, serviceAccountGVR, DatabaseServiceAccountName)
	assert.Equal(t, LabelPersistentValue, serviceAccount.GetLabels()[LabelPersistent])
}

// TestTheDatabaseServiceAccountUIDIsRead covers the UID an install grants
// the backup bucket to: the one the cluster gave the account.
func TestTheDatabaseServiceAccountUIDIsRead(t *testing.T) {
	serviceAccount := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      DatabaseServiceAccountName,
			"namespace": ControlPlaneNamespace,
			"uid":       "6b7a3e5c-1f2d-4c8b-9a0e-2d4f6a8c0e1b",
		},
	}}
	cpi := backupInstaller()

	uid, err := cpi.DatabaseServiceAccountUID(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), serviceAccount))
	require.NoError(t, err)
	assert.Equal(t, "6b7a3e5c-1f2d-4c8b-9a0e-2d4f6a8c0e1b", uid)

	_, err = cpi.DatabaseServiceAccountUID(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()))
	assert.Error(t, err, "a missing account has no UID to grant access to")
}

// TestTheBackupScheduleIsCreatedOnlyWithABucket covers the SQL the database
// migrator runs before the API server starts: the backup schedule is part of
// it on a control plane with a backup bucket, and on no other.
func TestTheBackupScheduleIsCreatedOnlyWithABucket(t *testing.T) {
	withBucket := backupInstaller().dbCreateData()
	require.Contains(t, withBucket, DbBackupScheduleFilename)
	assert.Equal(t,
		"CREATE SCHEDULE IF NOT EXISTS threeport_api_backup FOR BACKUP DATABASE threeport_api "+
			"INTO 'gs://threeport-prod-backups-acme-prod/threeport_api?AUTH=implicit' "+
			"RECURRING '@hourly' FULL BACKUP '@daily' WITH SCHEDULE OPTIONS first_run = 'now';\n",
		withBucket[DbBackupScheduleFilename],
	)

	cpi := backupInstaller()
	cpi.Opts.GcpBackupBucket = ""
	withoutBucket := cpi.dbCreateData()
	assert.NotContains(t, withoutBucket, DbBackupScheduleFilename)

	// the init script is the same either way, and never schedules a backup
	assert.Equal(t, withBucket[DbInitFilename], withoutBucket[DbInitFilename])
	assert.NotContains(t, strings.ToUpper(withBucket[DbInitFilename].(string)), "SCHEDULE")
}
