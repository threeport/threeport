package v0

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	messagebroker "github.com/threeport/threeport/pkg/messagebroker/v0"
)

const testControlPlaneNamespace = "threeport-control-plane"

// jsonDynamicClient stores what it creates and updates as JSON would, since
// the fake dynamic client cannot copy the plain ints the installer's
// manifests hold.
type jsonDynamicClient struct {
	*dynamicfake.FakeDynamicClient
}

func (c jsonDynamicClient) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return jsonResource{c.FakeDynamicClient.Resource(gvr)}
}

type jsonResource struct {
	dynamic.NamespaceableResourceInterface
}

func (r jsonResource) Namespace(namespace string) dynamic.ResourceInterface {
	return jsonNamespacedResource{r.NamespaceableResourceInterface.Namespace(namespace)}
}

type jsonNamespacedResource struct {
	dynamic.ResourceInterface
}

func (r jsonNamespacedResource) Create(
	ctx context.Context,
	obj *unstructured.Unstructured,
	options metav1.CreateOptions,
	subresources ...string,
) (*unstructured.Unstructured, error) {
	return r.ResourceInterface.Create(ctx, jsonCopy(obj), options, subresources...)
}

func (r jsonNamespacedResource) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	options metav1.UpdateOptions,
	subresources ...string,
) (*unstructured.Unstructured, error) {
	return r.ResourceInterface.Update(ctx, jsonCopy(obj), options, subresources...)
}

// jsonCopy returns obj as it reads back from JSON.
func jsonCopy(obj *unstructured.Unstructured) *unstructured.Unstructured {
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		panic(err)
	}
	copied := &unstructured.Unstructured{}
	if err := copied.UnmarshalJSON(raw); err != nil {
		panic(err)
	}

	return copied
}

// newBrokerTestClient returns a fake cluster seeded with objects, and the
// installer of a control plane in it.
func newBrokerTestClient(objects ...runtime.Object) (*dynamicfake.FakeDynamicClient, dynamic.Interface, *ControlPlaneInstaller) {
	fake := testKubeClient(objects...)
	cpi := &ControlPlaneInstaller{Opts: Options{
		Namespace:     testControlPlaneNamespace,
		InfraProvider: v0.KubernetesRuntimeInfraProviderEKS,
		RestApiInfo:   &v0.ControlPlaneComponent{ServiceResourceName: ThreeportAPIServiceResourceName},
	}}

	return fake, jsonDynamicClient{fake}, cpi
}

// installerTestMapper maps every kind the control plane's install writes.
func installerTestMapper() *meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Version: "v1"}, {Group: "apps", Version: "v1"}, {Group: "policy", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	for _, kind := range []schema.GroupVersionKind{
		{Version: "v1", Kind: "Secret"},
		{Version: "v1", Kind: "ConfigMap"},
		{Version: "v1", Kind: "Service"},
		{Version: "v1", Kind: "ServiceAccount"},
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
	} {
		mapper.Add(kind, meta.RESTScopeNamespace)
	}
	var restMapper meta.RESTMapper = mapper

	return &restMapper
}

// getTestObject returns an object of the control plane namespace.
func getTestObject(t *testing.T, kubeClient dynamic.Interface, gvr schema.GroupVersionResource, name string) *unstructured.Unstructured {
	t.Helper()

	obj, err := kubeClient.Resource(gvr).Namespace(testControlPlaneNamespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err, "expected %s %s to exist", gvr.Resource, name)

	return obj
}

// testSecretValue returns one value of a secret the fake cluster holds.
func testSecretValue(t *testing.T, kubeClient dynamic.Interface, name, key string) string {
	t.Helper()

	value, err := secretValue(getTestObject(t, kubeClient, secretGVR, name), key)
	require.NoError(t, err)

	return value
}

// testPodSpec returns the pod spec of a deployment or stateful set.
func testPodSpec(t *testing.T, obj *unstructured.Unstructured) map[string]interface{} {
	t.Helper()

	spec, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	require.NoError(t, err)
	require.True(t, found, "expected %s to have a pod spec", obj.GetName())

	return spec
}

// testContainer returns the named container of a pod spec.
func testContainer(t *testing.T, podSpec map[string]interface{}, name string) map[string]interface{} {
	t.Helper()

	containers, _, _ := unstructured.NestedSlice(podSpec, "containers")
	for _, c := range containers {
		container := c.(map[string]interface{})
		if container["name"] == name {
			return container
		}
	}
	t.Fatalf("expected a container named %s", name)

	return nil
}

// assertMountsSecret asserts that a pod spec mounts secret at mountPath in
// the named container.
func assertMountsSecret(t *testing.T, podSpec map[string]interface{}, container, secret, mountPath string) {
	t.Helper()

	volumeName := ""
	volumes, _, _ := unstructured.NestedSlice(podSpec, "volumes")
	for _, v := range volumes {
		volume := v.(map[string]interface{})
		if name, _, _ := unstructured.NestedString(volume, "secret", "secretName"); name == secret {
			volumeName = volume["name"].(string)
		}
	}
	require.NotEmpty(t, volumeName, "expected a volume of secret %s", secret)

	mounts, _, _ := unstructured.NestedSlice(testContainer(t, podSpec, container), "volumeMounts")
	for _, m := range mounts {
		mount := m.(map[string]interface{})
		if mount["name"] == volumeName {
			assert.Equal(t, mountPath, mount["mountPath"], "secret %s is mounted elsewhere", secret)
			return
		}
	}
	t.Errorf("expected container %s to mount secret %s", container, secret)
}

// assertEnvFromCredentials asserts that a container sets an environment
// variable from a key of the credentials secret.
func assertEnvFromCredentials(t *testing.T, container map[string]interface{}, name, key string) {
	t.Helper()

	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range env {
		variable := e.(map[string]interface{})
		if variable["name"] != name {
			continue
		}
		secret, _, _ := unstructured.NestedString(variable, "valueFrom", "secretKeyRef", "name")
		secretKey, _, _ := unstructured.NestedString(variable, "valueFrom", "secretKeyRef", "key")
		assert.Equal(t, natsCredentialsSecretName, secret, "%s comes from another secret", name)
		assert.Equal(t, key, secretKey, "%s comes from another key", name)
		return
	}
	t.Errorf("expected container %s to set %s", container["name"], name)
}

// envValue returns the value of a container's environment variable.
func envValue(container map[string]interface{}, name string) string {
	env, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range env {
		variable := e.(map[string]interface{})
		if variable["name"] == name {
			value, _ := variable["value"].(string)
			return value
		}
	}

	return ""
}

// installTestDependencies installs the control plane's dependencies into
// the fake cluster.
func installTestDependencies(t *testing.T, kubeClient dynamic.Interface, cpi *ControlPlaneInstaller) {
	t.Helper()

	require.NoError(t, cpi.InstallThreeportControlPlaneDependencies(kubeClient, installerTestMapper(), "key", nil))
}

func TestTheFirstInstallCreatesTheBrokerCertificatesAndCredentials(t *testing.T) {
	_, kubeClient, cpi := newBrokerTestClient()

	require.NoError(t, cpi.ensureMessageBrokerSecrets(kubeClient))

	// every secret outlives a reinstall
	for _, name := range []string{natsCASecretName, natsServerCertSecretName, NatsCACertSecretName, natsCredentialsSecretName} {
		secret := getTestObject(t, kubeClient, secretGVR, name)
		assert.True(t, isPersistent(secret), "secret %s is deleted by a reinstall", name)
		assert.Equal(t, LabelManagedByValue, secret.GetLabels()[LabelManagedBy], "secret %s is not installer-managed", name)
	}

	// the credentials
	assert.Equal(t, NatsUser, testSecretValue(t, kubeClient, natsCredentialsSecretName, "user"))
	password := testSecretValue(t, kubeClient, natsCredentialsSecretName, "password")
	assert.True(t, strings.HasPrefix(password, "tp"), "the password must start with letters so the server reads it as a string")
	assert.GreaterOrEqual(t, len(password), 40, "the password is too short")

	// clients get the CA's certificate and never its key
	clientCA := getTestObject(t, kubeClient, secretGVR, NatsCACertSecretName)
	clientCAData, _, _ := unstructured.NestedMap(clientCA.Object, "stringData")
	assert.Equal(t, []string{"ca.crt"}, mapKeys(clientCAData), "clients must only get the CA's certificate")
	assert.Equal(
		t,
		testSecretValue(t, kubeClient, natsCASecretName, "tls.crt"),
		testSecretValue(t, kubeClient, NatsCACertSecretName, "ca.crt"),
	)

	// the server's certificate is signed by that CA for every name a client
	// or a route dials
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM([]byte(testSecretValue(t, kubeClient, NatsCACertSecretName, "ca.crt"))))
	block, _ := pem.Decode([]byte(testSecretValue(t, kubeClient, natsServerCertSecretName, "tls.crt")))
	require.NotNil(t, block)
	serverCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	for _, name := range []string{
		"nats-js",
		"nats-js.threeport-control-plane",
		"nats-js.threeport-control-plane.svc",
		"nats-js.threeport-control-plane.svc.cluster.local",
		"nats-js-0.nats-js.threeport-control-plane.svc.cluster.local",
		"nats-js-2.nats-js",
		"localhost",
		"127.0.0.1",
	} {
		_, err := serverCert.Verify(x509.VerifyOptions{
			DNSName:   name,
			Roots:     roots,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		assert.NoError(t, err, "the server's certificate is not valid for %s", name)
	}
	_, err = serverCert.Verify(x509.VerifyOptions{DNSName: "nats-js.other-namespace.svc.cluster.local", Roots: roots})
	assert.Error(t, err, "the server's certificate must not be valid in another namespace")
}

func TestEveryLaterInstallReusesTheBrokerCertificatesAndCredentials(t *testing.T) {
	_, kubeClient, cpi := newBrokerTestClient()
	require.NoError(t, cpi.ensureMessageBrokerSecrets(kubeClient))

	before := map[string]string{}
	for _, item := range [][2]string{
		{natsCASecretName, "tls.key"},
		{natsServerCertSecretName, "tls.crt"},
		{NatsCACertSecretName, "ca.crt"},
		{natsCredentialsSecretName, "password"},
	} {
		before[item[0]] = testSecretValue(t, kubeClient, item[0], item[1])
	}

	require.NoError(t, cpi.ensureMessageBrokerSecrets(kubeClient))

	for _, item := range [][2]string{
		{natsCASecretName, "tls.key"},
		{natsServerCertSecretName, "tls.crt"},
		{NatsCACertSecretName, "ca.crt"},
		{natsCredentialsSecretName, "password"},
	} {
		assert.Equal(t, before[item[0]], testSecretValue(t, kubeClient, item[0], item[1]), "a later install replaced %s", item[0])
	}
}

func TestStoredCredentialsAreReadAsTheAPIServerReturnsThem(t *testing.T) {
	// the API server returns a secret's values base64-encoded under data
	stored := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      natsCredentialsSecretName,
			"namespace": testControlPlaneNamespace,
		},
		"data": map[string]interface{}{
			"user":     base64.StdEncoding.EncodeToString([]byte(NatsUser)),
			"password": base64.StdEncoding.EncodeToString([]byte("tpStored")),
		},
	}}
	_, kubeClient, cpi := newBrokerTestClient(stored)

	credentials, err := cpi.ensureNatsCredentials(kubeClient)
	require.NoError(t, err)
	assert.Equal(t, natsCredentials{User: NatsUser, Password: "tpStored"}, *credentials)

	credentials, err = cpi.messageBrokerCredentials(kubeClient)
	require.NoError(t, err)
	assert.Equal(t, "tpStored", credentials.Password)
}

func TestCredentialsWithoutAPasswordAreRefused(t *testing.T) {
	stored := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      natsCredentialsSecretName,
			"namespace": testControlPlaneNamespace,
		},
		"data": map[string]interface{}{
			"user":     base64.StdEncoding.EncodeToString([]byte(NatsUser)),
			"password": "",
		},
	}}
	_, kubeClient, cpi := newBrokerTestClient(stored)

	_, err := cpi.messageBrokerCredentials(kubeClient)
	assert.ErrorContains(t, err, "holds no user or password")
}

func TestAnInstallThatLosesTheRaceUsesTheCredentialsThatWon(t *testing.T) {
	fake, kubeClient, cpi := newBrokerTestClient()

	// another install creates the credentials between this one's read and
	// its create
	winner := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      natsCredentialsSecretName,
			"namespace": testControlPlaneNamespace,
		},
		"stringData": map[string]interface{}{"user": NatsUser, "password": "tpWinner"},
	}}
	fake.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		created := action.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if created.GetName() != natsCredentialsSecretName {
			return false, nil, nil
		}
		require.NoError(t, fake.Tracker().Add(winner))
		return true, nil, k8serrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, created.GetName())
	})

	credentials, err := cpi.ensureNatsCredentials(kubeClient)
	require.NoError(t, err)
	assert.Equal(t, "tpWinner", credentials.Password, "an install must use the credentials the broker was given")
}

func TestClientsRefuseToInstallBeforeTheBrokerHasCredentials(t *testing.T) {
	_, kubeClient, cpi := newBrokerTestClient()

	_, err := cpi.messageBrokerCredentials(kubeClient)
	assert.ErrorContains(t, err, "does not exist")
}

func TestTheBrokerOnlyLetsTheThreeportUserInOverTLS(t *testing.T) {
	_, kubeClient, cpi := newBrokerTestClient()
	installTestDependencies(t, kubeClient, cpi)

	// the server mounts its certificate where its config reads it, and takes
	// the credentials from their secret
	statefulSet := getTestObject(t, kubeClient, statefulSetGVR, natsServiceName)
	podSpec := testPodSpec(t, statefulSet)
	assertMountsSecret(t, podSpec, "nats", natsServerCertSecretName, natsServerCertsPath)
	// the reloader reads the config too, and exits when a file it names is missing
	assertMountsSecret(t, podSpec, "reloader", natsServerCertSecretName, natsServerCertsPath)
	nats := testContainer(t, podSpec, "nats")
	assertEnvFromCredentials(t, nats, "NATS_USER", "user")
	assertEnvFromCredentials(t, nats, "NATS_PASSWORD", "password")
	assert.True(t, isPersistent(statefulSet), "a reinstall must keep the broker's stateful set")

	// start a server from the config the cluster holds, with the secrets the
	// pod mounts and the environment it sets
	configMap := getTestObject(t, kubeClient, schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, "nats-js-config")
	config, _, _ := unstructured.NestedString(configMap.Object, "data", "nats.conf")
	require.Contains(t, config, natsServerCertsPath+"/tls.crt")
	require.NotContains(t, config, testSecretValue(t, kubeClient, natsCredentialsSecretName, "password"), "the password must not sit in the config map")

	certs := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certs, "tls.crt"), []byte(testSecretValue(t, kubeClient, natsServerCertSecretName, "tls.crt")), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(certs, "tls.key"), []byte(testSecretValue(t, kubeClient, natsServerCertSecretName, "tls.key")), 0600))
	configFile := filepath.Join(t.TempDir(), "nats.conf")
	require.NoError(t, os.WriteFile(configFile, []byte(strings.ReplaceAll(config, natsServerCertsPath, certs)), 0600))

	password := testSecretValue(t, kubeClient, natsCredentialsSecretName, "password")
	t.Setenv("POD_NAME", "nats-js-0")
	t.Setenv("NATS_USER", testSecretValue(t, kubeClient, natsCredentialsSecretName, "user"))
	t.Setenv("NATS_PASSWORD", password)

	opts, err := server.ProcessConfigFile(configFile)
	require.NoError(t, err, "the server cannot read its config")
	assert.NotNil(t, opts.TLSConfig, "the server must require TLS")
	assert.Equal(t, NatsUser, opts.Username)
	assert.Equal(t, password, opts.Password, "the server must read the password from its environment")
	opts.Host, opts.Port, opts.HTTPPort = "127.0.0.1", -1, 0
	opts.StoreDir, opts.PidFile = t.TempDir(), ""
	opts.NoLog, opts.NoSigs = true, true
	srv, err := server.NewServer(opts)
	require.NoError(t, err)
	go srv.Start()
	t.Cleanup(srv.Shutdown)
	require.True(t, srv.ReadyForConnections(10*time.Second))
	_, port, err := net.SplitHostPort(srv.Addr().String())
	require.NoError(t, err)

	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, []byte(testSecretValue(t, kubeClient, NatsCACertSecretName, "ca.crt")), 0600))

	// a client connects with what the installer gives it
	conn, err := messagebroker.Connect(messagebroker.Config{Host: "localhost", Port: port, User: NatsUser, Password: password, CAFile: caFile})
	require.NoError(t, err, "a client with the CA's certificate and the credentials must connect")
	js, err := conn.JetStream()
	require.NoError(t, err)
	_, err = js.AccountInfo()
	assert.NoError(t, err, "JetStream must be enabled")
	conn.Close()

	// and every other client is turned away
	for name, config := range map[string]messagebroker.Config{
		"no credentials": {Host: "localhost", Port: port, CAFile: caFile},
		"wrong password": {Host: "localhost", Port: port, User: NatsUser, Password: "tpWrong", CAFile: caFile},
		"no TLS":         {Host: "localhost", Port: port, User: NatsUser, Password: password},
	} {
		conn, err := messagebroker.Connect(config, natsTestTimeout())
		if conn != nil {
			conn.Close()
		}
		assert.Error(t, err, "a client with %s must be turned away", name)
	}
}

func TestTheNatsBoxConnectsOverTLSAsTheThreeportUser(t *testing.T) {
	_, kubeClient, cpi := newBrokerTestClient()
	installTestDependencies(t, kubeClient, cpi)

	podSpec := testPodSpec(t, getTestObject(t, kubeClient, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "nats-js-box"))
	assertMountsSecret(t, podSpec, "nats-box", NatsCACertSecretName, NatsClientCAPath)
	box := testContainer(t, podSpec, "nats-box")
	assert.Equal(t, "tls://nats-js:4222", envValue(box, "NATS_URL"))
	assert.Equal(t, NatsClientCAFile, envValue(box, "NATS_CA"))
	assertEnvFromCredentials(t, box, "NATS_USER", "user")
	assertEnvFromCredentials(t, box, "NATS_PASSWORD", "password")
}

func TestAnExistingBrokerRollsOntoTheNewTemplateAndKeepsItsVolumes(t *testing.T) {
	// a broker installed before it required TLS, scaled by hand
	existing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":            natsServiceName,
			"namespace":       testControlPlaneNamespace,
			"resourceVersion": "7",
			"labels":          map[string]interface{}{LabelPersistent: LabelPersistentValue},
		},
		"spec": map[string]interface{}{
			"replicas":    int64(3),
			"serviceName": natsServiceName,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{map[string]interface{}{"name": "nats", "image": "nats:old"}},
				},
			},
			"volumeClaimTemplates": []interface{}{map[string]interface{}{
				"metadata": map[string]interface{}{"name": "datadir"},
				"spec": map[string]interface{}{
					"storageClassName": "kept",
					"volumeMode":       "Filesystem",
				},
			}},
		},
	}}
	_, kubeClient, cpi := newBrokerTestClient(existing)
	cpi.Opts.CreateOrUpdateKubeResources = true
	installTestDependencies(t, kubeClient, cpi)

	statefulSet := getTestObject(t, kubeClient, statefulSetGVR, natsServiceName)
	podSpec := testPodSpec(t, statefulSet)
	assertMountsSecret(t, podSpec, "nats", natsServerCertSecretName, natsServerCertsPath)
	assert.NotEqual(t, "nats:old", testContainer(t, podSpec, "nats")["image"], "the broker must roll onto the new template")

	replicas, _, _ := unstructured.NestedInt64(statefulSet.Object, "spec", "replicas")
	assert.Equal(t, int64(3), replicas, "the replicas must be kept")
	claims, _, _ := unstructured.NestedSlice(statefulSet.Object, "spec", "volumeClaimTemplates")
	require.Len(t, claims, 1)
	storageClass, _, _ := unstructured.NestedString(claims[0].(map[string]interface{}), "spec", "storageClassName")
	assert.Equal(t, "kept", storageClass, "the volume claim templates cannot change and must be kept")
}

func TestTheAPIServerConnectsOverTLSAsTheThreeportUser(t *testing.T) {
	// the database certificates exist, as the install before the API's left them
	dbCerts := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": dbRootCertSecretName, "namespace": testControlPlaneNamespace},
	}}
	_, kubeClient, cpi := newBrokerTestClient(dbCerts)
	require.NoError(t, cpi.ensureMessageBrokerSecrets(kubeClient))
	cpi.Opts.RestApiInfo.Name = "rest-api"
	cpi.Opts.RestApiInfo.BinaryName = "rest-api"
	cpi.Opts.DatabaseMigratorInfo = &v0.ControlPlaneComponent{Name: "database-migrator", BinaryName: "database-migrator"}

	require.NoError(t, cpi.UpdateThreeportAPIDeployment(kubeClient, installerTestMapper(), nil))

	env, _ := secretValue(getTestObject(t, kubeClient, secretGVR, ThreeportApiConfigSecret), "env")
	password := testSecretValue(t, kubeClient, natsCredentialsSecretName, "password")
	for _, line := range []string{
		"NATS_HOST=nats-js.threeport-control-plane.svc.cluster.local",
		"NATS_USER=" + NatsUser,
		"NATS_PASSWORD=" + password,
		"NATS_CA_FILE=" + NatsClientCAFile,
	} {
		assert.Contains(t, strings.Split(env, "\n"), line)
	}

	podSpec := testPodSpec(t, getTestObject(t, kubeClient, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, ThreeportAPIServiceResourceName))
	assertMountsSecret(t, podSpec, "api-server", NatsCACertSecretName, NatsClientCAPath)
}

func TestTheControllersConnectOverTLSAsTheThreeportUser(t *testing.T) {
	_, _, cpi := newBrokerTestClient()

	secret := cpi.getControllerSecret("controller", testControlPlaneNamespace, &natsCredentials{User: NatsUser, Password: "tpSecret"})
	values, _, _ := unstructured.NestedStringMap(secret.Object, "stringData")
	assert.Equal(t, NatsUser, values["MSG_BROKER_USER"])
	assert.Equal(t, "tpSecret", values["MSG_BROKER_PASSWORD"])
	assert.Equal(t, NatsClientCAFile, values["MSG_BROKER_CA_FILE"])

	deployment, err := cpi.getControllerDeployment("threeport-secret-controller", testControlPlaneNamespace, v0.ControlPlaneComponent{Name: "secret-controller"})
	require.NoError(t, err)
	assertMountsSecret(t, testPodSpec(t, jsonCopy(deployment)), "secret-controller", NatsCACertSecretName, NatsClientCAPath)
}

func TestTheDropJobConnectsToTheBrokerItFinds(t *testing.T) {
	// the job's credentials and CA are optional, since a broker installed
	// before it required TLS has none
	for _, variable := range natsDropJobEnv() {
		variable := variable.(map[string]interface{})
		if ref, found, _ := unstructured.NestedMap(variable, "valueFrom", "secretKeyRef"); found {
			assert.Equal(t, true, ref["optional"], "%s must be optional", variable["name"])
		}
	}
	optional, _, _ := unstructured.NestedBool(natsDropJobCAVolume(), "secret", "optional")
	assert.True(t, optional, "the CA's certificate must be optional")
	// the client volume itself stays required
	_, found, _ := unstructured.NestedBool(NatsClientCAVolume(), "secret", "optional")
	assert.False(t, found, "the clients' CA volume must stay required")

	// a fake nats CLI that records how it was asked to connect, and plays a
	// broker that serves TLS, or not, as BROKER_TLS says: a broker that does
	// refuses a plain connection, and one that does not refuses TLS.  It
	// refuses as many TLS connections as TLS_FAILURES holds first, and fails
	// to list the streams when LIST_FAILS is set.
	bin := t.TempDir()
	scratch := t.TempDir()
	log := filepath.Join(scratch, "nats.log")
	tlsFailures := filepath.Join(scratch, "tls-failures")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "nats"), []byte(`#!/bin/sh
echo "$NATS_URL user=$NATS_USER $*" >> "$NATS_LOG"
case "$NATS_URL" in
tls://*)
  [ "$BROKER_TLS" = yes ] || exit 1
  left=$(cat "$TLS_FAILURES")
  if [ "$left" -gt 0 ]; then
    echo $((left - 1)) > "$TLS_FAILURES"
    exit 1
  fi
  ;;
*)
  [ "$BROKER_TLS" = yes ] && exit 1
  ;;
esac
if [ "$1 $2" = "stream ls" ]; then
  [ -n "$LIST_FAILS" ] && exit 1
  echo stream-a
fi
exit 0
`), 0700))
	// the job's waits between attempts are skipped
	require.NoError(t, os.WriteFile(filepath.Join(bin, "sleep"), []byte("#!/bin/sh\nexit 0\n"), 0700))

	run := func(failures string, env ...string) (string, error) {
		require.NoError(t, os.WriteFile(log, nil, 0600))
		require.NoError(t, os.WriteFile(tlsFailures, []byte(failures), 0600))
		cmd := exec.Command("/bin/sh", "-c", natsDropJobScript)
		cmd.Env = append([]string{
			"PATH=" + bin + ":/usr/bin:/bin",
			"NATS_LOG=" + log,
			"TLS_FAILURES=" + tlsFailures,
			"NATS_URL=tls://nats-js:4222",
		}, env...)
		out, err := cmd.CombinedOutput()
		calls, readErr := os.ReadFile(log)
		require.NoError(t, readErr)

		return string(calls) + string(out), err
	}
	credentials := []string{"NATS_USER=threeport", "NATS_PASSWORD=tpSecret"}

	// a broker that requires TLS is reached over TLS with the credentials
	calls, err := run("0", append(credentials, "BROKER_TLS=yes")...)
	require.NoError(t, err, calls)
	assert.Contains(t, calls, "tls://nats-js:4222 user=threeport stream rm stream-a --force")
	assert.NotContains(t, calls, "nats://nats-js:4222 user= stream")

	// a broker that requires TLS but refuses the first connections, as while
	// it restarts, is still reached over TLS
	calls, err = run("3", append(credentials, "BROKER_TLS=yes")...)
	require.NoError(t, err, calls)
	assert.Contains(t, calls, "tls://nats-js:4222 user=threeport stream rm stream-a --force")
	assert.NotContains(t, calls, "without TLS")

	// a broker from before TLS, which has no credentials secret
	calls, err = run("0")
	require.NoError(t, err, calls)
	assert.Contains(t, calls, "nats://nats-js:4222 user= stream rm stream-a --force")
	assert.NotContains(t, calls, "tls://")

	// a broker that has credentials but has not rolled onto TLS yet
	calls, err = run("0", append(credentials, "BROKER_TLS=no")...)
	require.NoError(t, err, calls)
	assert.Contains(t, calls, "nats://nats-js:4222 user= stream rm stream-a --force")

	// a broker reached neither way fails the job, without removing anything
	calls, err = run("100", append(credentials, "BROKER_TLS=yes")...)
	assert.Error(t, err, calls)
	assert.Contains(t, calls, "failed to connect to the message broker")
	assert.Equal(t, 10, strings.Count(calls, "tls://nats-js:4222 user=threeport account info"), calls)
	assert.NotContains(t, calls, "stream")

	// a failed listing of the streams fails the job too
	calls, err = run("0", append(credentials, "BROKER_TLS=yes", "LIST_FAILS=yes")...)
	assert.Error(t, err, calls)
	assert.NotContains(t, calls, "stream rm")
}

// natsTestTimeout keeps a refused connection from waiting long.
func natsTestTimeout() func(*nats.Options) error {
	return nats.Timeout(2 * time.Second)
}

// mapKeys returns the keys of m.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	return keys
}
