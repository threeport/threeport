package v0

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-logr/logr"
)

const (
	// tcpDialTimeout is the timeout for each TCP connection attempt.
	tcpDialTimeout = 2 * time.Second

	// tcpRetryInterval is the sleep duration between TCP connection attempts.
	tcpRetryInterval = 2 * time.Second

	// tcpMaxRetries is the maximum number of TCP connection retries before
	// the process exits.
	tcpMaxRetries = 60

	// tcpAPIPort is the port used for the threeport API service.
	tcpAPIPort = 443
)

// WaitForAPI waits for the threeport API server to become reachable on port
// 443. It retries up to 60 times with a 2-second dial timeout and 2-second
// sleep between attempts (~4 minutes total). If the endpoint is not reachable
// after all retries, the process exits with code 1 to trigger a pod restart.
func WaitForAPI(host string, log logr.Logger) {
	addr := fmt.Sprintf("%s:%d", host, tcpAPIPort)
	log.Info("waiting for API server", "address", addr)

	for i := 0; i < tcpMaxRetries; i++ {
		conn, err := net.DialTimeout("tcp", addr, tcpDialTimeout)
		if err == nil {
			conn.Close()
			log.Info("API server is reachable", "address", addr)
			return
		}
		time.Sleep(tcpRetryInterval)
	}

	log.Error(
		fmt.Errorf("API server not reachable after %d retries", tcpMaxRetries),
		"failed to connect to API server",
		"address", addr,
	)
	os.Exit(1)
}
