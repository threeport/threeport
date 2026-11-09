package v0

import (
	"fmt"
	"net"
	"os"
	"strconv"
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

// WaitForAPI dials host until the API accepts a TCP connection.  A bare host is
// dialed on 443 and 80; an address that names its port, such as the external
// endpoint of a control plane on another cluster, is dialed on that port only.
// It retries 60 times with a 2s dial timeout and 2s sleep, then exits with
// code 1 so the pod restarts.
func WaitForAPI(host string, log logr.Logger) {
	host, ports := splitAPIAddress(host)

	addrs := make([]string, 0, len(ports))
	for _, port := range ports {
		addrs = append(addrs, net.JoinHostPort(host, strconv.Itoa(port)))
	}
	log.Info("waiting for API server", "addresses", addrs)

	addr, err := waitForAPI(host, ports, tcpDialTimeout, tcpRetryInterval, tcpMaxRetries)
	if err != nil {
		log.Error(err, "failed to connect to API server", "addresses", addrs)
		os.Exit(1)
	}

	log.Info("API server is reachable", "address", addr)
}

// splitAPIAddress separates an API address into a host and the ports to dial.
// The address names a port only when it is host:port; a bare host, including a
// bare IPv6 literal, gets the API service ports.
func splitAPIAddress(address string) (string, []int) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return address, tcpAPIPorts
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return address, tcpAPIPorts
	}

	return host, []int{port}
}

// waitForAPI dials each host:port until one accepts a TCP connection.
func waitForAPI(
	host string,
	ports []int,
	dialTimeout time.Duration,
	retryInterval time.Duration,
	maxRetries int,
) (string, error) {
	addrs := make([]string, 0, len(ports))
	for _, port := range ports {
		addrs = append(addrs, net.JoinHostPort(host, strconv.Itoa(port)))
	}

	for i := 0; i < maxRetries; i++ {
		for _, addr := range addrs {
			conn, err := net.DialTimeout("tcp", addr, dialTimeout)
			if err == nil {
				conn.Close()
				return addr, nil
			}
		}
		time.Sleep(retryInterval)
	}

	return "", fmt.Errorf("API server not reachable after %d retries: %v", maxRetries, addrs)
}
