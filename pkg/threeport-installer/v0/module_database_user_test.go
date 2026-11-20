package v0

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	testDbThreeportNamespace = "threeport-control-plane"
	testDbModuleNamespace    = "threeport-acme-widget"
)

// recordedModuleDatabaseJob is a module user Job a test ran in place of the
// cluster.
type recordedModuleDatabaseJob struct {
	namespace, name, script string
}

// stubModuleDatabaseJobs records the module user Jobs run, in place of the
// cluster, for the rest of the test.
func stubModuleDatabaseJobs(t *testing.T) *[]recordedModuleDatabaseJob {
	t.Helper()
	var jobs []recordedModuleDatabaseJob
	original := runModuleDatabaseJob
	t.Cleanup(func() { runModuleDatabaseJob = original })
	runModuleDatabaseJob = func(_ dynamic.Interface, namespace, name, script string) error {
		jobs = append(jobs, recordedModuleDatabaseJob{namespace, name, script})
		return nil
	}

	return &jobs
}

// moduleDatabaseTestClient returns a client holding the control plane's
// database client certificates.
func moduleDatabaseTestClient() dynamic.Interface {
	threeportCerts := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": dbThreeportCertSecretName, "namespace": testDbThreeportNamespace},
		"data": map[string]interface{}{
			"ca.crt": base64.StdEncoding.EncodeToString([]byte("database-ca")),
		},
	}}

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{deploymentGVR: "DeploymentList"},
		threeportCerts,
	)
}

// moduleDeployment returns a module's API server Deployment, taking its
// database credentials from the module's user's secret when ownUser is set,
// and connecting with the database's client certificates otherwise.
func moduleDeployment(ownUser bool) *unstructured.Unstructured {
	// a module takes other variables from secrets of its own too
	env := []interface{}{
		map[string]interface{}{"name": "DB_HOST", "value": "crdb"},
		map[string]interface{}{"name": "ENCRYPTION_KEY", "valueFrom": map[string]interface{}{
			"secretKeyRef": map[string]interface{}{"name": "encryption-key", "key": "key"},
		}},
	}
	if ownUser {
		env = append(env, ModuleDatabaseEnv()...)
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "acme-widget-api-server", "namespace": testDbModuleNamespace},
		"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{"name": "api-server", "env": env}},
		}}},
	}}
}

// testSecretData returns a secret's decoded data, or nil when it's absent.
func testSecretData(t *testing.T, kubeClient dynamic.Interface, namespace, name string) map[string]string {
	t.Helper()
	secret, err := kubeClient.Resource(secretGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)
	data := map[string]string{}
	raw, _, _ := unstructured.NestedStringMap(secret.Object, "data")
	for key, encoded := range raw {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		data[key] = string(decoded)
	}

	return data
}

// TestAModulesDatabaseUserIsNamedForIt covers the user a module connects as:
// threeport_ and its name, holding only what a user name can, and no longer
// than CockroachDB accepts.
func TestAModulesDatabaseUserIsNamedForIt(t *testing.T) {
	assert.Equal(t, "threeport_acme_widget", ModuleDatabaseUser("acme-widget"))
	assert.Equal(t, "threeport_acme_widget", ModuleDatabaseUser("Acme.Widget-"))
	long := ModuleDatabaseUser(strings.Repeat("a", 80))
	assert.Len(t, long, 63)
	assert.True(t, strings.HasPrefix(long, "threeport_"))
	assert.NotEqual(t, ModuleDatabaseUser("acme"), "threeport", "never the control plane's own user")
	for _, name := range []string{"modules", "Modules", "audited-modules", "audited_modules"} {
		assert.NotEqual(t, moduleDatabaseAuditRole, ModuleDatabaseUser(name), "never the audited role: %s", name)
	}
	assert.False(t, strings.HasPrefix(moduleDatabaseAuditRole, moduleDatabaseUserPrefix), "no module's user can be named as the audited role")
}

// TestAModulesDatabaseUserCanDoWhatModulesDidAndIsAudited covers the
// statements that create a module's user: it logs in with its password, is
// a member of the threeport user, as modules connected as that user before,
// can use the control plane's database, leaves the tables it creates usable
// by the threeport user, and is audited with the other modules' users.
func TestAModulesDatabaseUserCanDoWhatModulesDidAndIsAudited(t *testing.T) {
	statements := ModuleDatabaseUserStatements("threeport_acme", "secret-password")
	assert.Equal(t, "USE threeport_api", statements[0], "default privileges are the database's")
	assert.Contains(t, statements, "CREATE USER IF NOT EXISTS threeport_acme")
	assert.Contains(t, statements, "ALTER USER threeport_acme WITH LOGIN PASSWORD 'secret-password'",
		"an existing user takes a rotated password, and logs in again after being disabled")
	assert.Contains(t, statements, "GRANT threeport, audited_modules TO threeport_acme")
	assert.Contains(t, statements, "GRANT ALL ON DATABASE threeport_api TO threeport_acme")
	assert.Contains(t, statements, "ALTER DEFAULT PRIVILEGES FOR ROLE threeport_acme GRANT ALL ON TABLES TO threeport")
	assert.Contains(t, statements, "ALTER DEFAULT PRIVILEGES FOR ROLE threeport_acme GRANT ALL ON SEQUENCES TO threeport")
	assert.Equal(t, "SET CLUSTER SETTING sql.log.user_audit = 'audited_modules ALL'", statements[len(statements)-1])

	drop := ModuleDatabaseDropStatements("threeport_acme")
	assert.Equal(t, []string{
		"USE threeport_api",
		"REASSIGN OWNED BY threeport_acme TO threeport",
		"ALTER DEFAULT PRIVILEGES FOR ROLE threeport_acme REVOKE ALL ON TABLES FROM threeport",
		"ALTER DEFAULT PRIVILEGES FOR ROLE threeport_acme REVOKE ALL ON SEQUENCES FROM threeport",
		"DROP OWNED BY threeport_acme",
		"DROP USER IF EXISTS threeport_acme",
	}, drop, "the tables it created are kept")
}

// TestAModulesDatabaseUserIsCreatedByTheControlPlane covers creating a
// module's user: the statements run in the control plane's namespace, the
// password is kept there and reused, and the module's namespace gets only
// the CA certificate, its user and its password.  Deleting the kept password
// rotates it.
func TestAModulesDatabaseUserIsCreatedByTheControlPlane(t *testing.T) {
	jobs := stubModuleDatabaseJobs(t)
	kubeClient := moduleDatabaseTestClient()

	digest, err := EnsureModuleDatabaseUser(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)

	kept := testSecretData(t, kubeClient, testDbThreeportNamespace, "db-module-user-acme-widget")
	password := kept["password"]
	assert.Len(t, password, 43, "32 random bytes")
	assert.NotContains(t, password, "'")
	keptSecret, err := kubeClient.Resource(secretGVR).Namespace(testDbThreeportNamespace).Get(
		context.Background(), "db-module-user-acme-widget", metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, LabelPersistentValue, keptSecret.GetLabels()[LabelPersistent], "a reinstall keeps it")

	require.Len(t, *jobs, 1)
	job := (*jobs)[0]
	assert.Equal(t, testDbThreeportNamespace, job.namespace, "root's credentials stay in the control plane's namespace")
	assert.Equal(t, "threeport-db-user-acme-widget", job.name)
	assert.Contains(t, job.script, "ALTER USER threeport_acme_widget WITH LOGIN PASSWORD '"+password+"'")
	assert.Contains(t, job.script, "--certs-dir=/cockroach/cockroach-certs --host=crdb --port=26257 <<'EOF'",
		"the statements, password included, reach the client on its standard input")

	assert.Equal(t, map[string]string{
		"ca.crt":      "database-ca",
		"DB_USER":     "threeport_acme_widget",
		"DB_PASSWORD": password,
	}, testSecretData(t, kubeClient, testDbModuleNamespace, ModuleDatabaseCredsSecretName))

	// installed again, the module keeps its password
	again, err := EnsureModuleDatabaseUser(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)
	assert.Equal(t, digest, again)
	assert.Equal(t, password, testSecretData(t, kubeClient, testDbModuleNamespace, ModuleDatabaseCredsSecretName)["DB_PASSWORD"])

	// with its kept password deleted, it gets a new one, and its pods a new digest
	require.NoError(t, kubeClient.Resource(secretGVR).Namespace(testDbThreeportNamespace).Delete(
		context.Background(), "db-module-user-acme-widget", metav1.DeleteOptions{},
	))
	rotated, err := EnsureModuleDatabaseUser(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)
	assert.NotEqual(t, digest, rotated)
	newPassword := testSecretData(t, kubeClient, testDbModuleNamespace, ModuleDatabaseCredsSecretName)["DB_PASSWORD"]
	assert.NotEqual(t, password, newPassword)
	assert.Contains(t, (*jobs)[2].script, "PASSWORD '"+newPassword+"'")
}

// TestAModulesDatabaseUserIsDroppedAlone covers dropping a module's user:
// the statements drop it alone, only when it exists, and its kept password
// is deleted.
func TestAModulesDatabaseUserIsDroppedAlone(t *testing.T) {
	jobs := stubModuleDatabaseJobs(t)
	kubeClient := moduleDatabaseTestClient()
	_, err := EnsureModuleDatabaseUser(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)

	require.NoError(t, DropModuleDatabaseUser(kubeClient, testDbThreeportNamespace, "acme-widget"))
	require.Len(t, *jobs, 2)
	drop := (*jobs)[1]
	assert.Equal(t, "threeport-db-drop-acme-widget", drop.name)
	assert.Equal(t, testDbThreeportNamespace, drop.namespace)
	assert.Contains(t, drop.script, "WHERE username = 'threeport_acme_widget'")
	assert.Contains(t, drop.script, "DROP USER IF EXISTS threeport_acme_widget")
	assert.NotContains(t, drop.script, "DROP USER IF EXISTS threeport;")
	assert.Nil(t, testSecretData(t, kubeClient, testDbThreeportNamespace, "db-module-user-acme-widget"))

	require.NoError(t, DropModuleDatabaseUser(kubeClient, testDbThreeportNamespace, "acme-widget"),
		"a user already dropped")
}

// TestAModuleDatabaseJobRunsAsRootInTheControlPlane covers the Job that
// creates or drops a module's user: in the control plane's namespace, with
// the database's own certificates, admitted by the database's network
// policy, as an unprivileged user, its script and password in a secret
// rather than its spec.
func TestAModuleDatabaseJobRunsAsRootInTheControlPlane(t *testing.T) {
	job := moduleDatabaseJob(testDbThreeportNamespace, "threeport-db-user-acme")
	assert.Equal(t, testDbThreeportNamespace, job.GetNamespace())

	labels, _, _ := unstructured.NestedStringMap(job.Object, "spec", "template", "metadata", "labels")
	assert.Equal(t, "true", labels[LabelMaintenance])
	runAsNonRoot, _, _ := unstructured.NestedBool(job.Object, "spec", "template", "spec", "securityContext", "runAsNonRoot")
	assert.True(t, runAsNonRoot)

	volumes, _, _ := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "volumes")
	secrets := map[string]bool{}
	for _, volume := range volumes {
		name, _, _ := unstructured.NestedString(volume.(map[string]interface{}), "secret", "secretName")
		secrets[name] = true
	}
	assert.Equal(t, map[string]bool{dbCredsSecretName: true, "threeport-db-user-acme": true}, secrets)

	containers, _, _ := unstructured.NestedSlice(job.Object, "spec", "template", "spec", "containers")
	require.Len(t, containers, 1)
	command, _, _ := unstructured.NestedStringSlice(containers[0].(map[string]interface{}), "command")
	assert.Equal(t, []string{"/bin/sh", "/threeport/module-database/script.sh"}, command)

	assert.Equal(t, "threeport-db-user-"+strings.Repeat("a", 45), moduleDatabaseJobName("user", strings.Repeat("a", 60)),
		"a Job's name fits the label its pods carry")
}

// TestAModulesDatabaseEnvComesFromItsCredentials covers the environment a
// module's API server and migrator connect with.
func TestAModulesDatabaseEnvComesFromItsCredentials(t *testing.T) {
	env := ModuleDatabaseEnv()
	require.Len(t, env, 2)
	for i, key := range []string{"DB_USER", "DB_PASSWORD"} {
		variable := env[i].(map[string]interface{})
		assert.Equal(t, key, variable["name"])
		ref, _, _ := unstructured.NestedStringMap(variable, "valueFrom", "secretKeyRef")
		assert.Equal(t, map[string]string{"name": ModuleDatabaseCredsSecretName, "key": key}, ref)
	}
}

// TestARedeployedModuleKeepsHowItConnects covers a redeploy onto another
// control plane: a module installed before modules had users of their own
// gets the database's client certificates copied, as before, and one with a
// user of its own gets one in the new control plane's database, and no
// certificate.
func TestARedeployedModuleKeepsHowItConnects(t *testing.T) {
	jobs := stubModuleDatabaseJobs(t)
	kubeClient := moduleDatabaseTestClient()

	legacy, err := redeployedModuleSecrets(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)
	assert.Equal(t, append(ModuleCopiedSecretNames(), dbRootCertSecretName, dbThreeportCertSecretName), legacy)
	assert.Empty(t, *jobs, "no user is created for a module that doesn't use one")

	_, err = EnsureModuleDatabaseUser(kubeClient, "earlier-control-plane", testDbModuleNamespace, "acme-widget")
	if err != nil {
		// the earlier control plane's certificates aren't in this client
		require.ErrorContains(t, err, "database's CA certificate")
	}
	creds := testSecretData(t, kubeClient, testDbModuleNamespace, ModuleDatabaseCredsSecretName)
	if creds == nil {
		_, err = EnsureModuleDatabaseUser(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
		require.NoError(t, err)
	}
	*jobs = nil

	// an upgrade that failed after writing the user's secret, but before
	// the Deployment took it, leaves pods on the client certificates, so
	// they're still copied
	deployments := kubeClient.Resource(deploymentGVR).Namespace(testDbModuleNamespace)
	_, err = deployments.Create(context.Background(), moduleDeployment(false), metav1.CreateOptions{})
	require.NoError(t, err)
	stranded, err := redeployedModuleSecrets(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)
	assert.Equal(t, append(ModuleCopiedSecretNames(), dbRootCertSecretName, dbThreeportCertSecretName), stranded,
		"the secret alone doesn't mean the pods use it")
	assert.Empty(t, *jobs)

	_, err = deployments.Update(context.Background(), moduleDeployment(true), metav1.UpdateOptions{})
	require.NoError(t, err)

	own, err := redeployedModuleSecrets(kubeClient, testDbThreeportNamespace, testDbModuleNamespace, "acme-widget")
	require.NoError(t, err)
	assert.Equal(t, ModuleCopiedSecretNames(), own)
	require.Len(t, *jobs, 1, "the user is created in the new control plane's database")
	assert.Equal(t, testDbThreeportNamespace, (*jobs)[0].namespace)
}
