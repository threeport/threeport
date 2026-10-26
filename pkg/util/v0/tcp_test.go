package v0

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitForAPIUsesSecondPortWhenFirstIsClosed covers dual-port dial.
func TestWaitForAPIUsesSecondPortWhenFirstIsClosed(t *testing.T) {
	require := require.New(t)

	// listen on an ephemeral port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(err)
	defer ln.Close()

	_, livePortStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(err)
	livePort, err := strconv.Atoi(livePortStr)
	require.NoError(err)

	addr, err := waitForAPI(
		"127.0.0.1",
		[]int{1, livePort},
		50*time.Millisecond,
		10*time.Millisecond,
		5,
	)
	require.NoError(err)
	require.Equal(fmt.Sprintf("127.0.0.1:%d", livePort), addr)
}

// TestWaitForAPIExhaustsRetriesWhenNoPortIsOpen covers the retry budget.
func TestWaitForAPIExhaustsRetriesWhenNoPortIsOpen(t *testing.T) {
	require := require.New(t)

	_, err := waitForAPI(
		"127.0.0.1",
		[]int{1},
		20*time.Millisecond,
		5*time.Millisecond,
		2,
	)
	require.Error(err)
}

func TestSplitAPIAddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantHost  string
		wantPorts []int
	}{
		{"bare service name", "threeport-api-server", "threeport-api-server", tcpAPIPorts},
		{"hostname with port", "api.example.com:443", "api.example.com", []int{443}},
		{"ip with port", "34.1.2.3:8443", "34.1.2.3", []int{8443}},
		{"bare ip", "34.1.2.3", "34.1.2.3", tcpAPIPorts},
		{"bare ipv6", "2001:db8::1", "2001:db8::1", tcpAPIPorts},
		{"bracketed ipv6 with port", "[2001:db8::1]:443", "2001:db8::1", []int{443}},
		{"port out of range", "api.example.com:99999", "api.example.com:99999", tcpAPIPorts},
		{"port not a number", "api.example.com:https", "api.example.com:https", tcpAPIPorts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, ports := splitAPIAddress(tt.address)
			require.Equal(t, tt.wantHost, host)
			require.Equal(t, tt.wantPorts, ports)
		})
	}
}
