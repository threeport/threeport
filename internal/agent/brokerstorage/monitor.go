// Package brokerstorage warns before the message broker's JetStream storage
// fills.  A full broker refuses every message published to it, so the
// control plane's notifications stop, and nothing else says so until then.
package brokerstorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/events"
)

const (
	// MonitorPort is the port of a NATS server's monitoring endpoint.
	MonitorPort = 8222

	// DefaultInterval is how often the broker's storage is checked.
	DefaultInterval = 5 * time.Minute

	// DefaultThreshold is the share of its JetStream storage a server uses
	// when it is warned about: early enough to grow its volume before it
	// fills.
	DefaultThreshold = 0.8

	// EventReason and EventAction are the reason and action of the warning
	// event recorded on a server's pod.
	EventReason = "MessageBrokerStorageNearlyFull"
	EventAction = "CheckStorage"

	// requestTimeout bounds each request to a server's monitoring endpoint.
	requestTimeout = 10 * time.Second
)

// Monitor checks each message broker server's JetStream storage on an
// interval, and records a warning event on the server's pod, and logs it,
// when the server uses more of it than the threshold.  NATS refuses messages
// once it uses all of it: the limit is its max_file, or, when that's unset,
// three quarters of its volume's free space when it started.  A control
// plane's broker sets max_file, so the limit is the same on every volume.
type Monitor struct {
	// Service is the broker's headless service, which resolves to the
	// address of each of its servers.
	Service string

	// Namespace is the broker's namespace, the one the event is recorded in.
	Namespace string

	// Port is the servers' monitoring port, MonitorPort when unset.
	Port int

	Interval  time.Duration
	Threshold float64

	Recorder   events.EventRecorder
	KubeClient kubernetes.Interface
	Log        logr.Logger

	// LookupHost and HTTPClient reach the servers.  A test stands in for
	// them.
	LookupHost func(ctx context.Context, host string) ([]string, error)
	HTTPClient *http.Client
}

// serverVarz is the part of a NATS server's /varz the monitor reads.
type serverVarz struct {
	ServerName string `json:"server_name"`
	JetStream  struct {
		Config *struct {
			MaxStorage int64 `json:"max_storage"`
		} `json:"config"`
		Stats *struct {
			Storage uint64 `json:"storage"`
		} `json:"stats"`
	} `json:"jetstream"`
}

// ServerStorage is how much of its JetStream storage a server uses.
type ServerStorage struct {
	Server string
	Used   uint64
	Limit  int64
}

// Share returns the share of its storage the server uses.
func (s ServerStorage) Share() float64 {
	if s.Limit <= 0 {
		return 0
	}

	return float64(s.Used) / float64(s.Limit)
}

// NeedLeaderElection has only the agent that leads check the broker, so one
// warning is recorded, not one for each replica of the agent.
func (m *Monitor) NeedLeaderElection() bool {
	return true
}

// Start checks the broker on the interval until ctx is done.
func (m *Monitor) Start(ctx context.Context) error {
	interval := m.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		m.Check(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Check reads the storage of each of the broker's servers, and warns about
// each that uses more of it than the threshold.  A server it can't read is
// logged and left for the next check.
func (m *Monitor) Check(ctx context.Context) {
	threshold := m.Threshold
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	host := fmt.Sprintf("%s.%s.svc", m.Service, m.Namespace)

	addresses, err := m.lookupHost(ctx, host)
	if err != nil {
		m.Log.Error(err, "failed to find the message broker's servers to check their storage", "service", host)
		return
	}
	for _, address := range addresses {
		storage, err := m.serverStorage(ctx, address)
		if err != nil {
			m.Log.Error(err, "failed to read a message broker server's storage", "address", address)
			continue
		}
		// a server without JetStream, or no limit, shares nothing
		if storage.Share() < threshold {
			continue
		}
		m.warn(ctx, storage)
	}
}

// warn logs a server nearly out of storage, and records a warning event on
// its pod.
func (m *Monitor) warn(ctx context.Context, storage ServerStorage) {
	message := fmt.Sprintf(
		"message broker server %s uses %.0f%% of its JetStream storage (%s of %s); it refuses messages once it is full, "+
			"which stops the control plane's notifications: find the stream holding the backlog with `nats stream report` before then",
		storage.Server, storage.Share()*100, formatBytes(storage.Used), formatBytes(uint64(storage.Limit)),
	)
	m.Log.Info(message, "server", storage.Server, "used", storage.Used, "limit", storage.Limit)
	if m.Recorder == nil {
		return
	}

	// the server is named for its pod; a pod that can't be read is
	// referred to by name
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: storage.Server, Namespace: m.Namespace}}
	if m.KubeClient != nil {
		if found, err := m.KubeClient.CoreV1().Pods(m.Namespace).Get(ctx, storage.Server, metav1.GetOptions{}); err == nil {
			pod = found
		}
	}
	m.Recorder.Eventf(pod, nil, corev1.EventTypeWarning, EventReason, EventAction, "%s", message)
}

// serverStorage reads a server's JetStream storage from its monitoring
// endpoint.
func (m *Monitor) serverStorage(ctx context.Context, address string) (ServerStorage, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	port := m.Port
	if port == 0 {
		port = MonitorPort
	}
	url := fmt.Sprintf("http://%s/varz", net.JoinHostPort(address, strconv.Itoa(port)))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ServerStorage{}, err
	}
	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return ServerStorage{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ServerStorage{}, fmt.Errorf("%s returned %s", url, response.Status)
	}

	var varz serverVarz
	if err := json.NewDecoder(response.Body).Decode(&varz); err != nil {
		return ServerStorage{}, fmt.Errorf("failed to read %s: %w", url, err)
	}
	storage := ServerStorage{Server: varz.ServerName}
	if varz.JetStream.Config != nil {
		storage.Limit = varz.JetStream.Config.MaxStorage
	}
	if varz.JetStream.Stats != nil {
		storage.Used = varz.JetStream.Stats.Storage
	}

	return storage, nil
}

// lookupHost resolves the broker's service to its servers' addresses.
func (m *Monitor) lookupHost(ctx context.Context, host string) ([]string, error) {
	if m.LookupHost != nil {
		return m.LookupHost(ctx, host)
	}

	return net.DefaultResolver.LookupHost(ctx, host)
}

// formatBytes writes a byte count in the largest binary unit it fills.
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	value, exponent := float64(bytes)/unit, 0
	for value >= unit && exponent < 4 {
		value /= unit
		exponent++
	}

	return fmt.Sprintf("%.1f%ciB", value, "KMGTP"[exponent])
}
