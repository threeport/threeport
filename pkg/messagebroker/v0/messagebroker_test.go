package v0

import (
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// testBroker is a NATS server that requires TLS and a user and password, as
// the control plane's does.
type testBroker struct {
	server *server.Server
	caFile string
	port   string
}

// newTestCA returns a CA and the path of its certificate.
func newTestCA(t *testing.T) (*auth.AuthConfig, string) {
	t.Helper()

	ca, err := auth.GetAuthConfig()
	require.NoError(t, err)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(caFile, []byte(ca.CAPemEncoded), 0600))

	return ca, caFile
}

// startTestBroker starts a broker with a certificate signed by ca.
func startTestBroker(t *testing.T, ca *auth.AuthConfig, caFile string) *testBroker {
	t.Helper()

	cert, key, err := auth.GenerateCertificate(ca.CAConfig, &ca.CAPrivateKey, "nats-js", "threeport", "messagebroker")
	require.NoError(t, err)
	keyPair, err := tls.X509KeyPair([]byte(cert), []byte(key))
	require.NoError(t, err)

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		Username:  "threeport",
		Password:  "s3cret",
		TLS:       true,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{keyPair}, MinVersion: tls.VersionTLS12},
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := server.NewServer(opts)
	require.NoError(t, err)
	go srv.Start()
	require.True(t, srv.ReadyForConnections(10*time.Second), "the test broker did not start")
	t.Cleanup(srv.Shutdown)

	_, port, err := net.SplitHostPort(srv.Addr().String())
	require.NoError(t, err)

	return &testBroker{server: srv, caFile: caFile, port: port}
}

// config returns a config that reaches the broker with the given
// credentials and CA.
func (b *testBroker) config(user, password, caFile string) Config {
	return Config{Host: "127.0.0.1", Port: b.port, User: user, Password: password, CAFile: caFile}
}

// TestAComponentConnectsOverTLSWithItsCredentials covers the connection every
// component makes: over TLS, verifying the broker against its CA, with the
// user and password, which is the only way the broker lets it in.
func TestAComponentConnectsOverTLSWithItsCredentials(t *testing.T) {
	ca, caFile := newTestCA(t)
	broker := startTestBroker(t, ca, caFile)

	conn, err := Connect(broker.config("threeport", "s3cret", caFile))
	require.NoError(t, err)
	defer conn.Close()
	state, err := conn.TLSConnectionState()
	require.NoError(t, err, "the connection is not over TLS")
	assert.True(t, state.HandshakeComplete)
}

// TestTheBrokerTurnsAwayEveryOtherClient covers the issue's acceptance
// criteria: a client without the password, without TLS, or that does not
// trust the broker's CA cannot connect.
func TestTheBrokerTurnsAwayEveryOtherClient(t *testing.T) {
	ca, caFile := newTestCA(t)
	broker := startTestBroker(t, ca, caFile)
	_, otherCAFile := newTestCA(t)

	for name, config := range map[string]Config{
		"no credentials": broker.config("", "", caFile),
		"wrong password": broker.config("threeport", "guess", caFile),
		"no TLS":         broker.config("threeport", "s3cret", ""),
		"another CA":     broker.config("threeport", "s3cret", otherCAFile),
	} {
		conn, err := Connect(config, nats.NoReconnect(), nats.Timeout(2*time.Second))
		if err == nil {
			conn.Close()
		}
		assert.Error(t, err, "a client with %s connected", name)
	}
}

// TestTheURLCarriesNoCredentials covers what a component logs: the URL and
// address name the broker without its password.
func TestTheURLCarriesNoCredentials(t *testing.T) {
	config := Config{Host: "nats-js.threeport-control-plane.svc.cluster.local", Port: "4222", User: "threeport", Password: "s3cret", CAFile: "/etc/threeport/nats-ca/ca.crt"}

	assert.Equal(t, "tls://nats-js.threeport-control-plane.svc.cluster.local:4222", config.URL())
	assert.NotContains(t, config.URL()+config.Address(), "s3cret")
	assert.Equal(t, "nats://localhost:4222", Config{Host: "localhost", Port: "4222"}.URL(), "without a CA the connection is plain")
}

// TestAConfigWithoutAnAddressIsRefused covers a component missing its broker
// settings: it fails saying so rather than dialing an empty address.
func TestAConfigWithoutAnAddressIsRefused(t *testing.T) {
	_, err := Connect(Config{Port: "4222"})
	assert.ErrorIs(t, err, ErrNoAddress)
}
