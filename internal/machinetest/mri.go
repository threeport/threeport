package machinetest

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// NewEncryptionKey returns a fresh base64-encoded AES-256 key for use in
// tests that need to encrypt SSH credentials or env values.
func NewEncryptionKey(t *testing.T) string {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	return key
}

// encryptOrFail encrypts plaintext with key and fails the test on error.
// Unexported because only MRIFromAddr in this package consumes it.
func encryptOrFail(t *testing.T, key, plaintext string) string {
	t.Helper()
	ct, err := encryption.Encrypt(key, plaintext)
	require.NoError(t, err)
	return ct
}

// MRIInfraOpts carries the optional host key and infra provisioning fields
// for NewMRIWithInfra. Zero-value fields are left unset on the instance.
type MRIInfraOpts struct {
	// HostKey, when non-empty, is set on the instance so GetClient runs in
	// verification mode instead of capture mode. It must be base64 of the
	// SSH wire-format public key, matching how captured host keys are
	// stored; HostKeyFromSigner produces it from a test server's signer.
	HostKey string

	Region    string
	NetworkID string

	// MachineRuntimeDefinitionID, when non-zero, links the instance to a
	// definition. A provisioned machine has one; an imported machine leaves
	// it unset.
	MachineRuntimeDefinitionID uint

	// ResourceInventory is raw JSON stored on the instance.
	ResourceInventory string
}

// NewMRIWithInfra builds a *v0.MachineRuntimeInstance like MRIFromAddr,
// then sets any non-zero fields from opts.
func NewMRIWithInfra(
	t *testing.T,
	id uint,
	name, addr, user, password, key string,
	opts MRIInfraOpts,
) *v0.MachineRuntimeInstance {
	t.Helper()
	mri := MRIFromAddr(t, id, name, addr, user, password, key)
	if opts.HostKey != "" {
		mri.HostKey = util.Ptr(opts.HostKey)
	}
	if opts.Region != "" {
		mri.Region = util.Ptr(opts.Region)
	}
	if opts.NetworkID != "" {
		mri.NetworkID = util.Ptr(opts.NetworkID)
	}
	if opts.MachineRuntimeDefinitionID != 0 {
		mri.MachineRuntimeDefinitionID = util.Ptr(opts.MachineRuntimeDefinitionID)
	}
	if opts.ResourceInventory != "" {
		inventory := datatypes.JSON([]byte(opts.ResourceInventory))
		mri.ResourceInventory = &inventory
	}
	return mri
}

// MRIFromAddr builds a *v0.MachineRuntimeInstance pointing at addr (a
// "host:port" string from StartSSHServer) with the given user + password.
// The password is encrypted with key. Reconciled defaults to true and
// HostKey is left nil so GetClient runs in capture mode.
func MRIFromAddr(
	t *testing.T,
	id uint,
	name, addr, user, password, key string,
) *v0.MachineRuntimeInstance {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return &v0.MachineRuntimeInstance{
		Common: v0.Common{ID: util.Ptr(id)},
		Reconciliation: v0.Reconciliation{
			Reconciled: util.Ptr(true),
		},
		Instance: v0.Instance{Name: util.Ptr(name)},
		Hostname: util.Ptr(host),
		Port:     util.Ptr(port),
		SSHUser:  util.Ptr(user),
		// SSHPassword is stored encrypted at rest; GetClient decrypts it
		// before building the ssh.AuthMethod.
		SSHPassword: util.Ptr(encryptOrFail(t, key, password)),
	}
}
