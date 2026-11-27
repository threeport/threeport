package v0

import (
	"crypto/tls"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
)

// The agent on a managed cluster is given its certificate by the control
// plane's API, which holds the CA key.  What these cover is that the installer
// asks only when the agent needs a certificate, sends no private key, and
// refuses an answer it cannot use.

var (
	apiCaOnce sync.Once
	apiCa     *auth.AuthConfig
	apiCaErr  error
)

// testApiCa returns one CA for the whole package; generating its key is slow.
func testApiCa(t *testing.T) *auth.AuthConfig {
	t.Helper()
	apiCaOnce.Do(func() { apiCa, apiCaErr = auth.GetAuthConfig() })
	require.NoError(t, apiCaErr)

	return apiCa
}

// apiIssuer stands in for the API: it signs the request it is sent and counts
// the calls.
func apiIssuer(t *testing.T, calls *[]string) AgentCertificateIssuer {
	t.Helper()
	ca := testApiCa(t)

	return func(request string) (*v0.AgentCertificate, error) {
		*calls = append(*calls, request)
		certificate, _, err := auth.SignClientCertificateRequest(
			ca, request, "threeport-agent-east", "threeport.io", auth.OUControlPlane,
		)
		if err != nil {
			return nil, err
		}

		return &v0.AgentCertificate{Certificate: certificate, CACertificate: ca.CAPemEncoded}, nil
	}
}

// stringData reads a secret's stringData as the installer wrote it.
func stringData(t *testing.T, secret *unstructured.Unstructured) map[string]string {
	t.Helper()
	data, found, err := unstructured.NestedStringMap(secret.Object, "stringData")
	require.NoError(t, err)
	require.True(t, found)

	return data
}

// getSecret returns the named secret, or nil when the cluster has none.
func getSecret(t *testing.T, cpi *ControlPlaneInstaller, client dynamic.Interface, name string) *unstructured.Unstructured {
	t.Helper()
	found, present, err := resourceInstalled(client, testMapper(), "", "v1", "Secret", cpi.Opts.Namespace, name)
	require.NoError(t, err)
	if !present {
		return nil
	}

	return found
}

// TestInstallAgentCredentialSecretsIssuesFromTheAPI covers the secrets an agent
// is given when the API signs its certificate.
func TestInstallAgentCredentialSecretsIssuesFromTheAPI(t *testing.T) {
	cpi := remoteAgentInstaller()
	client := testClient()
	var calls []string

	err := cpi.installAgentCredentialSecrets(client, testMapper(), issuedAgentCredentials(apiIssuer(t, &calls)))
	require.NoError(t, err)

	require.Len(t, calls, 1)
	assert.Contains(t, calls[0], "BEGIN CERTIFICATE REQUEST")
	assert.NotContains(t, calls[0], "PRIVATE KEY", "only a request for the public key is sent")

	ca := stringData(t, getSecret(t, cpi, client, agentCaSecretName))
	assert.Equal(t, testApiCa(t).CAPemEncoded, ca["tls.crt"])
	assert.NotContains(t, ca, "tls.key", "the agent is never given the CA's key")

	cert := stringData(t, getSecret(t, cpi, client, agentCertSecretName))
	_, err = tls.X509KeyPair([]byte(cert["tls.crt"]), []byte(cert["tls.key"]))
	assert.NoError(t, err, "the certificate is for the key the agent keeps")
}

// TestInstallAgentCredentialSecretsAsksOnlyWhenNeeded covers the repeat visits:
// the secrets are kept across installs, so a certificate fetched while they are
// there would be thrown away and left on the API's record.
func TestInstallAgentCredentialSecretsAsksOnlyWhenNeeded(t *testing.T) {
	t.Run("not when the agent has both secrets", func(t *testing.T) {
		cpi := remoteAgentInstaller()
		client := testClient(
			testSecret(cpi.Opts.Namespace, agentCaSecretName, nil),
			testSecret(cpi.Opts.Namespace, agentCertSecretName, nil),
		)
		var calls []string

		err := cpi.installAgentCredentialSecrets(client, testMapper(), issuedAgentCredentials(apiIssuer(t, &calls)))

		require.NoError(t, err)
		assert.Empty(t, calls)
	})

	for _, missing := range []string{agentCaSecretName, agentCertSecretName} {
		t.Run("when "+missing+" is missing", func(t *testing.T) {
			cpi := remoteAgentInstaller()
			var objects []runtime.Object
			for _, name := range []string{agentCaSecretName, agentCertSecretName} {
				if name != missing {
					objects = append(objects, testSecret(cpi.Opts.Namespace, name, nil))
				}
			}
			client := testClient(objects...)
			var calls []string

			err := cpi.installAgentCredentialSecrets(client, testMapper(), issuedAgentCredentials(apiIssuer(t, &calls)))

			require.NoError(t, err)
			assert.Len(t, calls, 1)
			assert.NotNil(t, getSecret(t, cpi, client, missing))
		})
	}
}

// TestIssuedAgentCredentialsRefusesWhatItCannotUse covers answers that would
// leave the agent with a certificate that does not work.
func TestIssuedAgentCredentialsRefusesWhatItCannotUse(t *testing.T) {
	ca := testApiCa(t)
	otherRequest, _, err := auth.GenerateCertificateRequest("someone-else")
	require.NoError(t, err)
	forOtherKey, _, err := auth.SignClientCertificateRequest(ca, otherRequest, "threeport-agent-east", "threeport.io", auth.OUControlPlane)
	require.NoError(t, err)

	cases := map[string]struct {
		issue AgentCertificateIssuer
		want  string
	}{
		"the API refuses": {
			issue: func(string) (*v0.AgentCertificate, error) { return nil, errors.New("forbidden") },
			want:  "forbidden",
		},
		"nothing comes back": {
			issue: func(string) (*v0.AgentCertificate, error) { return nil, nil },
			want:  "issued no client certificate",
		},
		"no CA certificate comes back": {
			issue: func(string) (*v0.AgentCertificate, error) {
				return &v0.AgentCertificate{Certificate: forOtherKey}, nil
			},
			want: "issued no client certificate",
		},
		"a certificate for another key": {
			issue: func(string) (*v0.AgentCertificate, error) {
				return &v0.AgentCertificate{Certificate: forOtherKey, CACertificate: ca.CAPemEncoded}, nil
			},
			want: "not for its key",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cpi := remoteAgentInstaller()
			client := testClient()

			err := cpi.installAgentCredentialSecrets(client, testMapper(), issuedAgentCredentials(tc.issue))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Nil(t, getSecret(t, cpi, client, agentCaSecretName), "no secret is written for an unusable certificate")
			assert.Nil(t, getSecret(t, cpi, client, agentCertSecretName))
		})
	}
}

// TestLocalAgentCredentials covers the installer that holds the CA itself: the
// one that creates a control plane signs its own agent's certificate.
func TestLocalAgentCredentials(t *testing.T) {
	ca := testApiCa(t)

	credentials, err := localAgentCredentials(ca)()
	require.NoError(t, err)

	_, err = tls.X509KeyPair([]byte(credentials.certificate), []byte(credentials.privateKey))
	assert.NoError(t, err)
	assert.Equal(t, ca.CAPemEncoded, credentials.caCertificate)
}

// TestInstallComputeSpaceControlPlaneComponentsNeedsAuthForACertificate covers
// the caller that asks for a certificate while the agent is set up without auth,
// which would give it a certificate its args never use.
func TestInstallComputeSpaceControlPlaneComponentsNeedsAuthForACertificate(t *testing.T) {
	cpi := NewInstaller()
	cpi.Opts.AuthEnabled = false
	var calls []string

	err := cpi.InstallComputeSpaceControlPlaneComponents(testClient(), testMapper(), apiIssuer(t, &calls))

	require.Error(t, err)
	assert.Empty(t, calls)
}
