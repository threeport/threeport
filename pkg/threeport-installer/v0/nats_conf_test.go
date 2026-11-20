package v0

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	auth "github.com/threeport/threeport/pkg/auth/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// staleChecksum is the value the stateful set used to carry, hard-coded.
const staleChecksum = "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7"

func natsInstaller(update bool) *ControlPlaneInstaller {
	cpi := backupInstaller()
	cpi.Opts.CreateOrUpdateKubeResources = update

	return cpi
}

// TestNatsConfSetsDiskCeiling checks the message broker names its disk ceiling
// and that the ceiling and the stream caps fit in the volume.
func TestNatsConfSetsDiskCeiling(t *testing.T) {
	conf := natsConf(ControlPlaneNamespace, 1)

	if !strings.Contains(conf, "max_file: 4Gi") {
		t.Errorf("nats.conf does not set max_file to 4Gi:\n%s", conf)
	}
	if notifications.MessageBrokerMaxFileGiB >= notifications.MessageBrokerMinVolumeGiB {
		t.Errorf(
			"max_file %dGi is not below the %dGi smallest volume",
			notifications.MessageBrokerMaxFileGiB, notifications.MessageBrokerMinVolumeGiB,
		)
	}
	// the development tier gets the smallest volume the broker accepts
	dev := resource.MustParse(devMessageBrokerStorageSize)
	if dev.Cmp(*resource.NewQuantity(int64(notifications.MessageBrokerMinVolumeGiB)<<30, resource.BinarySI)) < 0 {
		t.Errorf("the development volume %s is smaller than the message broker's minimum", devMessageBrokerStorageSize)
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
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(natsConf(ControlPlaneNamespace, 1))))
	if got := natsConfChecksum(ControlPlaneNamespace, 1); got != want {
		t.Errorf("checksum %s, want %s", got, want)
	}
	if got := natsConfChecksum(ControlPlaneNamespace, 1); got == "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7" {
		t.Error("checksum is still the hard-coded value")
	}
}

// TestInstallUpdatesAnExistingNatsStatefulSet runs the whole dependency install
// a second time over a stateful set from an older install, as an upgrade does.
func TestInstallUpdatesAnExistingNatsStatefulSet(t *testing.T) {
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
	// a reinstall keeps the servers the broker already runs
	replicas, _, _ := unstructured.NestedInt64(got.Object, "spec", "replicas")
	require.NotZero(t, replicas)
	annotations, _, _ := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	assert.Equal(t, natsConfChecksum(cpi.Opts.Namespace, replicas), annotations["checksum/config"])
	assert.Equal(t, "true", annotations["prometheus.io/scrape"])
	// the update replaces the pod template and nothing else
	claims, _, _ := unstructured.NestedSlice(got.Object, "spec", "volumeClaimTemplates")
	assert.Len(t, claims, 1)
}
