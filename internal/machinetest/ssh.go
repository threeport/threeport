// Package machinetest provides shared test helpers for the machine-runtime
// and machine-workload reconciler tests: an in-process SSH server, a fake
// events recorder, and an httptest-backed threeport API stub.
package machinetest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// NewSigner returns a fresh RSA ssh.Signer suitable for use as a host or
// client key in tests.
func NewSigner(t *testing.T) ssh.Signer {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(rsaKey)
	require.NoError(t, err)
	return signer
}

// SSHOpts controls per-test SSH server behavior.
type SSHOpts struct {
	// ExitCode is the exit status returned to the client on exec/shell.
	ExitCode int

	// HoldSession, when non-zero, holds the session open without replying
	// past this duration. Used to drive timeout tests.
	HoldSession time.Duration

	// HoldHandshake, when non-zero, holds each accepted connection open
	// for this duration before any SSH protocol bytes are exchanged, so a
	// client's dial blocks as if the host were not responding. The hold
	// ends early when the server stops. Used to drive connect timeout
	// tests.
	HoldHandshake time.Duration

	// OpenConns, when non-nil, counts connections currently open on the
	// server: incremented when a connection is accepted, decremented after
	// it closes. Leak tests close their clients, wait for the count to
	// drain to zero, and fail if it never does.
	OpenConns *atomic.Int64
}

// HostKeyFromSigner returns the signer's public key encoded the same way
// captured host keys are stored on a machine runtime instance: base64 of
// the SSH wire-format public key bytes. Set the result as the instance's
// known host key to make the client verify the server instead of capturing
// its key.
func HostKeyFromSigner(signer ssh.Signer) string {
	return base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal())
}

// StartSSHServer starts an in-process SSH server bound to 127.0.0.1 on a
// random port. The returned addr is the bound "host:port" and stop must be
// called to release the listener and wait for goroutines.
func StartSSHServer(
	t *testing.T,
	hostKey ssh.Signer,
	user, password string,
	opts SSHOpts,
) (addr string, stop func()) {
	t.Helper()

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("bad credentials")
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var wg sync.WaitGroup
	stopped := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			if opts.OpenConns != nil {
				opts.OpenConns.Add(1)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveSSHConn(conn, config, opts, stopped)
				if opts.OpenConns != nil {
					opts.OpenConns.Add(-1)
				}
			}()
		}
	}()

	stop = func() {
		close(stopped)
		_ = listener.Close()
		wg.Wait()
	}
	return listener.Addr().String(), stop
}

// serveSSHConn handles one accepted TCP conn: complete the SSH handshake
// and process session channels per opts.
func serveSSHConn(nConn net.Conn, config *ssh.ServerConfig, opts SSHOpts, stopped <-chan struct{}) {
	defer nConn.Close()
	// hold before sending any protocol bytes so the client's dial blocks
	// until its own timeout fires; end the hold early on server stop
	if opts.HoldHandshake > 0 {
		select {
		case <-time.After(opts.HoldHandshake):
		case <-stopped:
			return
		}
	}
	_, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go handleSession(channel, requests, opts, stopped)
	}
}

// handleSession either replies with opts.ExitCode once the client finishes
// writing its script to stdin, or holds the session open for opts.HoldSession
// (used to drive timeout tests).
func handleSession(channel ssh.Channel, requests <-chan *ssh.Request, opts SSHOpts, stopped <-chan struct{}) {
	defer channel.Close()
	for req := range requests {
		switch req.Type {
		case "exec", "shell":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			if opts.HoldSession > 0 {
				select {
				case <-time.After(opts.HoldSession):
				case <-stopped:
				}
				return
			}
			// drain stdin synchronously to simulate the shell consuming
			// the piped script; only then send exit-status, matching
			// real SSH server behavior
			_, _ = io.Copy(io.Discard, channel)
			payload := []byte{0, 0, 0, byte(opts.ExitCode)}
			_, _ = channel.SendRequest("exit-status", false, payload)
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}
