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
	"strings"
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

// agentNamePattern is what a runtime instance name has to look like to be used
// in a common name as it is.
var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// AgentCommonName returns the common name of the client certificate issued to
// the agent on the named runtime instance, so each runtime's agent is
// distinguishable in the API's logs.  It is AgentCommonNamePrefix followed by a
// dash and a form of the runtime name, which is what keeps an issued
// certificate from ever carrying another component's name.
//
// It works for any name the API accepts for a runtime instance, which is any
// non-empty string.  A name that is already made of letters, digits, dots,
// underscores and dashes, and short enough to leave the result within the 64
// characters RFC 5280 allows, is used as it is.  Any other name is made into
// one: unsupported characters become dashes, the result is cut to fit, and a
// hash of the whole name is added, so two names that differ only in what was
// changed still get different common names.
func AgentCommonName(runtimeName string) string {
	prefix := AgentCommonNamePrefix + "-"
	maxName := maxCommonNameLength - len(prefix)
	if len(runtimeName) <= maxName && agentNamePattern.MatchString(runtimeName) {
		return prefix + runtimeName
	}

	sum := sha256.Sum256([]byte(runtimeName))
	hash := hex.EncodeToString(sum[:4])

	// the readable part is what is left of the name once it is safe to print
	var readable strings.Builder
	for _, char := range runtimeName {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9',
			char == '.', char == '_', char == '-':
			readable.WriteRune(char)
		default:
			readable.WriteRune('-')
		}
	}
	// a name starts with a letter or digit, and the hash follows after a dash
	name := strings.TrimLeft(readable.String(), "-._")
	if keep := maxName - len(hash) - 1; len(name) > keep {
		name = name[:keep]
	}
	name = strings.TrimRight(name, "-._")
	if name == "" {
		return prefix + hash
	}

	return prefix + name + "-" + hash
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
