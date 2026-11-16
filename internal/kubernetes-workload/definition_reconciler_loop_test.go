package kubernetesworkload

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/internal/kubernetes-workload/notif"
	"github.com/threeport/threeport/internal/machinetest"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	event "github.com/threeport/threeport/pkg/event/v0"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// runDefinitionReconciler starts the generated KubernetesWorkloadDefinitionReconciler
// against an in-process JetStream server and the fixture's API, and returns a
// function that publishes an update notification for the definition and
// waits for the loop to finish it.
func runDefinitionReconciler(t *testing.T, fixture *definitionUpdateFixture) (publishUpdate func()) {
	t.Helper()

	// Port -1 is nats-server RANDOM_PORT; NoSigs leaves process signals alone
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)
	go server.Start()
	require.True(t, server.ReadyForConnections(10*time.Second), "nats server did not start")
	// shut down last: the loop exits the process on a missing stream
	t.Cleanup(server.Shutdown)

	conn, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	js, err := conn.JetStream()
	require.NoError(t, err)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     notif.KubernetesWorkloadStreamName,
		Subjects: []string{notif.KubernetesWorkloadDefinitionSubject},
	})
	require.NoError(t, err)
	keyValue, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "lock"})
	require.NoError(t, err)

	// the name and subject the controller registers the reconciler under
	const name = "KubernetesWorkloadDefinitionReconciler"
	sub, err := js.PullSubscribe(
		notif.KubernetesWorkloadDefinitionSubject,
		name+"Consumer",
		nats.BindStream(notif.KubernetesWorkloadStreamName),
	)
	require.NoError(t, err)

	recorder := machinetest.NewFakeRecorder()
	ready := &atomic.Bool{}
	ready.Store(true)
	log := logr.Discard()
	shutdown := make(chan bool, 1)
	var shutdownWait sync.WaitGroup
	go KubernetesWorkloadDefinitionReconciler(&controller.Reconciler{
		Name:             name,
		APIServer:        fixture.reconciler.APIServer,
		APIClient:        &http.Client{},
		JetStreamContext: js,
		Sub:              sub,
		KeyValue:         keyValue,
		ControllerID:     uuid.New(),
		Ready:            ready,
		Log:              &log,
		Shutdown:         shutdown,
		ShutdownWait:     &shutdownWait,
		EventsRecorder:   recorder,
	})
	t.Cleanup(func() {
		shutdown <- true
		shutdownWait.Wait()
	})
	// close first so a parked fetch returns
	t.Cleanup(conn.Close)

	succeeded := event.GetSuccessReasonForOperation(notifications.NotificationOperationUpdated)
	return func() {
		t.Helper()
		fixture.api.mu.Lock()
		payload, err := fixture.api.definition.NotificationPayload(
			notifications.NotificationOperationUpdated, false, time.Now().Unix(),
		)
		fixture.api.mu.Unlock()
		require.NoError(t, err)
		_, err = js.Publish(notif.KubernetesWorkloadDefinitionUpdateSubject, *payload)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			for _, reason := range recorder.GetReasons() {
				if reason == succeeded {
					return true
				}
			}
			return false
		}, 20*time.Second, 25*time.Millisecond, "the update was never reconciled; events: %v", recorder.GetReasons())
	}
}

// TestTheReconcilerLoopLeavesAnUpdatedDefinitionReconciled runs a definition
// update through the generated reconciler loop rather than calling the handler
// alone, because the gap is between the two.
//
// The handler marks the definition unreconciled in the API while it works, so
// instances created meanwhile wait for it. The loop marks it reconciled again
// afterwards only when the object it holds says it is not - and that object is
// the one it read before calling the handler. Told nothing, the loop saw a
// definition still reconciled, wrote nothing, and left it unreconciled in the
// API for good, so every instance created from it later was refused.
func TestTheReconcilerLoopLeavesAnUpdatedDefinitionReconciled(t *testing.T) {
	fixture := newDefinitionUpdateFixture(t, yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.26"), webService))
	fixture.api.mu.Lock()
	fixture.api.definition = &v0.KubernetesWorkloadDefinition{
		Common:         v0.Common{ID: fixture.definition.ID},
		Definition:     v0.Definition{Name: fixture.definition.Name},
		Reconciliation: v0.Reconciliation{Reconciled: util.Ptr(true)},
		YAMLDocument:   yamlDocument(fmt.Sprintf(webDeployment, 1, "nginx:1.27"), webService),
	}
	fixture.api.mu.Unlock()

	runDefinitionReconciler(t, fixture)()

	fixture.api.mu.Lock()
	defer fixture.api.mu.Unlock()
	assert.Equal(t, []bool{false, true}, fixture.api.definitionReconciled,
		"held back while the update ran, and released once it finished")
	require.NotNil(t, fixture.api.definition.Reconciled)
	assert.True(t, *fixture.api.definition.Reconciled, "an instance created from the definition now would be refused")

	// the update did reach the instances, so this is the path that marks it
	var instanceWrites int
	for _, write := range fixture.api.writes {
		if strings.HasPrefix(write, http.MethodPatch+" "+v0.PathKubernetesWorkloadInstances+"/") {
			instanceWrites++
		}
	}
	assert.NotZero(t, instanceWrites)
}
