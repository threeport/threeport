// Package v0 provides client helpers for connecting to and executing scripts
// on threeport machine runtime instances.
package v0

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

const (
	// defaultSSHPort is the default TCP port used for SSH connections when
	// MachineRuntimeInstance.Port is not set.
	defaultSSHPort = 22

	// connectTimeout is the SSH dial timeout.
	connectTimeout = 10 * time.Second
)

// GetClient establishes an authenticated SSH connection to the given machine
// runtime instance.  SSHKey and SSHPassword are decrypted using the provided
// encryption key and used to build auth methods (key preferred, password
// fallback).  Port defaults to 22 if not set on the instance.
//
// If HostKey is set on the instance, the server's host key is verified against
// it.  If HostKey is nil, the server's key is accepted and returned as the
// second value so the caller can persist it for future verification.
func GetClient(mri *v0.MachineRuntimeInstance, encryptionKey string) (*ssh.Client, string, error) {
	// decrypt ssh credentials (at least one is guaranteed by BeforeCreate hook)
	var decryptedKey, decryptedPassword string
	if mri.SSHKey != nil {
		dk, err := encryption.Decrypt(encryptionKey, *mri.SSHKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decrypt ssh key: %w", err)
		}
		decryptedKey = dk
	}
	if mri.SSHPassword != nil {
		dp, err := encryption.Decrypt(encryptionKey, *mri.SSHPassword)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decrypt ssh password: %w", err)
		}
		decryptedPassword = dp
	}

	// build auth methods
	authMethods, err := buildAuthMethods(decryptedKey, decryptedPassword)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build ssh auth methods: %w", err)
	}

	// resolve port
	port := defaultSSHPort
	if mri.Port != nil {
		port = *mri.Port
	}

	// build host key callback
	var capturedHostKey string
	hostKeyCallback, err := buildHostKeyCallback(mri.HostKey, &capturedHostKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to build host key callback: %w", err)
	}

	// build client config
	config := &ssh.ClientConfig{
		User:            *mri.SSHUser,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         connectTimeout,
	}

	// dial the remote host
	addr := net.JoinHostPort(*mri.Hostname, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, "", fmt.Errorf("failed to dial ssh %s: %w", addr, err)
	}

	return client, capturedHostKey, nil
}

// buildHostKeyCallback returns an ssh.HostKeyCallback based on whether a known
// host key is provided.  If knownHostKey is non-nil, the callback verifies the
// server's key matches.  If nil, the callback accepts any key and writes the
// base64-encoded public key to capturedKey for the caller to persist.
func buildHostKeyCallback(knownHostKey *string, capturedKey *string) (ssh.HostKeyCallback, error) {
	if knownHostKey == nil {
		// capture mode: accept any key, record it
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			*capturedKey = base64.StdEncoding.EncodeToString(key.Marshal())
			return nil
		}, nil
	}

	// verification mode: parse the known key and compare
	knownKeyBytes, err := base64.StdEncoding.DecodeString(*knownHostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode stored host key: %w", err)
	}
	knownPubKey, err := ssh.ParsePublicKey(knownKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stored host key: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if !bytes.Equal(key.Marshal(), knownPubKey.Marshal()) {
			return fmt.Errorf("host key mismatch for %s: expected %s, got %s",
				hostname,
				base64.StdEncoding.EncodeToString(knownPubKey.Marshal()),
				base64.StdEncoding.EncodeToString(key.Marshal()),
			)
		}
		return nil
	}, nil
}

// Ping runs `true` on the remote over the existing SSH client to verify the
// connection is usable.
func Ping(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer session.Close()

	if err := session.Run("true"); err != nil {
		return fmt.Errorf("failed to run ping command: %w", err)
	}

	return nil
}

// DecryptEnv decrypts the VALUE portion of each KEY=VALUE entry using the
// provided encryption key, leaving keys in plaintext.  Returns nil when env
// is nil.
func DecryptEnv(env *[]string, encryptionKey string) ([]string, error) {
	if env == nil {
		return nil, nil
	}
	decrypted := make([]string, len(*env))
	for i, entry := range *env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("env entry %d %q is not in KEY=VALUE format", i, entry)
		}
		decValue, err := encryption.Decrypt(encryptionKey, parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt env entry %d: %w", i, err)
		}
		decrypted[i] = parts[0] + "=" + decValue
	}
	return decrypted, nil
}

// MergeEnv merges two env slices in KEY=VALUE format, with entries from b
// taking precedence over entries in a on duplicate keys.  Returns a new slice
// containing the merged result.
func MergeEnv(a []string, b []string) []string {
	seen := make(map[string]int)
	var merged []string

	add := func(entries []string) {
		for _, e := range entries {
			if e == "" {
				continue
			}
			key := e
			if idx := strings.Index(e, "="); idx >= 0 {
				key = e[:idx]
			}
			if pos, ok := seen[key]; ok {
				merged[pos] = e
				continue
			}
			seen[key] = len(merged)
			merged = append(merged, e)
		}
	}

	add(a)
	add(b)

	return merged
}

// buildAuthMethods returns a slice of ssh.AuthMethod built from the optional
// private key PEM and password.  Key is preferred when present; password is
// added as a fallback.
func buildAuthMethods(key string, password string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if key != "" {
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("failed to parse ssh private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if password != "" {
		methods = append(methods, ssh.Password(password))
	}

	return methods, nil
}
