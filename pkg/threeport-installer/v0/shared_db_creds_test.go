package v0

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// sharedDbCredsMutex guards sharedDbCredsByNamespace.
var sharedDbCredsMutex sync.Mutex

// sharedDbCredsByNamespace holds the database credentials generated for each
// namespace. Generating a set takes four RSA 4096 bit keys, which is seconds
// per test, so tests that only need a valid set share one.
var sharedDbCredsByNamespace = map[string]*auth.DbCreds{}

// sharedDbCreds returns database credentials for a namespace, generating them
// the first time a test asks. Callers must not modify the result.
func sharedDbCreds(t *testing.T, namespace string) *auth.DbCreds {
	t.Helper()

	// reuse the set already generated for the namespace
	sharedDbCredsMutex.Lock()
	defer sharedDbCredsMutex.Unlock()
	if creds, ok := sharedDbCredsByNamespace[namespace]; ok {
		return creds
	}

	// generate and keep a set for the namespace
	creds, err := auth.GenerateDbCreds(namespace)
	require.NoError(t, err)
	sharedDbCredsByNamespace[namespace] = creds

	return creds
}

// brokerSecretNames are the objects ensureMessageBrokerSecrets creates.
var brokerSecretNames = []string{
	natsCASecretName,
	natsServerCertSecretName,
	NatsCACertSecretName,
	natsCredentialsSecretName,
}

// sharedBrokerSecretsMutex guards sharedBrokerSecretsByNamespace.
var sharedBrokerSecretsMutex sync.Mutex

// sharedBrokerSecretsByNamespace holds the message broker secrets generated
// for each namespace. A set takes two RSA 4096 bit keys, the CA's and the
// server's, so tests that only need a valid set share one.
var sharedBrokerSecretsByNamespace = map[string][]*unstructured.Unstructured{}

// sharedBrokerSecrets returns the message broker secrets for a namespace,
// generating them the first time a test asks. Callers must not modify the
// result.
func sharedBrokerSecrets(t *testing.T, namespace string) []*unstructured.Unstructured {
	t.Helper()

	// reuse the set already generated for the namespace
	sharedBrokerSecretsMutex.Lock()
	defer sharedBrokerSecretsMutex.Unlock()
	if secrets, ok := sharedBrokerSecretsByNamespace[namespace]; ok {
		return secrets
	}

	// generate the set in a scratch cluster
	cpi := &ControlPlaneInstaller{Opts: Options{Namespace: namespace}}
	scratch := jsonDynamicClient{testKubeClient()}
	require.NoError(t, cpi.ensureMessageBrokerSecrets(scratch))

	// keep what the install stored
	var secrets []*unstructured.Unstructured
	for _, name := range brokerSecretNames {
		secret, err := cpi.getSecret(scratch, name)
		require.NoError(t, err)
		require.NotNil(t, secret, "the broker secret %s was not created", name)
		secrets = append(secrets, secret)
	}
	sharedBrokerSecretsByNamespace[namespace] = secrets

	return secrets
}

// seedBrokerSecrets stores the shared message broker secrets in the cluster so
// an install finds them and generates none. A cluster that already holds any
// of them is left as it is, since a test that seeds its own has a reason to.
func seedBrokerSecrets(t *testing.T, cpi *ControlPlaneInstaller, kubeClient dynamic.Interface) {
	t.Helper()

	// leave a cluster that holds any of the secrets alone
	for _, name := range brokerSecretNames {
		existing, err := cpi.getSecret(kubeClient, name)
		require.NoError(t, err)
		if existing != nil {
			return
		}
	}

	// store a copy of each shared secret
	for _, secret := range sharedBrokerSecrets(t, cpi.Opts.Namespace) {
		_, err := kubeClient.Resource(secretGVR).Namespace(cpi.Opts.Namespace).Create(
			context.Background(),
			secret.DeepCopy(),
			metav1.CreateOptions{},
		)
		require.NoError(t, err)
	}
}

// ensureSharedBrokerSecrets runs ensureMessageBrokerSecrets on a cluster
// seeded with the shared secrets, for tests that need the secrets to exist and
// not how they were generated.
func ensureSharedBrokerSecrets(t *testing.T, cpi *ControlPlaneInstaller, kubeClient dynamic.Interface) {
	t.Helper()

	// seed the shared secrets, then run the install step over them
	seedBrokerSecrets(t, cpi, kubeClient)
	require.NoError(t, cpi.ensureMessageBrokerSecrets(kubeClient))
}
