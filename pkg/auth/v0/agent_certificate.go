package v0

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"
)

const (
	// maxCommonNameLength is the longest common name RFC 5280 allows.
	maxCommonNameLength = 64

	// minClientKeyBits is the smallest RSA key a certificate request may carry.
	minClientKeyBits = 2048

	// clientCertificateBackdate leaves room for clock skew between the cluster
	// the certificate is issued on and the cluster it is used from.
	clientCertificateBackdate = 5 * time.Minute

	// csrPemType is the PEM block type of a certificate signing request.
	csrPemType = "CERTIFICATE REQUEST"
)

// ErrInvalidCertificateRequest is wrapped by every error that comes from the
// certificate request the caller supplied, as opposed to a failure of the CA, so
// a server can tell a request it should refuse from a fault of its own.
var ErrInvalidCertificateRequest = errors.New("invalid certificate request")

// runtimeNamePattern is what a runtime instance name has to look like to be
// part of a common name.
var runtimeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateRuntimeName reports whether name can be used to build an agent's
// common name.
func ValidateRuntimeName(name string) error {
	if name == "" {
		return fmt.Errorf("runtime instance name is empty")
	}
	if len(name) > 253 {
		return fmt.Errorf("runtime instance name is longer than 253 characters")
	}
	if !runtimeNamePattern.MatchString(name) {
		return fmt.Errorf("runtime instance name %q has characters a certificate common name cannot take", name)
	}

	return nil
}

// AgentCommonName returns the common name of the client certificate issued to
// the agent on the named runtime instance, so each runtime's agent is
// distinguishable in the API's logs.  The name is always AgentCommonNamePrefix
// followed by a dash and the runtime name, which is what keeps an issued
// certificate from ever carrying another component's name.  A runtime name too
// long to leave the result within the 64 characters RFC 5280 allows is
// shortened and given a hash of the whole name, so two long names still differ.
// The name has to pass ValidateRuntimeName.
func AgentCommonName(runtimeName string) string {
	prefix := AgentCommonNamePrefix + "-"
	if len(prefix)+len(runtimeName) <= maxCommonNameLength {
		return prefix + runtimeName
	}

	sum := sha256.Sum256([]byte(runtimeName))
	suffix := "-" + hex.EncodeToString(sum[:4])
	keep := maxCommonNameLength - len(prefix) - len(suffix)

	return prefix + runtimeName[:keep] + suffix
}

// GenerateCertificateRequest generates an RSA private key and a certificate
// signing request for it, both PEM-encoded.  The subject asks for commonName,
// but a CA that signs the request with SignClientCertificateRequest decides the
// subject itself.
func GenerateCertificateRequest(commonName string) (csrPem string, privateKeyPem string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	csrDer, err := x509.CreateCertificateRequest(
		rand.Reader,
		&x509.CertificateRequest{Subject: pkix.Name{CommonName: commonName}},
		privateKey,
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate request: %w", err)
	}

	return GetPEMEncoding(csrDer, csrPemType), GetPrivateKeyPEMEncoding(privateKey), nil
}

// SignClientCertificateRequest issues a client certificate from the CA for the
// public key in a PEM-encoded certificate signing request.
//
// Only the public key is taken from the request.  Its subject, alternative
// names and extensions are ignored: the certificate carries the common name,
// organization and organizational unit passed here, is valid for client
// authentication only, and is not a CA.  Who may ask for which identity is the
// caller's decision to make before calling this.
//
// It returns the PEM-encoded certificate and its serial number.
func SignClientCertificateRequest(
	authConfig *AuthConfig,
	csrPem string,
	commonName string,
	organization string,
	organizationalUnit string,
) (certificate string, serialNumber string, err error) {
	if authConfig == nil || authConfig.CAConfig == nil {
		return "", "", fmt.Errorf("no certificate authority to sign with")
	}

	block, _ := pem.Decode([]byte(csrPem))
	if block == nil || block.Type != csrPemType {
		return "", "", fmt.Errorf("%w: not PEM-encoded", ErrInvalidCertificateRequest)
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("%w: failed to parse: %v", ErrInvalidCertificateRequest, err)
	}
	// proves the requester holds the private key for the public key it sent
	if err := request.CheckSignature(); err != nil {
		return "", "", fmt.Errorf("%w: signature is invalid: %v", ErrInvalidCertificateRequest, err)
	}
	publicKey, ok := request.PublicKey.(*rsa.PublicKey)
	if !ok {
		return "", "", fmt.Errorf("%w: no RSA public key", ErrInvalidCertificateRequest)
	}
	if publicKey.N.BitLen() < minClientKeyBits {
		return "", "", fmt.Errorf("%w: key is %d bits, at least %d are required", ErrInvalidCertificateRequest, publicKey.N.BitLen(), minClientKeyBits)
	}

	// a random identifier for the serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	serial, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:         commonName,
			Organization:       []string{organization},
			OrganizationalUnit: []string{organizationalUnit},
		},
		NotBefore:             now.Add(-clientCertificateBackdate),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  false,
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, authConfig.CAConfig, publicKey, &authConfig.CAPrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate: %w", err)
	}

	return GetCertificatePEMEncoding(der), serial.Text(16), nil
}
