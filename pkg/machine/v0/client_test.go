package v0

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestBuildAuthMethods covers the three input shapes the helper supports: key
// only, password only, both, and neither.
func TestBuildAuthMethods(t *testing.T) {
	privateKeyPEM := generateTestPrivateKeyPEM(t)

	t.Run("returns empty when both inputs empty", func(t *testing.T) {
		methods, err := buildAuthMethods("", "")
		require.NoError(t, err)
		assert.Empty(t, methods)
	})

	t.Run("returns one method when only password set", func(t *testing.T) {
		methods, err := buildAuthMethods("", "hunter2")
		require.NoError(t, err)
		assert.Len(t, methods, 1)
	})

	t.Run("returns one method when only key set", func(t *testing.T) {
		methods, err := buildAuthMethods(privateKeyPEM, "")
		require.NoError(t, err)
		assert.Len(t, methods, 1)
	})

	t.Run("returns two methods when both set", func(t *testing.T) {
		methods, err := buildAuthMethods(privateKeyPEM, "hunter2")
		require.NoError(t, err)
		assert.Len(t, methods, 2)
	})

	t.Run("returns error on malformed private key", func(t *testing.T) {
		_, err := buildAuthMethods("not-a-valid-pem", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse ssh private key")
	})
}

// TestBuildHostKeyCallback covers capture mode (knownHostKey nil) and verify
// mode (knownHostKey set), including the mismatch path.
func TestBuildHostKeyCallback(t *testing.T) {
	signer := generateTestSigner(t)
	pubKey := signer.PublicKey()
	encodedKey := encodeHostKey(pubKey)

	otherSigner := generateTestSigner(t)
	otherPubKey := otherSigner.PublicKey()

	t.Run("capture mode records the presented key", func(t *testing.T) {
		var captured string
		callback, err := buildHostKeyCallback(nil, &captured)
		require.NoError(t, err)

		err = callback("host", &net.TCPAddr{}, pubKey)
		require.NoError(t, err)
		assert.Equal(t, encodedKey, captured)
	})

	t.Run("verify mode accepts matching key", func(t *testing.T) {
		known := encodedKey
		var captured string
		callback, err := buildHostKeyCallback(&known, &captured)
		require.NoError(t, err)

		err = callback("host", &net.TCPAddr{}, pubKey)
		require.NoError(t, err)
		assert.Empty(t, captured, "verify mode should not write capturedKey")
	})

	t.Run("verify mode rejects mismatched key", func(t *testing.T) {
		known := encodedKey
		var captured string
		callback, err := buildHostKeyCallback(&known, &captured)
		require.NoError(t, err)

		err = callback("host", &net.TCPAddr{}, otherPubKey)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host key mismatch")
	})

	t.Run("returns error on undecodable known key", func(t *testing.T) {
		bad := "not base64!!"
		var captured string
		_, err := buildHostKeyCallback(&bad, &captured)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode stored host key")
	})
}

// TestDecryptEnv covers the KEY=VALUE round-trip: each entry's value is
// encrypted separately, and DecryptEnv recovers the plaintext while
// preserving keys verbatim.
func TestDecryptEnv(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	t.Run("round-trips encrypted values", func(t *testing.T) {
		plain := []string{"DB_PASSWORD=hunter2", "API_KEY=abc-123"}
		encrypted := make([]string, len(plain))
		for i, entry := range plain {
			k, v, _ := strings.Cut(entry, "=")
			enc, encErr := encryption.Encrypt(key, v)
			require.NoError(t, encErr)
			encrypted[i] = k + "=" + enc
		}

		got, decErr := DecryptEnv(&encrypted, key)
		require.NoError(t, decErr)
		assert.Equal(t, plain, got)
	})

	t.Run("rejects entry without =", func(t *testing.T) {
		_, err := DecryptEnv(&[]string{"NOEQUALS"}, key)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in KEY=VALUE format")
	})

	t.Run("rejects invalid ciphertext", func(t *testing.T) {
		_, err := DecryptEnv(&[]string{"FOO=not-real-cipher"}, key)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt env entry")
	})

	t.Run("nil pointer returns nil result", func(t *testing.T) {
		got, err := DecryptEnv(nil, key)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// TestMergeEnv covers the merge semantics: b overrides a on key collisions,
// order is stable based on first appearance, empty entries dropped, entries
// without = merged by full string. Pins behavior from PR comment 3337202617.
func TestMergeEnv(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "both empty",
			a:    nil,
			b:    nil,
			want: nil,
		},
		{
			name: "only a",
			a:    []string{"FOO=1", "BAR=2"},
			b:    nil,
			want: []string{"FOO=1", "BAR=2"},
		},
		{
			name: "only b",
			a:    nil,
			b:    []string{"FOO=1", "BAR=2"},
			want: []string{"FOO=1", "BAR=2"},
		},
		{
			name: "disjoint keys preserve order",
			a:    []string{"FOO=1"},
			b:    []string{"BAR=2"},
			want: []string{"FOO=1", "BAR=2"},
		},
		{
			name: "b overrides a on collision",
			a:    []string{"FOO=1", "BAR=2"},
			b:    []string{"FOO=99"},
			want: []string{"FOO=99", "BAR=2"},
		},
		{
			name: "b override preserves a's position",
			a:    []string{"FOO=1", "BAR=2", "BAZ=3"},
			b:    []string{"BAR=99"},
			want: []string{"FOO=1", "BAR=99", "BAZ=3"},
		},
		{
			name: "empty entries dropped",
			a:    []string{"", "FOO=1", ""},
			b:    []string{"", "BAR=2"},
			want: []string{"FOO=1", "BAR=2"},
		},
		{
			name: "entries without = use full string as key",
			a:    []string{"FOO"},
			b:    []string{"FOO"},
			want: []string{"FOO"},
		},
		{
			name: "duplicate keys within b: last write wins",
			a:    nil,
			b:    []string{"FOO=1", "FOO=2"},
			want: []string{"FOO=2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeEnv(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGetClient_CapturesHostKey stands up an in-process ssh server, dials it
// via GetClient with HostKey nil, and verifies the returned captured key
// matches the server's host key. Also pings the resulting client.
func TestGetClient_CapturesHostKey(t *testing.T) {
	serverSigner := generateTestSigner(t)
	expectedHostKey := encodeHostKey(serverSigner.PublicKey())

	addr, stop := startTestSSHServer(t, serverSigner, "testuser", "testpassword")
	defer stop()

	host, port := splitHostPort(t, addr)

	mri := &v0.MachineRuntimeInstance{
		Hostname:    util.Ptr(host),
		SSHUser:     util.Ptr("testuser"),
		SSHPassword: encryptString(t, "testpassword"),
		Port:        util.Ptr(port),
	}

	client, captured, err := GetClient(mri, mustEncryptionKey(t))
	require.NoError(t, err)
	defer client.Close()
	assert.Equal(t, expectedHostKey, captured)

	err = Ping(client)
	assert.NoError(t, err)
}

// TestGetClient_VerifiesHostKeyMatch verifies the success path of host-key
// verification: stored key matches server, dial succeeds.
func TestGetClient_VerifiesHostKeyMatch(t *testing.T) {
	serverSigner := generateTestSigner(t)
	knownHostKey := encodeHostKey(serverSigner.PublicKey())

	addr, stop := startTestSSHServer(t, serverSigner, "testuser", "testpassword")
	defer stop()

	host, port := splitHostPort(t, addr)

	mri := &v0.MachineRuntimeInstance{
		Hostname:    util.Ptr(host),
		SSHUser:     util.Ptr("testuser"),
		SSHPassword: encryptString(t, "testpassword"),
		Port:        util.Ptr(port),
		HostKey:     util.Ptr(knownHostKey),
	}

	client, captured, err := GetClient(mri, mustEncryptionKey(t))
	require.NoError(t, err)
	defer client.Close()
	assert.Empty(t, captured, "verify mode should not populate captured key")
}

// TestGetClient_VerifiesHostKeyMismatch verifies the failure path: dial
// fails when the stored host key doesn't match the server's actual key.
func TestGetClient_VerifiesHostKeyMismatch(t *testing.T) {
	serverSigner := generateTestSigner(t)
	otherSigner := generateTestSigner(t)
	wrongHostKey := encodeHostKey(otherSigner.PublicKey())

	addr, stop := startTestSSHServer(t, serverSigner, "testuser", "testpassword")
	defer stop()

	host, port := splitHostPort(t, addr)

	mri := &v0.MachineRuntimeInstance{
		Hostname:    util.Ptr(host),
		SSHUser:     util.Ptr("testuser"),
		SSHPassword: encryptString(t, "testpassword"),
		Port:        util.Ptr(port),
		HostKey:     util.Ptr(wrongHostKey),
	}

	_, _, err := GetClient(mri, mustEncryptionKey(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host key mismatch")
}

// TestGetClient_DecryptError verifies a bad encryption key surfaces as a
// decrypt error before any dial is attempted.
func TestGetClient_DecryptError(t *testing.T) {
	mustEncryptionKey(t)

	mri := &v0.MachineRuntimeInstance{
		Hostname:    util.Ptr("127.0.0.1"),
		SSHUser:     util.Ptr("testuser"),
		SSHPassword: util.Ptr("not-real-cipher"),
		Port:        util.Ptr(2222),
	}

	_, _, err := GetClient(mri, mustEncryptionKey(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt ssh password")
}

// envSecretRow is a self-contained gorm model that mirrors the encrypt-tag +
// jsonb-serializer shape used on MachineWorkloadDefinition.Env, scoped to
// this test file so the end-to-end exercise doesn't depend on the real api
// type's other invariants (foreign keys, validate hooks, etc.). Env is
// *[]string here so the test exercises the production *[]string + encrypt
// hook path used by MachineWorkloadDefinition.
type envSecretRow struct {
	ID  uint      `gorm:"primaryKey"`
	Env *[]string `gorm:"serializer:json" encrypt:"true"`
}

// EncryptedFields advertises Env to the encrypt-hook dispatch.
func (r *envSecretRow) EncryptedFields() []apilib.EncryptedField {
	return []apilib.EncryptedField{{Name: "Env", Value: r.Env}}
}

// BeforeCreate forwards to the production encrypt-hook so the test
// exercises the real ProcessEncryptTaggedFields path.
func (r *envSecretRow) BeforeCreate(tx *gorm.DB) error {
	return apilib.ProcessEncryptTaggedFields(tx, r)
}

// TestEncryptHookEnvSliceJSONBRoundtrip exercises the encrypt-hook + gorm
// json serializer combo end-to-end on the same shape MachineWorkloadDefinition
// uses: keys remain plaintext (LIKE-queryable in production), values land
// as ciphertext in the JSON blob, and the full round-trip through
// DecryptValues recovers the original plaintext.
func TestEncryptHookEnvSliceJSONBRoundtrip(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("ENCRYPTION_KEY", key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&envSecretRow{}))

	plainEnv := []string{"DB_PASSWORD=hunter2", "API_KEY=abc-123"}
	envCopy := append([]string{}, plainEnv...)
	obj := &envSecretRow{Env: &envCopy}
	require.NoError(t, db.Create(obj).Error)

	var stored envSecretRow
	require.NoError(t, db.First(&stored, obj.ID).Error)
	require.NotNil(t, stored.Env)
	require.Len(t, *stored.Env, len(plainEnv))

	// per-entry: key stays plaintext, value is ciphertext that decrypts back
	for i, entry := range *stored.Env {
		wantKey, wantValue, _ := strings.Cut(plainEnv[i], "=")
		gotKey, gotCipher, ok := strings.Cut(entry, "=")
		require.True(t, ok, "stored entry %d should still be KEY=VALUE shape", i)
		assert.Equal(t, wantKey, gotKey, "key portion must remain plaintext (LIKE-queryable)")
		assert.NotEqual(t, wantValue, gotCipher, "value portion must be ciphertext, not plaintext")

		decValue, decErr := encryption.Decrypt(key, gotCipher)
		require.NoError(t, decErr, "stored value portion should be valid ciphertext")
		assert.Equal(t, wantValue, decValue)
	}

	// DecryptValues recovers the full plaintext slice via EncryptedFields().
	_, err = apilib.DecryptValues(&stored, key)
	require.NoError(t, err)
	require.NotNil(t, stored.Env)
	assert.Equal(t, plainEnv, *stored.Env)
}

// --- test helpers ---

// mustEncryptionKey returns the current ENCRYPTION_KEY or generates one
// and sets it on the env so in-process code that calls encryption.KeyFromEnv
// finds it.
func mustEncryptionKey(t *testing.T) string {
	t.Helper()
	if existing := os.Getenv(encryption.KeyEnvVar); existing != "" {
		return existing
	}
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv(encryption.KeyEnvVar, key)
	return key
}

// encryptString encrypts s under the current ENCRYPTION_KEY and returns a
// *string suitable for an api type's encrypted field.
func encryptString(t *testing.T, s string) *string {
	t.Helper()
	key := mustEncryptionKey(t)
	cipher, err := encryption.Encrypt(key, s)
	require.NoError(t, err)
	return &cipher
}

// generateTestPrivateKeyPEM returns a PEM-encoded RSA private key suitable
// for ssh.ParsePrivateKey.
func generateTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privDER := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	})
	return string(pemBytes)
}

// generateTestSigner returns a fresh ssh.Signer backed by an in-memory RSA
// key, for use as either a server host key or a client credential.
func generateTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

// encodeHostKey returns the base64 form GetClient uses on capture and
// expects on verify.
func encodeHostKey(pub ssh.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub.Marshal())
}

// startTestSSHServer accepts connections on a loopback TCP listener and
// handles each by completing the SSH handshake, accepting session
// channels, and replying with exit-status 0 on every exec/shell request.
// Returns the listener address and a stop function.
func startTestSSHServer(t *testing.T, hostKey ssh.Signer, user, password string) (string, func()) {
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
				select {
				case <-stopped:
				default:
				}
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveSSHConn(conn, config)
			}()
		}
	}()

	stop := func() {
		close(stopped)
		_ = listener.Close()
		wg.Wait()
	}
	return listener.Addr().String(), stop
}

// serveSSHConn handles one accepted TCP conn: complete the SSH handshake,
// then accept session channels and reply exit-status 0 to exec requests.
func serveSSHConn(nConn net.Conn, config *ssh.ServerConfig) {
	defer nConn.Close()
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
		go handleSession(channel, requests)
	}
}

// handleSession processes one session channel: ack exec/shell requests
// and reply exit-status 0. Drains stdin so client writes return cleanly.
func handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	go func() {
		_, _ = io.Copy(io.Discard, channel)
	}()
	for req := range requests {
		switch req.Type {
		case "exec", "shell":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// splitHostPort parses a TCP listener address string and returns the host
// portion and integer port.
func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return host, port
}
