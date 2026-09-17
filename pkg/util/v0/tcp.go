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
)

// tcpAPIPorts is the API service ports: 443 when TLS is on, 80 when it is not.
var tcpAPIPorts = []int{443, 80}

// WaitForAPI dials host until the API accepts a TCP connection on 443 or 80.
// It retries 60 times with a 2s dial timeout and 2s sleep, then exits with
// code 1 so the pod restarts.
func WaitForAPI(host string, log logr.Logger) {
	// build host:port list for the known API service ports
	addrs := make([]string, 0, len(tcpAPIPorts))
	for _, port := range tcpAPIPorts {
		addrs = append(addrs, fmt.Sprintf("%s:%d", host, port))
	}
	log.Info("waiting for API server", "addresses", addrs)

	// dial each address until one succeeds
	for i := 0; i < tcpMaxRetries; i++ {
		for _, addr := range addrs {
			conn, err := net.DialTimeout("tcp", addr, tcpDialTimeout)
			if err == nil {
				conn.Close()
				log.Info("API server is reachable", "address", addr)
				return
			}
		}
		time.Sleep(tcpRetryInterval)
	}

	// exit so the controller pod restarts
	log.Error(
		fmt.Errorf("API server not reachable after %d retries", tcpMaxRetries),
		"failed to connect to API server",
		"addresses", addrs,
	)
	os.Exit(1)
}
