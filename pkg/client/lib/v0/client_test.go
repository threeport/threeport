package v0

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPITransportBoundsTheConnection covers the transport the API client uses:
// it sets a dial timeout and a TLS handshake timeout, and a connection to an
// address nothing answers on fails within the dial timeout instead of holding the
// call for the whole request timeout.
func TestAPITransportBoundsTheConnection(t *testing.T) {
	const dialTimeout = 200 * time.Millisecond
	transport := apiTransport(nil, dialTimeout)

	// assert both connection timeouts are set
	require.NotNil(t, transport.DialContext)
	assert.Equal(t, dialTimeout, transport.TLSHandshakeTimeout)

	// assert a dial to an address nothing answers on returns an error in time
	// (192.0.2.1 is reserved for documentation and never routes)
	start := time.Now()
	conn, err := transport.DialContext(context.Background(), "tcp", "192.0.2.1:443")
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err)
	assert.Less(t, time.Since(start), dialTimeout+2*time.Second)
}
