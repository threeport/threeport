package v0

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestRedeployInstalledModuleSkipsCore checks that a core module is not retargeted.
func TestRedeployInstalledModuleSkipsCore(t *testing.T) {
	// return before any cluster call
	cpi := &ControlPlaneInstaller{}
	err := cpi.redeployInstalledModule(nil, nil, api.ModuleApi{Core: util.Ptr(true)}, nil, time.Time{})
	require.NoError(t, err)
}

// TestTrustBundleKeepsBothAuthorities joins the API CA and the broker CA.
func TestTrustBundleKeepsBothAuthorities(t *testing.T) {
	apiCA := base64.StdEncoding.EncodeToString([]byte("api"))
	natsCA := base64.StdEncoding.EncodeToString([]byte("nats"))
	got, err := trustBundle(apiCA, natsCA)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := base64.StdEncoding.DecodeString(got)
	if err != nil || string(plain) != "api\nnats" {
		t.Fatalf("bundle = %q", plain)
	}
}

// TestRetargetControlPlaneHostRewritesTheDatabase points a restored address at the replacement.
func TestRetargetControlPlaneHostRewritesTheDatabase(t *testing.T) {
	// the database host names the plane the module was installed on
	got := retargetControlPlaneHost(
		"cockroach sql --host crdb.old-plane.svc.cluster.local",
		"new-plane",
	)
	if got != "cockroach sql --host crdb.new-plane.svc.cluster.local" {
		t.Fatalf("host = %s", got)
	}
}

// TestModuleDeploymentNamesUsesTheInstalledName checks the module-prefixed deployment name.
func TestModuleDeploymentNamesUsesTheInstalledName(t *testing.T) {
	// the install puts the module in front of the recorded controller name
	names := moduleDeploymentNames("threeport-core-controller", "acme")
	if len(names) != 2 || names[1] != "threeport-acme-core-controller" {
		t.Fatalf("names = %v", names)
	}
}

// TestRecordedModuleImages checks when a retarget is allowed to change images.
func TestRecordedModuleImages(t *testing.T) {
	// require both values and skip a core module
	namespace, tag := recordedModuleImages(&api.ModuleApi{
		ImageNamespace: util.Ptr("repo.example"),
		ImageTag:       util.Ptr("v1"),
	})
	assert.Equal(t, "repo.example", namespace)
	assert.Equal(t, "v1", tag)

	namespace, tag = recordedModuleImages(&api.ModuleApi{
		Core:           util.Ptr(true),
		ImageNamespace: util.Ptr("repo.example"),
		ImageTag:       util.Ptr("v1"),
	})
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", tag)

	namespace, tag = recordedModuleImages(&api.ModuleApi{
		ImageNamespace: util.Ptr("repo.example"),
	})
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", tag)
}

// TestSplitDeploymentName checks a recorded controller deployment reference.
func TestSplitDeploymentName(t *testing.T) {
	// accept namespace/name and reject anything else
	namespace, name, err := splitDeploymentName("acme/threeport-acme-widget-controller")
	require.NoError(t, err)
	assert.Equal(t, "acme", namespace)
	assert.Equal(t, "threeport-acme-widget-controller", name)

	_, _, err = splitDeploymentName("threeport-acme-widget-controller")
	require.Error(t, err)
}

// TestClientCredentialRejectsAMismatch refuses a certificate that does not match its key.
func TestClientCredentialRejectsAMismatch(t *testing.T) {
	// decode succeeds and the pair does not load
	err := clientCredential(
		base64.StdEncoding.EncodeToString([]byte("not-a-cert")),
		base64.StdEncoding.EncodeToString([]byte("not-a-key")),
	)
	require.Error(t, err)
}

// TestBrokerCAEntriesKeepsTheCertificate copies the keys the module reads.
func TestBrokerCAEntriesKeepsTheCertificate(t *testing.T) {
	// drop a key the broker secret does not serve to the module
	got := brokerCAEntries(map[string]string{"tls.crt": "cert", "tls.key": "key", "extra": "no"})
	require.Equal(t, map[string]string{"tls.crt": "cert", "tls.key": "key"}, got)
}

// TestEnsureBrokerCAMountsTheSecret adds the broker CA once.
func TestEnsureBrokerCAMountsTheSecret(t *testing.T) {
	// a module deployment has no broker volume yet
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{"name": "api"},
					},
				},
			},
		},
	}}
	require.True(t, ensureBrokerCA(deployment))
	require.False(t, ensureBrokerCA(deployment))

	volumes, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "volumes")
	require.NoError(t, err)
	require.True(t, namedObject(volumes, ModuleBrokerCAVolumeName))
	containers, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	mounts, _, err := unstructured.NestedSlice(containers[0].(map[string]interface{}), "volumeMounts")
	require.NoError(t, err)
	require.True(t, mountNamed(mounts, NatsClientCAPath))
	require.True(t, mountNamed(mounts, ModuleBrokerCASystemFile))
}

// TestEnsureBrokerCALeavesTheInstallMount checks the mounts a module install
// creates are the ones the reconcile looks for.
func TestEnsureBrokerCALeavesTheInstallMount(t *testing.T) {
	// a module install already has the volume and both mounts
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{ModuleBrokerCAVolume()},
					"containers": []interface{}{
						map[string]interface{}{
							"name": "api",
							"volumeMounts": []interface{}{
								ModuleBrokerCAVolumeMount(),
								ModuleBrokerCASystemVolumeMount(),
							},
						},
					},
				},
			},
		},
	}}
	require.False(t, ensureBrokerCA(deployment))
}

// TestMissingModuleNamespaceIsAbsent covers a removed module namespace.
func TestMissingModuleNamespaceIsAbsent(t *testing.T) {
	present, err := moduleNamespacePresent(testKubeClient(), "threeport-example")
	require.NoError(t, err)
	require.False(t, present)
}

// TestRejectExistingModuleNamespaceReportsAnotherOwner covers the namespace check.
func TestRejectExistingModuleNamespaceReportsAnotherOwner(t *testing.T) {
	// an absent namespace is free to install into
	apiDeployment := moduleAPIDeploymentName("example")
	require.NoError(t, RejectExistingModuleNamespace(
		testKubeClient(), "threeport-example", "primary-test", apiDeployment,
	))

	// a namespace with no owner label and no API deployment is taken
	err := RejectExistingModuleNamespace(
		testKubeClient(testModuleNamespace("threeport-example", "uid")),
		"threeport-example",
		"primary-test",
		apiDeployment,
	)
	require.ErrorIs(t, err, ErrModuleNamespaceExists)

	// a namespace labeled for another control plane is taken
	other := testModuleNamespace("threeport-example", "uid")
	other.SetLabels(map[string]string{ModuleNamespaceOwnerLabel: "genesis-test"})
	err = RejectExistingModuleNamespace(
		testKubeClient(other, testNamedDeployment(
			"threeport-example", apiDeployment, controlPlaneDatabaseHost("primary-test"),
		)),
		"threeport-example",
		"primary-test",
		apiDeployment,
	)
	require.ErrorIs(t, err, ErrModuleNamespaceExists)

	// a namespace this control plane labeled is left for the install
	owned := testModuleNamespace("threeport-example", "uid")
	owned.SetLabels(map[string]string{ModuleNamespaceOwnerLabel: "primary-test"})
	require.NoError(t, RejectExistingModuleNamespace(
		testKubeClient(owned), "threeport-example", "primary-test", apiDeployment,
	))
}

// TestRejectExistingModuleNamespaceAdoptsAnUnlabeledModule covers a namespace
// created before installs wrote an owner label.
func TestRejectExistingModuleNamespaceAdoptsAnUnlabeledModule(t *testing.T) {
	// an unlabeled namespace whose API deployment names this database is left for the install
	apiDeployment := moduleAPIDeploymentName("example")
	host := controlPlaneDatabaseHost("primary-test")
	require.NoError(t, RejectExistingModuleNamespace(
		testKubeClient(
			testModuleNamespace("threeport-example", "uid"),
			testNamedDeployment("threeport-example", apiDeployment, host),
		),
		"threeport-example",
		"primary-test",
		apiDeployment,
	))

	// the same API deployment for another control plane stays taken
	err := RejectExistingModuleNamespace(
		testKubeClient(
			testModuleNamespace("threeport-example", "uid"),
			testNamedDeployment(
				"threeport-example", apiDeployment, controlPlaneDatabaseHost("other-plane"),
			),
		),
		"threeport-example",
		"primary-test",
		apiDeployment,
	)
	require.ErrorIs(t, err, ErrModuleNamespaceExists)

	// a deployment that only names this database stays taken
	err = RejectExistingModuleNamespace(
		testKubeClient(
			testModuleNamespace("threeport-example", "uid"),
			testNamedDeployment("threeport-example", "other-module-api-server", host),
		),
		"threeport-example",
		"primary-test",
		apiDeployment,
	)
	require.ErrorIs(t, err, ErrModuleNamespaceExists)
}

// testNamedDeployment returns a deployment whose command names host.
func testNamedDeployment(namespace, name, host string) *unstructured.Unstructured {
	// name this database in the container command
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "api",
							"command": []interface{}{
								"cockroach sql --host " + host,
							},
						},
					},
				},
			},
		},
	}}
}

// TestRestartPointsTheDatabaseAtTheReplacement covers a module restart
// rewriting the database host stored in the init command.
func TestRestartPointsTheDatabaseAtTheReplacement(t *testing.T) {
	// seed a deployment whose init command still names the source plane
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "threeport-example-api-server",
			"namespace": "threeport-example",
		},
		"spec": map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"initContainers": []interface{}{
						map[string]interface{}{
							"name": "db-init",
							"command": []interface{}{
								"--host crdb.primary-12.svc.cluster.local",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name": "controller",
							"args": []interface{}{
								"-msg-broker-host=nats-js.primary-12.svc.cluster.local",
								"-msg-broker-user=old-user",
								"-msg-broker-password=old-password",
							},
							"env": []interface{}{
								map[string]interface{}{
									"name":  "NATS_USER",
									"value": "old-user",
								},
							},
						},
					},
				},
			},
		},
	}}
	kubeClient := testKubeClient(deployment)

	// restart the deployment onto the replacement plane
	err := restartModuleDeployment(
		kubeClient,
		"threeport-example",
		"threeport-example-api-server",
		"primary-13",
		map[string]string{},
		map[string]string{},
		time.Now(),
	)
	require.NoError(t, err)

	// check that the stored command names the replacement
	updated, err := kubeClient.Resource(deploymentGVR).Namespace("threeport-example").Get(
		context.Background(), "threeport-example-api-server", metav1.GetOptions{},
	)
	require.NoError(t, err)
	initContainers, _, err := unstructured.NestedSlice(updated.Object, "spec", "template", "spec", "initContainers")
	require.NoError(t, err)
	command, _, err := unstructured.NestedSlice(initContainers[0].(map[string]interface{}), "command")
	require.NoError(t, err)
	require.Contains(t, command[0], "crdb.primary-13.svc.cluster.local")
	containers, _, err := unstructured.NestedSlice(updated.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	args, _, err := unstructured.NestedStringSlice(containers[0].(map[string]interface{}), "args")
	require.NoError(t, err)
	require.Contains(t, args, "-msg-broker-host=nats-js.primary-13.svc.cluster.local")
	require.Contains(t, args, "-msg-broker-user=$(NATS_USER)")
	require.Contains(t, args, "-msg-broker-password=$(NATS_PASSWORD)")
	env, _, err := unstructured.NestedSlice(containers[0].(map[string]interface{}), "env")
	require.NoError(t, err)
	require.Len(t, env, 2)
}
