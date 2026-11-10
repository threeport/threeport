// Package v0 connects Threeport's components to its message broker.
//
// The broker is a NATS server that requires TLS and a user and password on
// every client connection.  A component is given the broker's host and port,
// the credentials, and the certificate of the CA that signed the broker's
// certificate, and connects with all of them.
package v0

import (
	"errors"
	"fmt"
	"net"

	"github.com/nats-io/nats.go"
)

// Config is how a component reaches the message broker.
type Config struct {
	// Host and Port are the broker's address.
	Host string
	Port string

	// User and Password authenticate the component to the broker.
	User     string
	Password string

	// CAFile is the path of the certificate of the CA that signed the
	// broker's certificate.  When set, the connection uses TLS and verifies
	// the broker against it; when empty, it is plain, which only a broker
	// without TLS, such as one in a test, accepts.
	CAFile string
}

// ErrNoAddress is returned for a config without the broker's host or port.
var ErrNoAddress = errors.New("no message broker host and port")

// Address returns the broker's host and port, without credentials, for
// logging.
func (c Config) Address() string {
	return net.JoinHostPort(c.Host, c.Port)
}

// URL returns the URL a component connects to, without credentials, which
// are passed apart from it so they never end up in a log line or an error.
func (c Config) URL() string {
	scheme := "nats"
	if c.CAFile != "" {
		scheme = "tls"
	}

	return fmt.Sprintf("%s://%s", scheme, c.Address())
}

// Options returns the connection options the config calls for.
func (c Config) Options() []nats.Option {
	var options []nats.Option
	if c.User != "" || c.Password != "" {
		options = append(options, nats.UserInfo(c.User, c.Password))
	}
	if c.CAFile != "" {
		options = append(options, nats.RootCAs(c.CAFile))
	}

	return options
}

// Connect connects to the message broker, with any further options given.
func Connect(c Config, options ...nats.Option) (*nats.Conn, error) {
	if c.Host == "" || c.Port == "" {
		return nil, ErrNoAddress
	}

	conn, err := nats.Connect(c.URL(), append(c.Options(), options...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the message broker at %s: %w", c.URL(), err)
	}

	return conn, nil
}
