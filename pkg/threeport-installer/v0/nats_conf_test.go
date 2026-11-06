package v0

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	auth "github.com/threeport/threeport/pkg/auth/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// staleChecksum is the value the stateful set used to carry, hard-coded.
const staleChecksum = "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7"

// natsStatefulSetWith builds an existing message broker stateful set with a
// config checksum, a volume claim template and a replica count.
func natsStatefulSetWith(checksum string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]interface{}{
			"name":      natsServiceName,
			"namespace": ControlPlaneNamespace,
		},
		"spec": map[string]interface{}{
			"replicas": int64(1),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						"checksum/config":      checksum,
						"prometheus.io/scrape": "true",
					},
				},
			},
			"volumeClaimTemplates": []interface{}{
				map[string]interface{}{
					"metadata": map[string]interface{}{"name": "datadir"},
				},
			},
		},
	}}
}

// natsPatchClient returns a fake cluster holding the given objects and a
// counter of the patches sent to it.
func natsPatchClient(objects ...runtime.Object) (*dynamicfake.FakeDynamicClient, *int) {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{statefulSetGVR: "StatefulSetList"},
		objects...,
	)
	patches := 0
	client.PrependReactor("patch", "statefulsets", func(clienttesting.Action) (bool, runtime.Object, error) {
		patches++
		return false, nil, nil
	})

	return client, &patches
}

func natsInstaller(update bool) *ControlPlaneInstaller {
	cpi := backupInstaller()
	cpi.Opts.CreateOrUpdateKubeResources = update

	return cpi
}

// TestNatsConfSetsDiskCeiling checks the message broker names its disk ceiling
// and that the ceiling and the stream caps fit in the volume.
func TestNatsConfSetsDiskCeiling(t *testing.T) {
	conf := natsConf()

	if !strings.Contains(conf, "max_file: 4Gi") {
		t.Errorf("nats.conf does not set max_file to 4Gi:\n%s", conf)
	}
	if notifications.MessageBrokerMaxFileGiB >= notifications.MessageBrokerVolumeGiB {
		t.Errorf(
			"max_file %dGi is not below the %dGi volume",
			notifications.MessageBrokerMaxFileGiB, notifications.MessageBrokerVolumeGiB,
		)
	}

	streams := int64(notifications.ControllerStreamCount) * notifications.ControllerStreamMaxBytes
	if ceiling := int64(notifications.MessageBrokerMaxFileGiB) << 30; streams > ceiling {
		t.Errorf("stream caps total %d bytes, over the %d byte max_file", streams, ceiling)
	}
}

// TestNatsConfChecksumTracksConfig checks the pod template annotation moves
// with the configuration, because the server ignores a reloaded JetStream
// limit and has to be restarted.
func TestNatsConfChecksumTracksConfig(t *testing.T) {
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(natsConf())))
	if got := natsConfChecksum(); got != want {
		t.Errorf("checksum %s, want %s", got, want)
	}
	if got := natsConfChecksum(); got == "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7" {
		t.Error("checksum is still the hard-coded value")
	}
}

// TestExistingNatsStatefulSetGetsTheNewChecksum covers an upgrade.  The
// stateful set is persistent, so the install does not rewrite it, and the new
// checksum has to be patched in for the broker to restart.  The volume claim
// templates and the other pod template annotations must survive.
func TestExistingNatsStatefulSetGetsTheNewChecksum(t *testing.T) {
	client, patches := natsPatchClient(natsStatefulSetWith(staleChecksum))

	require.NoError(t, natsInstaller(true).refreshNatsConfigChecksum(client))

	assert.Equal(t, 1, *patches)
	got, err := client.Resource(statefulSetGVR).Namespace(ControlPlaneNamespace).Get(
		context.Background(), natsServiceName, metav1.GetOptions{},
	)
	require.NoError(t, err)
	annotations, _, _ := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	assert.Equal(t, natsConfChecksum(), annotations["checksum/config"])
	assert.Equal(t, "true", annotations["prometheus.io/scrape"])
	claims, _, _ := unstructured.NestedSlice(got.Object, "spec", "volumeClaimTemplates")
	assert.Len(t, claims, 1)
	replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	assert.Equal(t, int64(1), replicas)
}

// TestCurrentNatsStatefulSetIsNotPatched checks a repeated install does not
// write, which would otherwise bump the resource version on every run.
func TestCurrentNatsStatefulSetIsNotPatched(t *testing.T) {
	client, patches := natsPatchClient(natsStatefulSetWith(natsConfChecksum()))

	require.NoError(t, natsInstaller(true).refreshNatsConfigChecksum(client))

	assert.Equal(t, 0, *patches)
}

// TestMissingNatsStatefulSetIsNotCreatedByThePatch checks the patch has no
// effect before the install has created the stateful set.
func TestMissingNatsStatefulSetIsNotCreatedByThePatch(t *testing.T) {
	client, patches := natsPatchClient()

	require.NoError(t, natsInstaller(true).refreshNatsConfigChecksum(client))

	assert.Equal(t, 0, *patches)
}

// TestInstallPatchesAnExistingNatsStatefulSet runs the whole dependency install
// a second time over a stateful set from an older install, as an upgrade does.
func TestInstallPatchesAnExistingNatsStatefulSet(t *testing.T) {
	cpi := natsInstaller(true)
	client := installDependencies(t, cpi)

	// the stateful set from an older install
	current := getObject(t, client, statefulSetGVR, natsServiceName)
	require.NoError(t, unstructured.SetNestedField(
		current.Object, staleChecksum, "spec", "template", "metadata", "annotations", "checksum/config",
	))
	_, err := client.Resource(statefulSetGVR).Namespace(ControlPlaneNamespace).Update(
		context.Background(), normalized(current), metav1.UpdateOptions{},
	)
	require.NoError(t, err)

	mapper := backupTestMapper()
	dbCreds, err := auth.GenerateDbCreds(cpi.Opts.Namespace)
	require.NoError(t, err)
	// a production install bootstraps the database with a job, which no fake
	// cluster runs
	succeedDropJob(client)
	require.NoError(t, cpi.InstallThreeportControlPlaneDependencies(normalizingClient{client}, &mapper, "", dbCreds))

	got := getObject(t, client, statefulSetGVR, natsServiceName)
	checksum, _, _ := unstructured.NestedString(got.Object, "spec", "template", "metadata", "annotations", "checksum/config")
	assert.Equal(t, natsConfChecksum(), checksum)
}
