package v0

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
)

// precedenceManagedProject is the project of the GKE cluster being
// authenticated to in the precedence tests.
const precedenceManagedProject = "the-managed-project"

// precedenceStoredEmail is the service account stored on the managed
// cluster's GCP provider, when it has one.
const precedenceStoredEmail = "threeport@the-managed-project.iam.gserviceaccount.com"

// asCaller makes this process the given caller for the length of a test.
func asCaller(t *testing.T, caller GkeCaller) {
	t.Helper()
	previous := gkeCaller
	gkeCaller = caller
	t.Cleanup(func() { gkeCaller = previous })
}

// withAmbient makes this process's ambient credentials resolve as given for
// the length of a test.
func withAmbient(t *testing.T, resolve func(context.Context) (*google.Credentials, error)) {
	t.Helper()
	previous := ambientCredentials
	ambientCredentials = resolve
	t.Cleanup(func() { ambientCredentials = previous })
}

// workloadIdentity is ambient credentials from the metadata server: a GKE
// pod's own Workload Identity principal.
func workloadIdentity(context.Context) (*google.Credentials, error) {
	return &google.Credentials{
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "workload-identity"}),
	}, nil
}

// noAmbient is a process with no ambient Google credentials at all.
func noAmbient(context.Context) (*google.Credentials, error) {
	return nil, errors.New("google: could not find default credentials")
}

// gcloudLogin writes the file `gcloud auth application-default login` leaves
// and points this process at it, so the real resolution path reads it.
func gcloudLogin(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "application_default_credentials.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "type": "authorized_user",
  "client_id": "764086051850-example.apps.googleusercontent.com",
  "client_secret": "a-client-secret",
  "refresh_token": "a-refresh-token"
}`), 0600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

// TestGkeCredentialPrecedence covers every row of the decision gkeAuthSource
// makes, in one place, so a change made for one caller cannot quietly break
// another - the way the ambient fallback for non-GKE hosts (#470) let an
// unauthorized identity win (#577), and the fix for that locked out a person
// running tptctl with their own gcloud login (#646).
func TestGkeCredentialPrecedence(t *testing.T) {
	encryptionKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	stored, err := encryption.Encrypt(
		encryptionKey,
		serviceAccountCredentials(t, precedenceManagedProject, precedenceStoredEmail),
	)
	require.NoError(t, err)

	for _, testCase := range []struct {
		name         string
		caller       GkeCaller
		hostProvider string
		hostProject  string
		ambient      func(*testing.T)
		stored       string

		// the identity that must arrive: "ambient" for this process's own
		// credentials, the stored account's email for the provider's
		wantIdentity string
		wantError    string
	}{
		{
			// #646: the genesis provider stores nothing, and the person's
			// login is the identity that created the cluster
			name:         "an operator with a gcloud login acts as themselves",
			caller:       GkeCallerOperator,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      gcloudLogin,
			wantIdentity: "ambient",
		},
		{
			name:         "an operator's own login comes before a stored account",
			caller:       GkeCallerOperator,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      gcloudLogin,
			stored:       stored,
			wantIdentity: "ambient",
		},
		{
			name:         "an operator with no login uses the stored account",
			caller:       GkeCallerOperator,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      func(t *testing.T) { withAmbient(t, noAmbient) },
			stored:       stored,
			wantIdentity: precedenceStoredEmail,
		},
		{
			name:         "an operator with neither is told how to sign in",
			caller:       GkeCallerOperator,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      func(t *testing.T) { withAmbient(t, noAmbient) },
			wantError:    "gcloud auth application-default login",
		},
		{
			// #470: the genesis control plane's controllers on their own
			// host cluster, and a child control plane's on GKE once the
			// records they look up exist (#653)
			name:         "a controller hosted on GKE in the cluster's project acts as its Workload Identity principal",
			caller:       GkeCallerController,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      func(t *testing.T) { withAmbient(t, workloadIdentity) },
			wantIdentity: "ambient",
		},
		{
			name:         "a controller hosted on GKE in the cluster's project, behind a credentials file, uses the stored account",
			caller:       GkeCallerController,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  precedenceManagedProject,
			ambient:      gcloudLogin,
			stored:       stored,
			wantIdentity: precedenceStoredEmail,
		},
		{
			// #577: its principal is from a pool this cluster does not
			// authenticate against
			name:         "a controller hosted on GKE in another project uses the stored account",
			caller:       GkeCallerController,
			hostProvider: v0.KubernetesRuntimeInfraProviderGKE,
			hostProject:  "a-different-project",
			ambient:      func(t *testing.T) { withAmbient(t, workloadIdentity) },
			stored:       stored,
			wantIdentity: precedenceStoredEmail,
		},
		{
			// #577: credentials lying around on the host are not the
			// control plane's
			name:         "a controller hosted on EKS uses the stored account, not incidental credentials",
			caller:       GkeCallerController,
			hostProvider: v0.KubernetesRuntimeInfraProviderEKS,
			ambient:      gcloudLogin,
			stored:       stored,
			wantIdentity: precedenceStoredEmail,
		},
		{
			name:         "a controller with no identity that applies says so",
			caller:       GkeCallerController,
			hostProvider: v0.KubernetesRuntimeInfraProviderEKS,
			ambient:      gcloudLogin,
			wantError:    "no stored service account credentials",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			asCaller(t, testCase.caller)
			testCase.ambient(t)
			srv := threeportAPI(t, testCase.hostProvider, testCase.hostProject, precedenceManagedProject, testCase.stored)
			defer srv.Close()

			runtimeId := uint(7)
			tokenSource, subject, err := gkeAuthSource(
				&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &runtimeId}},
				&http.Client{},
				strings.TrimPrefix(srv.URL, "http://"),
				encryptionKey,
			)

			if testCase.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.wantError)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tokenSource)
			if testCase.wantIdentity == "ambient" {
				assert.Empty(t, subject, "this process's own credentials, not a stored account")
			} else {
				assert.Equal(t, testCase.wantIdentity, subject)
			}
		})
	}
}

// TestAControllerActsAsItsOwnPrincipal checks the Workload Identity row
// reaches the token itself, not only the identity reported for it.
func TestAControllerActsAsItsOwnPrincipal(t *testing.T) {
	asCaller(t, GkeCallerController)
	withAmbient(t, workloadIdentity)
	srv := threeportAPI(t, v0.KubernetesRuntimeInfraProviderGKE, precedenceManagedProject, precedenceManagedProject, "")
	defer srv.Close()

	runtimeId := uint(7)
	tokenSource, _, err := gkeAuthSource(
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &runtimeId}},
		&http.Client{},
		strings.TrimPrefix(srv.URL, "http://"),
		"",
	)
	require.NoError(t, err)
	token, err := tokenSource.Token()
	require.NoError(t, err)
	assert.Equal(t, "workload-identity", token.AccessToken)
}

// TestAnOperatorCannotNameTheControlPlanesIdentity covers the one question an
// operator's process must not answer: which identity a managed cluster
// authorizes for the control plane. Its own would read as a Workload Identity
// principal it is not.
func TestAnOperatorCannotNameTheControlPlanesIdentity(t *testing.T) {
	asCaller(t, GkeCallerOperator)
	gcloudLogin(t)

	runtimeId := uint(7)
	_, err := GkeAuthSubject(
		&v0.KubernetesRuntimeInstance{Common: v0.Common{ID: &runtimeId}},
		&http.Client{},
		"unused",
		"",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "an operator's process cannot present it")
}

// TestAProgramThatDeclaresNothingIsAController covers the default: a program
// that never declares itself an operator is held to the narrower policy.
func TestAProgramThatDeclaresNothingIsAController(t *testing.T) {
	assert.Equal(t, GkeCallerController, gkeCaller)
}
