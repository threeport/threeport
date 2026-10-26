package v0

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"time"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// AuthConfig contains root CA and private key for generating client certificates.
type AuthConfig struct {
	CAConfig                  *x509.Certificate
	CAPrivateKey              rsa.PrivateKey
	CA                        []byte
	CAPemEncoded              string
	CABase64Encoded           string
	CAPrivateKeyPemEncoded    string
	CAPrivateKeyBase64Encoded string
}

// GetAuthConfig populates an AuthConfig object and returns a pointer to it.
func GetAuthConfig() (*AuthConfig, error) {
	// generate certificate authority for the threeport API
	caConfig, ca, caPrivateKey, err := GenerateCACertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate authority and private key: %w", err)
	}

	// get PEM-encoded keypairs as strings to pass into deployment manifests
	caEncoded := GetCertificatePEMEncoding(ca)
	caPrivateKeyEncoded := GetPrivateKeyPEMEncoding(caPrivateKey)

	return &AuthConfig{
		CAConfig:                  caConfig,
		CAPrivateKey:              *caPrivateKey,
		CA:                        ca,
		CAPemEncoded:              caEncoded,
		CABase64Encoded:           util.Base64Encode(caEncoded),
		CAPrivateKeyPemEncoded:    caPrivateKeyEncoded,
		CAPrivateKeyBase64Encoded: util.Base64Encode(caPrivateKeyEncoded),
	}, nil
}

// LoadAuthConfig builds an AuthConfig from an existing certificate authority:
// its PEM-encoded certificate and PKCS#1 PEM-encoded RSA private key, as
// GetAuthConfig encodes them.  It is for a component that has to issue client
// certificates from the control plane's CA after install, when the CA was not
// generated in this process.
func LoadAuthConfig(caPem []byte, caPrivateKeyPem []byte) (*AuthConfig, error) {
	certBlock, _ := pem.Decode(caPem)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("CA certificate is not PEM-encoded")
	}
	caConfig, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}
	if !caConfig.IsCA {
		return nil, fmt.Errorf("certificate %q is not a certificate authority", caConfig.Subject.CommonName)
	}

	keyBlock, _ := pem.Decode(caPrivateKeyPem)
	if keyBlock == nil {
		return nil, fmt.Errorf("CA private key is not PEM-encoded")
	}
	caPrivateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	// the key has to be the one the certificate was issued for, or every
	// certificate signed with it would fail verification against the CA
	caPublicKey, ok := caConfig.PublicKey.(*rsa.PublicKey)
	if !ok || !caPublicKey.Equal(&caPrivateKey.PublicKey) {
		return nil, fmt.Errorf("CA private key does not match the CA certificate")
	}

	caEncoded := GetCertificatePEMEncoding(caConfig.Raw)
	caPrivateKeyEncoded := GetPrivateKeyPEMEncoding(caPrivateKey)

	return &AuthConfig{
		CAConfig:                  caConfig,
		CAPrivateKey:              *caPrivateKey,
		CA:                        caConfig.Raw,
		CAPemEncoded:              caEncoded,
		CABase64Encoded:           util.Base64Encode(caEncoded),
		CAPrivateKeyPemEncoded:    caPrivateKeyEncoded,
		CAPrivateKeyBase64Encoded: util.Base64Encode(caPrivateKeyEncoded),
	}, nil
}

// GenerateCACertificate generates a certificate authority and private key for the Threeport API.
func GenerateCACertificate() (caConfig *x509.Certificate, ca []byte, caPrivateKey *rsa.PrivateKey, err error) {
	// generate a random identifier for use as a serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	randomNumber, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate random serial number: %w", err)
	}

	// set config options for a new CA certificate
	caConfig = &x509.Certificate{
		SerialNumber: randomNumber,
		URIs:         []*url.URL{{Scheme: "https", Host: "localhost"}},
		DNSNames: []string{
			"localhost",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		Subject: pkix.Name{
			CommonName: "threeport",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// generate private and public keys for the CA
	caPrivateKey, err = rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// generate a certificate authority
	ca, err = x509.CreateCertificate(rand.Reader, caConfig, caConfig, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	return caConfig, ca, caPrivateKey, nil

}

// GenerateCertificate generates a certificate and private key with a given
// common name from a CA cert and key.
func GenerateCertificate(
	caConfig *x509.Certificate,
	caPrivateKey *rsa.PrivateKey,
	commonName string,
	organization string,
	organizationalUnit string,
	altNames ...string,
) (certificate string, privateKey string, err error) {
	// generate a random identifier for use as a serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	randomNumber, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random serial number: %w", err)
	}

	dnsNames := []string{"localhost"}
	ipAddresses := []net.IP{net.ParseIP("127.0.0.1")}

	// Process altNames to separate IPs and DNS names
	for _, altName := range altNames {
		if ip := net.ParseIP(altName); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		} else {
			dnsNames = append(dnsNames, altName)
		}
	}

	cert := &x509.Certificate{
		SerialNumber: randomNumber,
		URIs:         []*url.URL{{Scheme: "https", Host: "localhost"}},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
		Subject: pkix.Name{
			CommonName:         commonName,
			Organization:       []string{organization},
			OrganizationalUnit: []string{organizationalUnit},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  false,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// generate private and public keys for the CA
	serverPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// generate a certificate authority
	serverCert, err := x509.CreateCertificate(rand.Reader, cert, caConfig, &serverPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create CA certificate: %w", err)
	}

	serverCertificateEncoded := GetCertificatePEMEncoding(serverCert)
	serverPrivateKeyEncoded := GetPrivateKeyPEMEncoding(serverPrivateKey)
	return serverCertificateEncoded, serverPrivateKeyEncoded, nil
}

// GetCertificatePEMEncoding returns a PEM encoded string for a given certificate.
func GetCertificatePEMEncoding(cert []byte) string {
	return GetPEMEncoding(cert, "CERTIFICATE")
}

// GetPrivateKeyPEMEncoding returns a PEM encoded string for a given private key.
func GetPrivateKeyPEMEncoding(privateKey *rsa.PrivateKey) string {
	return GetPEMEncoding(x509.MarshalPKCS1PrivateKey(privateKey), "RSA PRIVATE KEY")
}

// GetPEMEncoding returns a PEM encoded string for a given certificate or private key.
func GetPEMEncoding(cert []byte, encodingType string) (pemEncodingString string) {
	pemEncoding := new(bytes.Buffer)
	pem.Encode(pemEncoding, &pem.Block{
		Type:  encodingType,
		Bytes: cert,
	})

	return pemEncoding.String()
}
