package v0

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	restapiutil "github.com/threeport/threeport/cmd/rest-api/util"
	secretnotif "github.com/threeport/threeport/internal/secret/notif"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	messagebroker "github.com/threeport/threeport/pkg/messagebroker/v0"
)

// freePort returns a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

// TestAProductionBrokerIsAClusterThatSurvivesTheLossOfAServer runs the
// production broker as the installer writes it: three servers, each started
// from the config map and the certificate and credentials secrets the way
// its pod starts, with only the addresses changed to this machine's.  The
// API server's streams and a controller's lock bucket are created with the
// replicas the installer tells them, and keep their messages and keep
// taking writes when the server leading them is lost.
func TestAProductionBrokerIsAClusterThatSurvivesTheLossOfAServer(t *testing.T) {
	kubeClient, _ := installTestBroker(t, ControlPlaneTierProd)
	config := brokerConfig(t, kubeClient)

	certs := t.TempDir()
	for _, key := range []string{"tls.crt", "tls.key", "ca.crt"} {
		require.NoError(t, os.WriteFile(filepath.Join(certs, key), []byte(testSecretValue(t, kubeClient, natsServerCertSecretName, key)), 0600))
	}
	configFile := filepath.Join(t.TempDir(), "nats.conf")
	require.NoError(t, os.WriteFile(configFile, []byte(strings.ReplaceAll(config, natsServerCertsPath, certs)), 0600))
	password := testSecretValue(t, kubeClient, natsCredentialsSecretName, "password")
	t.Setenv("NATS_USER", NatsUser)
	t.Setenv("NATS_PASSWORD", password)

	// each pod's route port, on this machine
	routePorts := []int{freePort(t), freePort(t), freePort(t)}
	servers := make([]*server.Server, len(routePorts))
	for i := range servers {
		t.Setenv("POD_NAME", fmt.Sprintf("nats-js-%d", i))
		opts, err := server.ProcessConfigFile(configFile)
		require.NoError(t, err, "the server cannot read its config")

		// the cluster the config describes
		assert.Equal(t, natsClusterName, opts.Cluster.Name)
		assert.Equal(t, natsClusterPort, opts.Cluster.Port)
		assert.True(t, opts.Cluster.NoAdvertise, "clients must keep using the service name their certificate covers")
		require.NotNil(t, opts.Cluster.TLSConfig, "the routes must use TLS")
		assert.Equal(t, tls.RequireAndVerifyClientCert, opts.Cluster.TLSConfig.ClientAuth, "a server must present a certificate from the broker's CA to join")
		assert.Equal(t, fmt.Sprintf("nats-js-%d", i), opts.ServerName)
		require.Len(t, opts.Routes, len(routePorts), "every server routes to every pod")

		// the same cluster, on this machine: a route to the pod named
		// nats-js-N goes to the Nth port.  The certificate covers localhost,
		// so the routes are verified as they are in the cluster.
		routes := make([]*url.URL, 0, len(opts.Routes))
		for _, route := range opts.Routes {
			pod, _, _ := strings.Cut(route.Hostname(), ".")
			index, err := strconv.Atoi(strings.TrimPrefix(pod, "nats-js-"))
			require.NoError(t, err, "route %s does not name a pod", route)
			assert.Equal(t, fmt.Sprintf("nats-js-%d.nats-js.%s.svc.cluster.local", index, testControlPlaneNamespace), route.Hostname())
			routes = append(routes, &url.URL{Scheme: route.Scheme, Host: fmt.Sprintf("localhost:%d", routePorts[index])})
		}
		opts.Routes = routes
		opts.Cluster.Host, opts.Cluster.Port = "127.0.0.1", routePorts[i]
		opts.Host, opts.Port, opts.HTTPPort = "127.0.0.1", -1, 0
		opts.StoreDir, opts.PidFile = t.TempDir(), ""
		opts.NoLog, opts.NoSigs = true, true

		srv, err := server.NewServer(opts)
		require.NoError(t, err)
		go srv.Start()
		t.Cleanup(srv.Shutdown)
		servers[i] = srv
	}
	for _, srv := range servers {
		require.True(t, srv.ReadyForConnections(10*time.Second))
	}

	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, []byte(testSecretValue(t, kubeClient, NatsCACertSecretName, "ca.crt")), 0600))
	connect := func(srv *server.Server) *nats.Conn {
		t.Helper()
		_, port, err := net.SplitHostPort(srv.Addr().String())
		require.NoError(t, err)
		conn, err := messagebroker.Connect(messagebroker.Config{Host: "localhost", Port: port, User: NatsUser, Password: password, CAFile: caFile})
		require.NoError(t, err)
		t.Cleanup(conn.Close)
		return conn
	}

	// the API server creates its streams once the servers have formed a
	// JetStream cluster, replicated as the installer tells it
	replicas, err := messagebroker.ParseReplicas(fmt.Sprint(brokerReplicas(t, kubeClient)))
	require.NoError(t, err)
	conn := connect(servers[0])
	var js *nats.JetStreamContext
	require.Eventually(t, func() bool {
		js, err = restapiutil.InitJetStream(conn, replicas)
		return err == nil
	}, 30*time.Second, 250*time.Millisecond, "the servers must form a JetStream cluster: %v", err)

	info, err := (*js).StreamInfo(secretnotif.SecretStreamName)
	require.NoError(t, err)
	assert.Equal(t, 3, info.Config.Replicas)
	require.NotNil(t, info.Cluster)
	assert.Equal(t, natsClusterName, info.Cluster.Name)
	assert.Len(t, info.Cluster.Replicas, 2, "the stream must have a copy on each of the other two servers")

	// a controller's lock bucket, as the controllers create it
	kv, err := controller.CreateLockBucketIfNotExists(*js, &nats.KeyValueConfig{
		Bucket:   "cluster-test-lock",
		TTL:      time.Minute,
		Replicas: replicas,
	})
	require.NoError(t, err)
	status, err := kv.Status()
	require.NoError(t, err)
	assert.Equal(t, 3, status.(*nats.KeyValueBucketStatus).StreamInfo().Config.Replicas)

	subject := secretnotif.GetSecretSubjects()[0]
	_, err = (*js).Publish(subject, []byte("before"))
	require.NoError(t, err)
	_, err = kv.Put("lock", []byte("held"))
	require.NoError(t, err)

	// lose the server leading the stream
	lostLeader := info.Cluster.Leader
	var survivor *server.Server
	for _, srv := range servers {
		if srv.Name() == lostLeader {
			srv.Shutdown()
			srv.WaitForShutdown()
		} else {
			survivor = srv
		}
	}
	require.NotNil(t, survivor)

	js2, err := connect(survivor).JetStream()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, err = js2.StreamInfo(secretnotif.SecretStreamName)
		return err == nil && info.Cluster != nil && info.Cluster.Leader != "" && info.Cluster.Leader != lostLeader
	}, 30*time.Second, 250*time.Millisecond, "the stream must elect a new leader: %v", err)
	assert.Equal(t, uint64(1), info.State.Msgs, "the stream must keep its message")

	_, err = js2.Publish(subject, []byte("after"))
	assert.NoError(t, err, "the stream must keep taking messages")

	kv2, err := js2.KeyValue("cluster-test-lock")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		entry, err := kv2.Get("lock")
		return err == nil && string(entry.Value()) == "held"
	}, 30*time.Second, 250*time.Millisecond, "the lock bucket must keep its entry")
	_, err = kv2.Put("lock", []byte("taken over"))
	assert.NoError(t, err, "the lock bucket must keep taking writes")
}
