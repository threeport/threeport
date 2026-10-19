package v0

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2/google"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// serviceAccountCredentials builds a credential of the shape Google's libraries
// accept, so a test exercises the real resolution path rather than a stub.
func serviceAccountCredentials(t *testing.T, project, email string) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY",
		Bytes: func() []byte {
			der, err := x509.MarshalPKCS8PrivateKey(key)
			require.NoError(t, err)
			return der
		}(),
	})

	credentials, err := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  project,
		"private_key_id":              "a-key-id",
		"private_key":                 string(keyPem),
		"client_email":                email,
		"client_id":                   "1234567890",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
	})
	require.NoError(t, err)

	return string(credentials)
}

// threeportAPI serves the records the GKE auth path reads: the control plane
// host runtime and the managed runtime, each with its own GCP provider.
//
// Both projects are reached the same way - runtime instance, its GKE record,
// then that record's provider - so the fake routes on which instance is asked
// about rather than serving one provider for every lookup.
func threeportAPI(
	t *testing.T,
	hostProvider string,
	hostProject string,
	managedProject string,
	storedCredentials string,
) *httptest.Server {
	t.Helper()

	hostInstanceId, hostDefinitionId := uint(1), uint(1)
	hostInstanceName := "control-plane-host"
	isHost := true
	hostProviderId, managedProviderId := uint(1), uint(2)

	respond := func(w http.ResponseWriter, objects ...apiserver_lib.Object) {
		body, err := json.Marshal(apiserver_lib.Response{
			Status: apiserver_lib.Status{Code: http.StatusOK, Message: "OK"},
			Data:   objects,
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == v0.PathKubernetesRuntimeInstances:
			respond(w, v0.KubernetesRuntimeInstance{
				Common:                        v0.Common{ID: &hostInstanceId},
				Instance:                      v0.Instance{Name: &hostInstanceName},
				ThreeportControlPlaneHost:     &isHost,
				KubernetesRuntimeDefinitionID: &hostDefinitionId,
			})
		case strings.HasPrefix(r.URL.Path, v0.PathKubernetesRuntimeDefinitions):
			// a name and a provider, and no InfraProviderAccountName - the
			// shape CreateGenesisControlPlane writes for every bootstrapped
			// control plane
			respond(w, v0.KubernetesRuntimeDefinition{
				Definition:    v0.Definition{Name: &hostInstanceName},
				InfraProvider: &hostProvider,
			})
		case strings.Contains(r.URL.Path, "gcp-gke-kubernetes-runtime-instances"):
			providerId := managedProviderId
			if r.URL.Query().Get("kubernetesruntimeinstanceid") == fmt.Sprint(hostInstanceId) {
				providerId = hostProviderId
			}
			respond(w, v0.GcpGkeKubernetesRuntimeInstance{GcpProviderID: &providerId})
		case strings.HasPrefix(r.URL.Path, v0.PathGcpProviders):
			if strings.HasSuffix(r.URL.Path, fmt.Sprint(hostProviderId)) {
				respond(w, v0.GcpProvider{ProjectID: &hostProject})
				return
			}
			provider := v0.GcpProvider{ProjectID: &managedProject}
			if storedCredentials != "" {
				provider.ServiceAccountCredentials = &storedCredentials
			}
			respond(w, provider)
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestGkeAuthSourceIgnoresAmbientCredentialsTheClusterCannotAuthorize covers the
// case ambient credentials resolving does not decide.
//
// google.DefaultTokenSource answers whether any Google credential is reachable
// in this process, not whether the identity it yields is one the managed cluster
// was told to authorize. A control plane hosted outside GKE presents no Workload
// Identity principal at all, and one hosted in another project presents a
// principal from a pool that cluster's API server does not authenticate against,
// so in both cases nothing can arrive as the identity the binding names. Keying
// on resolution leaves the stored credential - the one that provisioned the
// cluster - unused, and fails at the cluster instead, far from the cause.
func TestGkeAuthSourceIgnoresAmbientCredentialsTheClusterCannotAuthorize(t *testing.T) {
	const (
		managedProject = "the-managed-project"
		storedEmail    = "threeport@the-managed-project.iam.gserviceaccount.com"
	)

	encryptionKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	stored, err := encryption.Encrypt(
		encryptionKey,
		serviceAccountCredentials(t, managedProject, storedEmail),
	)
	require.NoError(t, err)

	// ambient credentials this process can resolve, unrelated to the cluster
	ambient := filepath.Join(t.TempDir(), "ambient.json")
	require.NoError(t, os.WriteFile(
		ambient,
		[]byte(serviceAccountCredentials(t, "some-other-project", "ambient@elsewhere.iam.gserviceaccount.com")),
		0600,
	))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", ambient)

	runtimeId := uint(7)
	runtime := &v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &runtimeId}}

	for _, testCase := range []struct {
		name           string
		hostProvider   string
		hostProject    string
		managedProject string
	}{
		{
			name:           "control plane hosted outside GKE has no workload identity to present",
			hostProvider:   v0.KubernetesRuntimeInfraProviderEKS,
			hostProject:    "",
			managedProject: managedProject,
		},
		{
			name:           "a workload identity principal from another project reaches nothing here",
			hostProvider:   v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:    "a-different-project",
			managedProject: managedProject,
		},
		{
			// the case a project comparison alone gets wrong: this process is
			// where a Workload Identity principal would come from, but a
			// credentials file stands in front of it and names another account
			name:           "same project, yet a credentials file is not the principal",
			hostProvider:   v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:    managedProject,
			managedProject: managedProject,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			srv := threeportAPI(t, testCase.hostProvider, testCase.hostProject, testCase.managedProject, stored)
			defer srv.Close()

			subject, err := GkeAuthSubject(
				runtime,
				&http.Client{},
				strings.TrimPrefix(srv.URL, "http://"),
				encryptionKey,
			)
			require.NoError(t, err)
			assert.Equal(
				t, storedEmail, subject,
				"the stored account is the identity this cluster can authorize",
			)
		})
	}
}

// TestAmbientIdentityReachesCluster covers which control planes could present a
// Workload Identity principal this cluster is able to authorize.
//
// The binding a managed cluster is given names a principal of the control
// plane's Workload Identity pool, and a GKE API server authenticates pod tokens
// against its own project's pool. From another project, or from a control plane
// not hosted on GKE at all, no request can arrive as that principal, so the
// binding authorizes nobody.
func TestAmbientIdentityReachesCluster(t *testing.T) {
	const managedProject = "the-managed-project"

	for _, testCase := range []struct {
		name           string
		hostProvider   string
		hostProject    string
		managedProject string
		reaches        bool
	}{
		{
			name:           "hosted outside GKE",
			hostProvider:   v0.KubernetesRuntimeInfraProviderEKS,
			hostProject:    "",
			managedProject: managedProject,
		},
		{
			// "not GKE-hosted" is recorded as an empty project, so a managed
			// provider missing its own project would otherwise match it
			name:           "an unrecorded project is not the same project",
			hostProvider:   v0.KubernetesRuntimeInfraProviderEKS,
			hostProject:    "",
			managedProject: "",
		},
		{
			name:           "GKE-hosted in another project",
			hostProvider:   v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:    "a-different-project",
			managedProject: managedProject,
		},
		{
			name:           "GKE-hosted in this cluster's project",
			hostProvider:   v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:    managedProject,
			managedProject: managedProject,
			reaches:        true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			srv := threeportAPI(t, testCase.hostProvider, testCase.hostProject, testCase.managedProject, "")
			defer srv.Close()

			reaches, err := ambientIdentityReachesCluster(
				&v0.GcpProvider{ProjectID: &testCase.managedProject},
				&http.Client{},
				strings.TrimPrefix(srv.URL, "http://"),
			)
			require.NoError(t, err)
			assert.Equal(t, testCase.reaches, reaches)
		})
	}
}

// TestCredentialsAreWorkloadIdentity covers the second half of that decision:
// whether this process arrives as a Workload Identity principal at all.
//
// Only the metadata server yields the pod's own principal. A credentials file
// names some other service account, and that account is what reaches the
// cluster - while a binding told this is the Workload Identity path names a
// principal nothing authenticates as.
func TestCredentialsAreWorkloadIdentity(t *testing.T) {
	assert.True(
		t, credentialsAreWorkloadIdentity(&google.Credentials{}),
		"no file was read, so the token came from the metadata server",
	)
	assert.False(
		t, credentialsAreWorkloadIdentity(&google.Credentials{
			JSON: []byte(serviceAccountCredentials(t, "a-project", "someone@a-project.iam.gserviceaccount.com")),
		}),
		"a credentials file names an account, not this pod's principal",
	)
	assert.False(t, credentialsAreWorkloadIdentity(nil))
}

// TestGkeAuthSourceSaysSoWhenNoIdentityApplies covers the case where the stored
// credential is absent and ambient credentials are not this cluster's.
//
// Returning the ambient one anyway is what makes the failure opaque: the caller
// gets a token source that looks valid and finds out at the GKE API server, with
// nothing naming the cause.
func TestGkeAuthSourceSaysSoWhenNoIdentityApplies(t *testing.T) {
	ambient := filepath.Join(t.TempDir(), "ambient.json")
	require.NoError(t, os.WriteFile(
		ambient,
		[]byte(serviceAccountCredentials(t, "some-other-project", "ambient@elsewhere.iam.gserviceaccount.com")),
		0600,
	))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", ambient)

	encryptionKey, err := encryption.GenerateKey()
	require.NoError(t, err)

	srv := threeportAPI(t, v0.KubernetesRuntimeInfraProviderEKS, "", "the-managed-project", "")
	defer srv.Close()

	runtimeId := uint(7)
	_, err = GkeAuthSubject(
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &runtimeId}},
		&http.Client{},
		strings.TrimPrefix(srv.URL, "http://"),
		encryptionKey,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no stored service account credentials")
}
