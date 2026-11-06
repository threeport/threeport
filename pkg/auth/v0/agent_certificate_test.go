package v0

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testCaOnce   sync.Once
	testCa       *AuthConfig
	testCaErr    error
	testCsrOnce  sync.Once
	testCsr      string
	testCsrKey   string
	testCsrError error
)

// testAuthConfig returns one CA for the whole package: generating a 4096-bit
// key is the slow part of every test here.
func testAuthConfig(t *testing.T) *AuthConfig {
	t.Helper()
	testCaOnce.Do(func() { testCa, testCaErr = GetAuthConfig() })
	require.NoError(t, testCaErr)
	return testCa
}

// testRequest returns one certificate request and its key for the whole package.
func testRequest(t *testing.T) (string, string) {
	t.Helper()
	testCsrOnce.Do(func() { testCsr, testCsrKey, testCsrError = GenerateCertificateRequest("whatever-the-requester-says") })
	require.NoError(t, testCsrError)
	return testCsr, testCsrKey
}

// parseCertificate decodes a PEM certificate.
func parseCertificate(t *testing.T, certPem string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(certPem))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

// requestFor builds a certificate request with the given key and subject.
func requestFor(t *testing.T, key *rsa.PrivateKey, template *x509.CertificateRequest) string {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	require.NoError(t, err)
	return GetPEMEncoding(der, "CERTIFICATE REQUEST")
}

// TestSignClientCertificateRequestIssuesTheIdentityTheCallerChose covers what
// the certificate says: the signer's identity, not the requester's.
func TestSignClientCertificateRequestIssuesTheIdentityTheCallerChose(t *testing.T) {
	ca := testAuthConfig(t)
	csr, keyPem := testRequest(t)

	certPem, serial, err := SignClientCertificateRequest(ca, csr, "threeport-agent-east", "threeport.io", OUControlPlane)
	require.NoError(t, err)

	cert := parseCertificate(t, certPem)
	assert.Equal(t, "threeport-agent-east", cert.Subject.CommonName)
	assert.Equal(t, []string{"threeport.io"}, cert.Subject.Organization)
	assert.Equal(t, []string{OUControlPlane}, cert.Subject.OrganizationalUnit)
	assert.Equal(t, serial, cert.SerialNumber.Text(16))
	assert.False(t, cert.IsCA)
	assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, cert.ExtKeyUsage)
	assert.Zero(t, cert.KeyUsage&x509.KeyUsageCertSign, "a client certificate must not sign certificates")
	assert.Empty(t, cert.DNSNames)

	// it is the requester's key that the certificate is for
	block, _ := pem.Decode([]byte(keyPem))
	require.NotNil(t, block)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	assert.True(t, cert.PublicKey.(*rsa.PublicKey).Equal(&key.PublicKey))

	// and the API's check of a client certificate passes
	pool := x509.NewCertPool()
	pool.AddCert(parseCertificate(t, ca.CAPemEncoded))
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	assert.NoError(t, err)
}

// TestSignClientCertificateRequestIgnoresWhatTheRequestAsksFor covers a request
// that tries to bring its own subject, names and CA flag.
func TestSignClientCertificateRequestIgnoresWhatTheRequestAsksFor(t *testing.T) {
	ca := testAuthConfig(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	csr := requestFor(t, key, &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         "threeport-owner",
			Organization:       []string{"evil.example"},
			OrganizationalUnit: []string{"database"},
		},
		DNSNames: []string{"threeport-api-server"},
		ExtraExtensions: []pkix.Extension{
			// basicConstraints CA:TRUE
			{Id: []int{2, 5, 29, 19}, Critical: true, Value: []byte{0x30, 0x03, 0x01, 0x01, 0xff}},
		},
	})

	certPem, _, err := SignClientCertificateRequest(ca, csr, "threeport-agent-east", "threeport.io", OUControlPlane)
	require.NoError(t, err)

	cert := parseCertificate(t, certPem)
	assert.Equal(t, "threeport-agent-east", cert.Subject.CommonName)
	assert.Equal(t, []string{"threeport.io"}, cert.Subject.Organization)
	assert.Equal(t, []string{OUControlPlane}, cert.Subject.OrganizationalUnit)
	assert.Empty(t, cert.DNSNames)
	assert.False(t, cert.IsCA)
}

// TestSignClientCertificateRequestRejectsBadRequests covers a request that is
// not a valid proof of a usable key.
func TestSignClientCertificateRequestRejectsBadRequests(t *testing.T) {
	ca := testAuthConfig(t)
	good, _ := testRequest(t)

	smallKey, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)

	// a request whose signature does not match its key: swap the signature
	// bytes for the tail of another request's
	block, _ := pem.Decode([]byte(good))
	tampered := append([]byte{}, block.Bytes...)
	tampered[len(tampered)-1] ^= 0xff
	tamperedPem := GetPEMEncoding(tampered, "CERTIFICATE REQUEST")

	cases := []struct {
		name string
		csr  string
		want string
	}{
		{"empty", "", "not PEM-encoded"},
		{"wrong block type", GetPEMEncoding(block.Bytes, "CERTIFICATE"), "not PEM-encoded"},
		{"not a request", GetPEMEncoding([]byte("junk"), "CERTIFICATE REQUEST"), "failed to parse"},
		{"bad signature", tamperedPem, "signature is invalid"},
		{"small key", requestFor(t, smallKey, &x509.CertificateRequest{}), "1024 bits"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := SignClientCertificateRequest(ca, tc.csr, "threeport-agent-east", "threeport.io", OUControlPlane)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.ErrorIs(t, err, ErrInvalidCertificateRequest)
		})
	}

	_, _, err = SignClientCertificateRequest(nil, good, "threeport-agent-east", "threeport.io", OUControlPlane)
	assert.Error(t, err, "no CA is an error, not a panic")
}

// TestAgentCommonName covers the name an agent's certificate carries.
func TestAgentCommonName(t *testing.T) {
	assert.Equal(t, "threeport-agent-east", AgentCommonName("east"))

	// a name that already fits is kept whole
	fits := strings.Repeat("a", maxCommonNameLength-len(AgentCommonNamePrefix)-1)
	assert.Equal(t, AgentCommonNamePrefix+"-"+fits, AgentCommonName(fits))

	// a name one character too long is shortened, and stays within the limit
	long := fits + "b"
	other := fits + "c"
	assert.LessOrEqual(t, len(AgentCommonName(long)), maxCommonNameLength)
	assert.True(t, strings.HasPrefix(AgentCommonName(long), AgentCommonNamePrefix+"-"))
	assert.NotEqual(t, AgentCommonName(long), AgentCommonName(other), "names that differ only at the end stay distinct")
	assert.Equal(t, AgentCommonName(long), AgentCommonName(long), "the same name always gives the same common name")
}

// TestValidateRuntimeName covers which runtime names may enter a common name.
func TestValidateRuntimeName(t *testing.T) {
	for _, ok := range []string{"east", "dev-21", "gke_prod.1", "A1"} {
		assert.NoError(t, ValidateRuntimeName(ok), ok)
	}
	for _, bad := range []string{"", "-lead", ".lead", "has space", "a/b", "a\nb", "a,CN=threeport-owner", strings.Repeat("a", 254)} {
		assert.Error(t, ValidateRuntimeName(bad), bad)
	}
}

// TestLoadAuthConfigRoundTrips covers loading a CA from its PEM files and the
// mismatches it has to refuse.
func TestLoadAuthConfigRoundTrips(t *testing.T) {
	ca := testAuthConfig(t)

	loaded, err := LoadAuthConfig([]byte(ca.CAPemEncoded), []byte(ca.CAPrivateKeyPemEncoded))
	require.NoError(t, err)
	assert.Equal(t, ca.CAPemEncoded, loaded.CAPemEncoded)

	// the loaded CA signs what the original CA's pool verifies
	certPem, _, err := SignClientCertificateRequest(loaded, func() string { c, _ := testRequest(t); return c }(), "threeport-agent-east", "threeport.io", OUControlPlane)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(parseCertificate(t, ca.CAPemEncoded))
	_, err = parseCertificate(t, certPem).Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	assert.NoError(t, err)

	// another key does not belong to this CA
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	_, err = LoadAuthConfig([]byte(ca.CAPemEncoded), []byte(GetPrivateKeyPEMEncoding(otherKey)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")

	// a leaf is not a CA
	leafPem, _, err := SignClientCertificateRequest(ca, func() string { c, _ := testRequest(t); return c }(), "x", "y", "z")
	require.NoError(t, err)
	_, err = LoadAuthConfig([]byte(leafPem), []byte(ca.CAPrivateKeyPemEncoded))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a certificate authority")

	_, err = LoadAuthConfig([]byte("junk"), []byte(ca.CAPrivateKeyPemEncoded))
	assert.Error(t, err)
	_, err = LoadAuthConfig([]byte(ca.CAPemEncoded), []byte("junk"))
	assert.Error(t, err)
}
