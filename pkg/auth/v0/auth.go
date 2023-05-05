package auth

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

	"github.com/threeport/threeport/internal/util"
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
			"threeport-api-server",
			"threeport-api-server.threeport-control-plane",
			"threeport-api-server.threeport-control-plane.svc",
			"threeport-api-server.threeport-control-plane.svc.cluster",
			"threeport-api-server.threeport-control-plane.svc.cluster.local",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Threeport"},
			Country:      []string{"US"},
			Locality:     []string{"Tampa"},
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

// GenerateCertificate generates a certificate and private key for the current Threeport instance.
func GenerateCertificate(caConfig *x509.Certificate, caPrivateKey *rsa.PrivateKey) (certificate string, privateKey string, err error) {

	// generate a random identifier for use as a serial number
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil)
	randomNumber, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random serial number: %w", err)
	}

	// set config options for a new CA certificate
	cert := &x509.Certificate{
		SerialNumber: randomNumber,
		URIs:         []*url.URL{{Scheme: "https", Host: "localhost"}},
		DNSNames: []string{
			"localhost",
			"threeport-api-server",
			"threeport-api-server.threeport-control-plane",
			"threeport-api-server.threeport-control-plane.svc",
			"threeport-api-server.threeport-control-plane.svc.cluster",
			"threeport-api-server.threeport-control-plane.svc.cluster.local",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Threeport"},
			Country:      []string{"US"},
			Locality:     []string{"Tampa"},
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
