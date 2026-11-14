package v0

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.True(t, namedObject(volumes, brokerCAVolume))
	containers, _, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	mounts, _, err := unstructured.NestedSlice(containers[0].(map[string]interface{}), "volumeMounts")
	require.NoError(t, err)
	require.True(t, mountNamed(mounts, brokerCAMount))
	require.True(t, mountNamed(mounts, brokerCASystemFile))
}
