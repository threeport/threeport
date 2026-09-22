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
