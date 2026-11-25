package brokerstorage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
)

// brokerServer is what a server's /varz reports.
type brokerServer struct {
	name           string
	used           uint64
	limit          int64
	noJetStream    bool
	status         int
	unreadableBody bool
}

// testMonitor returns a monitor of servers, each on its own local listener,
// and the recorder its events go to.
func testMonitor(t *testing.T, servers ...brokerServer) (*Monitor, *events.FakeRecorder) {
	t.Helper()
	var addresses []string
	byHost := map[string]brokerServer{}
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/varz", r.URL.Path)
		server := byHost[strings.Split(r.Host, ":")[0]]
		if server.status != 0 {
			// a server that isn't well reports nothing to trust, whatever
			// its body says
			w.WriteHeader(server.status)
			fmt.Fprintf(w, `{"server_name": %q, "jetstream": {"config": {"max_storage": 100}, "stats": {"storage": 99}}}`, server.name)
			return
		}
		if server.unreadableBody {
			fmt.Fprint(w, "{")
			return
		}
		if server.noJetStream {
			fmt.Fprintf(w, `{"server_name": %q, "jetstream": {}}`, server.name)
			return
		}
		fmt.Fprintf(w, `{"server_name": %q, "jetstream": {"config": {"max_storage": %d}, "stats": {"storage": %d}}}`,
			server.name, server.limit, server.used)
	}))
	t.Cleanup(listener.Close)
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(listener.URL, "http://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	// each server has an address of its own, which the client dials the
	// listener for, and the handler tells them apart by
	for i, server := range servers {
		address := fmt.Sprintf("127.0.0.%d", i+1)
		byHost[address] = server
		addresses = append(addresses, address)
	}

	recorder := events.NewFakeRecorder(10)
	kubeClient := fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "nats-js-0", Namespace: "threeport-control-plane", UID: "pod-uid"}})
	return &Monitor{
		Service:    "nats-js",
		Namespace:  "threeport-control-plane",
		Port:       port,
		Recorder:   recorder,
		KubeClient: kubeClient,
		Log:        logr.Discard(),
		HTTPClient: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, listener.Listener.Addr().String())
			},
		}},
		LookupHost: func(_ context.Context, host string) ([]string, error) {
			require.Equal(t, "nats-js.threeport-control-plane.svc", host)
			return addresses, nil
		},
	}, recorder
}

// recordedEvents drains the events recorded.
func recordedEvents(recorder *events.FakeRecorder) []string {
	var recorded []string
	for {
		select {
		case event := <-recorder.Events:
			recorded = append(recorded, event)
		default:
			return recorded
		}
	}
}

// TestAServerNearlyOutOfStorageIsWarnedAbout covers the check of each of the
// broker's servers: one that uses 80% or more of its JetStream storage gets
// a warning event on its pod, and one below, one without JetStream, or one
// that can't be read gets none.
func TestAServerNearlyOutOfStorageIsWarnedAbout(t *testing.T) {
	const gib = 1 << 30
	monitor, recorder := testMonitor(t,
		brokerServer{name: "nats-js-0", used: 160 * gib, limit: 187 * gib},
		brokerServer{name: "nats-js-1", used: 100 * gib, limit: 187 * gib},
		brokerServer{name: "nats-js-2", used: 0, limit: 0},
		brokerServer{name: "nats-js-3", noJetStream: true},
		brokerServer{name: "nats-js-4", status: http.StatusServiceUnavailable},
		brokerServer{name: "nats-js-5", unreadableBody: true},
		brokerServer{name: "nats-js-6", used: 80, limit: 100},
	)
	monitor.Check(context.Background())

	recorded := recordedEvents(recorder)
	require.Len(t, recorded, 2, "%v", recorded)
	assert.Equal(t,
		"Warning MessageBrokerStorageNearlyFull message broker server nats-js-0 uses 86% of its JetStream storage (160.0GiB of 187.0GiB); "+
			"it refuses messages once it is full, which stops the control plane's notifications: grow its volume before then",
		recorded[0])
	assert.Contains(t, recorded[1], "server nats-js-6 uses 80% of its JetStream storage (80B of 100B)", "at the threshold")
}

// TestTheWarningIsRecordedOnTheServersPod covers the object the event is
// recorded on: the server's pod, read so the event is listed with it, or one
// named for the server when it can't be read.
func TestTheWarningIsRecordedOnTheServersPod(t *testing.T) {
	monitor, _ := testMonitor(t, brokerServer{name: "nats-js-0", used: 9, limit: 10}, brokerServer{name: "nats-js-9", used: 9, limit: 10})
	var objects []*corev1.Pod
	monitor.Recorder = recordingObjects{events.NewFakeRecorder(10), &objects}
	monitor.Check(context.Background())

	require.Len(t, objects, 2)
	assert.Equal(t, "pod-uid", string(objects[0].UID), "the pod as it is")
	assert.Equal(t, "nats-js-9", objects[1].Name)
	assert.Equal(t, "threeport-control-plane", objects[1].Namespace)
}

// recordingObjects keeps the pods events are recorded on.
type recordingObjects struct {
	*events.FakeRecorder
	pods *[]*corev1.Pod
}

func (r recordingObjects) Eventf(regarding, _ runtime.Object, _, _, _, _ string, _ ...interface{}) {
	*r.pods = append(*r.pods, regarding.(*corev1.Pod))
}

// TestAThresholdCanBeSet covers a threshold other than the default.
func TestAThresholdCanBeSet(t *testing.T) {
	monitor, recorder := testMonitor(t, brokerServer{name: "nats-js-0", used: 50, limit: 100})
	monitor.Check(context.Background())
	assert.Empty(t, recordedEvents(recorder))

	monitor.Threshold = 0.5
	monitor.Check(context.Background())
	assert.Len(t, recordedEvents(recorder), 1)
}

// TestABrokerThatCantBeFoundIsCheckedAgainLater covers a lookup of the
// broker's servers failing: nothing is recorded, and the next check tries
// again.
func TestABrokerThatCantBeFoundIsCheckedAgainLater(t *testing.T) {
	monitor, recorder := testMonitor(t, brokerServer{name: "nats-js-0", used: 99, limit: 100})
	found := monitor.LookupHost
	monitor.LookupHost = func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") }
	monitor.Check(context.Background())
	assert.Empty(t, recordedEvents(recorder))

	monitor.LookupHost = found
	monitor.Check(context.Background())
	assert.Len(t, recordedEvents(recorder), 1)
}

// TestOnlyTheLeadingAgentChecksTheBroker covers the monitor running in the
// agent that holds the lease, so one warning is recorded, not one for each
// replica.
func TestOnlyTheLeadingAgentChecksTheBroker(t *testing.T) {
	assert.True(t, (&Monitor{}).NeedLeaderElection())
}

// TestStartChecksUntilStopped covers the monitor checking once on start and
// stopping with its context.
func TestStartChecksUntilStopped(t *testing.T) {
	monitor, recorder := testMonitor(t, brokerServer{name: "nats-js-0", used: 99, limit: 100})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitor.Interval = 10 * time.Millisecond
	lookups := 0
	found := monitor.LookupHost
	monitor.LookupHost = func(ctx context.Context, host string) ([]string, error) {
		lookups++
		if lookups == 2 {
			cancel()
		}
		return found(ctx, host)
	}
	require.NoError(t, monitor.Start(ctx))
	assert.Equal(t, 2, lookups, "checked on start, and again on the interval")
	assert.Len(t, recordedEvents(recorder), 1)
}

// TestByteCountsAreWrittenInBinaryUnits covers the sizes in the warning.
func TestByteCountsAreWrittenInBinaryUnits(t *testing.T) {
	for bytes, want := range map[uint64]string{0: "0B", 1023: "1023B", 1024: "1.0KiB", 5 << 30: "5.0GiB", 250 << 30: "250.0GiB", 1 << 40: "1.0TiB"} {
		assert.Equal(t, want, formatBytes(bytes), bytes)
	}
}
