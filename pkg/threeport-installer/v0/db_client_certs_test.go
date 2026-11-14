package v0

import (
	"testing"

	"github.com/stretchr/testify/require"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestEnsureDbClientSecretsUsesTheStoredCA checks a retry against a stored database CA.
func TestEnsureDbClientSecretsUsesTheStoredCA(t *testing.T) {
	// seed the database secret and a fresh CA
	_, kubeClient, cpi := newBrokerTestClient(dbCertsSecret("stored-ca", "stored-root", "stored-threeport"))

	// write the client secrets
	err := cpi.ensureDbClientSecrets(kubeClient, installerTestMapper(), freshDbCreds("fresh-ca"))
	require.NoError(t, err)

	// read the CA the client secrets hold
	require.Equal(t, "stored-ca", testSecretValue(t, kubeClient, dbRootCertSecretName, "ca.crt"))
	require.Equal(t, "stored-root-cert", testSecretValue(t, kubeClient, dbRootCertSecretName, "client.root.crt"))
	require.Equal(t, "stored-ca", testSecretValue(t, kubeClient, dbThreeportCertSecretName, "ca.crt"))
	require.Equal(t, "stored-threeport-cert", testSecretValue(t, kubeClient, dbThreeportCertSecretName, "client.threeport.crt"))
}

// TestEnsureDbClientSecretsUsesTheAttemptWhenNothingIsStored checks the first write.
func TestEnsureDbClientSecretsUsesTheAttemptWhenNothingIsStored(t *testing.T) {
	// start with no database secret
	_, kubeClient, cpi := newBrokerTestClient()

	// write the client secrets from this attempt
	err := cpi.ensureDbClientSecrets(kubeClient, installerTestMapper(), freshDbCreds("fresh-ca"))
	require.NoError(t, err)

	// read the CA the client secrets hold
	require.Equal(t, "fresh-ca", testSecretValue(t, kubeClient, dbRootCertSecretName, "ca.crt"))
	require.Equal(t, "fresh-ca-root-cert", testSecretValue(t, kubeClient, dbRootCertSecretName, "client.root.crt"))
	require.Equal(t, "fresh-ca", testSecretValue(t, kubeClient, dbThreeportCertSecretName, "ca.crt"))
	require.Equal(t, "fresh-ca-threeport-cert", testSecretValue(t, kubeClient, dbThreeportCertSecretName, "client.threeport.crt"))
}

// freshDbCreds returns credentials from one generated CA.
func freshDbCreds(ca string) *auth.DbCreds {
	// build the credentials
	return &auth.DbCreds{
		AuthConfig:    &auth.AuthConfig{CAPemEncoded: ca},
		RootCert:      ca + "-root-cert",
		RootKey:       ca + "-root-key",
		ThreeportCert: ca + "-threeport-cert",
		ThreeportKey:  ca + "-threeport-key",
	}
}

// dbCertsSecret returns the secret the database pods mount.
func dbCertsSecret(ca, root, threeport string) runtime.Object {
	// build the stored secret
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      dbCredsSecretName,
			"namespace": testControlPlaneNamespace,
		},
		"stringData": map[string]interface{}{
			"ca.crt":               ca,
			"client.root.crt":      root + "-cert",
			"client.root.key":      root + "-key",
			"client.threeport.crt": threeport + "-cert",
			"client.threeport.key": threeport + "-key",
		},
	}}
}
