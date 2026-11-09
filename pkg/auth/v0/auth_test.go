package v0

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadAuthConfig covers rebuilding a CA from its PEM encoding: the result
// has to sign certificates the original CA's certificate verifies.
func TestLoadAuthConfig(t *testing.T) {
	generated, err := GetAuthConfig()
	require.NoError(t, err)

	t.Run("a loaded CA signs certificates the CA certificate verifies", func(t *testing.T) {
		loaded, err := LoadAuthConfig(
			[]byte(generated.CAPemEncoded),
			[]byte(generated.CAPrivateKeyPemEncoded),
		)
		require.NoError(t, err)
		assert.Equal(t, generated.CAPemEncoded, loaded.CAPemEncoded)

		certPem, _, err := GenerateCertificate(
			loaded.CAConfig, &loaded.CAPrivateKey, "threeport-agent-test", "org", OUControlPlane,
		)
		require.NoError(t, err)

		block, _ := pem.Decode([]byte(certPem))
		require.NotNil(t, block)
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)

		roots := x509.NewCertPool()
		require.True(t, roots.AppendCertsFromPEM([]byte(generated.CAPemEncoded)))
		_, err = cert.Verify(x509.VerifyOptions{
			Roots:     roots,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		assert.NoError(t, err)
	})

	t.Run("a key from another CA is refused", func(t *testing.T) {
		other, err := GetAuthConfig()
		require.NoError(t, err)

		_, err = LoadAuthConfig(
			[]byte(generated.CAPemEncoded),
			[]byte(other.CAPrivateKeyPemEncoded),
		)
		assert.ErrorContains(t, err, "does not match")
	})

	t.Run("input that is not PEM is refused", func(t *testing.T) {
		_, err := LoadAuthConfig([]byte("nope"), []byte(generated.CAPrivateKeyPemEncoded))
		assert.Error(t, err)

		_, err = LoadAuthConfig([]byte(generated.CAPemEncoded), []byte("nope"))
		assert.Error(t, err)
	})
}
