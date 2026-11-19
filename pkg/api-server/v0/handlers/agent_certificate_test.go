package handlers

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	agentTestOnce sync.Once
	agentTestCa   *auth.AuthConfig
	agentTestCsr  string
	agentTestErr  error
)

// agentTestMaterial returns one CA and one certificate request for the whole
// package: generating 4096-bit keys is the slow part of every test here.
func agentTestMaterial(t *testing.T) (*auth.AuthConfig, string) {
	t.Helper()
	agentTestOnce.Do(func() {
		agentTestCa, agentTestErr = auth.GetAuthConfig()
		if agentTestErr != nil {
			return
		}
		agentTestCsr, _, agentTestErr = auth.GenerateCertificateRequest("requester")
	})
	require.NoError(t, agentTestErr)
	return agentTestCa, agentTestCsr
}

// writeSigningCa lays the CA out the way the API pod has it mounted and
// returns the directory.
func writeSigningCa(t *testing.T) string {
	t.Helper()
	ca, _ := agentTestMaterial(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte(ca.CAPemEncoded), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), []byte(ca.CAPrivateKeyPemEncoded), 0o600))
	return dir
}

// runtimeController is the caller the API accepts agent certificate requests from.
var runtimeController = api_lib.CallerIdentity{
	CommonName:         auth.CNKubernetesRuntimeController,
	Organization:       api_lib.CoreApiNamespace,
	OrganizationalUnit: auth.OUControlPlane,
}

// runtimeExistsFor reports only the named runtime instances as existing.
func runtimeExistsFor(names ...string) func(context.Context, string) (bool, error) {
	return func(_ context.Context, name string) (bool, error) {
		for _, n := range names {
			if n == name {
				return true, nil
			}
		}
		return false, nil
	}
}

// statusOf returns the HTTP status an error maps to, or 0 when it is not one
// the caller is told about.
func statusOf(err error) int {
	var httpErr *util.HttpError
	if errors.As(err, &httpErr) {
		return httpErr.GetStatusCode()
	}
	return 0
}

// TestIssueAgentCertificateIssuesFromTheCa covers the certificate a permitted
// request gets and the record of it.
func TestIssueAgentCertificateIssuesFromTheCa(t *testing.T) {
	ca, csr := agentTestMaterial(t)
	core, logs := observer.New(zap.InfoLevel)

	certificate, err := issueAgentCertificate(
		context.Background(),
		writeSigningCa(t),
		runtimeController,
		api_v0.AgentCertificateRequest{RuntimeInstanceName: "east", CertificateSigningRequest: csr},
		runtimeExistsFor("east"),
		zap.New(core),
	)
	require.NoError(t, err)

	block, _ := pem.Decode([]byte(certificate.Certificate))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "threeport-agent-east", cert.Subject.CommonName)
	assert.Equal(t, []string{api_lib.CoreApiNamespace}, cert.Subject.Organization)
	assert.Equal(t, []string{auth.OUControlPlane}, cert.Subject.OrganizationalUnit)

	caBlock, _ := pem.Decode([]byte(certificate.CACertificate))
	require.NotNil(t, caBlock)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	assert.NoError(t, err)
	assert.Equal(t, ca.CAPemEncoded, certificate.CACertificate)

	// nothing of the CA's private key leaves the API
	assert.NotContains(t, certificate.Certificate+certificate.CACertificate, "PRIVATE KEY")

	// the issue is on record
	entries := logs.FilterMessage("issued agent certificate").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "threeport-agent-east", fields["commonName"])
	assert.Equal(t, cert.SerialNumber.Text(16), fields["serialNumber"])
	assert.Equal(t, auth.CNKubernetesRuntimeController, fields["requestedBy"])
}

// TestIssueAgentCertificateRefusals covers every request that gets no
// certificate, and the status each owes the caller.
func TestIssueAgentCertificateRefusals(t *testing.T) {
	_, csr := agentTestMaterial(t)
	caDir := writeSigningCa(t)

	otherController := runtimeController
	otherController.CommonName = "helm-workload-controller"
	notControlPlane := runtimeController
	notControlPlane.OrganizationalUnit = "database"
	owner := runtimeController
	owner.CommonName = "threeport-owner"

	valid := api_v0.AgentCertificateRequest{RuntimeInstanceName: "east", CertificateSigningRequest: csr}
	withName := func(name string) api_v0.AgentCertificateRequest {
		r := valid
		r.RuntimeInstanceName = name
		return r
	}
	withCsr := func(csr string) api_v0.AgentCertificateRequest {
		r := valid
		r.CertificateSigningRequest = csr
		return r
	}

	cases := []struct {
		name    string
		caDir   string
		caller  api_lib.CallerIdentity
		request api_v0.AgentCertificateRequest
		want    int
	}{
		{"another controller", caDir, otherController, valid, http.StatusForbidden},
		{"a controller's unit is not enough without its name", caDir, api_lib.CallerIdentity{OrganizationalUnit: auth.OUControlPlane}, valid, http.StatusForbidden},
		{"the owner identity", caDir, owner, valid, http.StatusForbidden},
		{"the right name in another unit", caDir, notControlPlane, valid, http.StatusForbidden},
		{"no caller", caDir, api_lib.CallerIdentity{}, valid, http.StatusForbidden},
		{"auth disabled, no CA mounted", t.TempDir(), runtimeController, valid, http.StatusConflict},
		{"empty runtime name", caDir, runtimeController, withName(""), http.StatusBadRequest},
		{"runtime name that is not a name", caDir, runtimeController, withName("east,CN=threeport-owner"), http.StatusBadRequest},
		{"empty request", caDir, runtimeController, withCsr(""), http.StatusBadRequest},
		{"oversized request", caDir, runtimeController, withCsr(strings.Repeat("a", maxCertificateRequestLength+1)), http.StatusBadRequest},
		{"request that is not a request", caDir, runtimeController, withCsr("junk"), http.StatusBadRequest},
		{"runtime that does not exist", caDir, runtimeController, withName("west"), http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)

			certificate, err := issueAgentCertificate(
				context.Background(), tc.caDir, tc.caller, tc.request, runtimeExistsFor("east"), zap.New(core),
			)

			require.Error(t, err)
			assert.Nil(t, certificate)
			assert.Equal(t, tc.want, statusOf(err))
			assert.Zero(t, logs.FilterMessage("issued agent certificate").Len(), "a refusal issues nothing")
		})
	}
}

// TestIssueAgentCertificateServerFaults covers the failures that are the API's
// own, which the caller is not given the detail of.
func TestIssueAgentCertificateServerFaults(t *testing.T) {
	_, csr := agentTestMaterial(t)
	request := api_v0.AgentCertificateRequest{RuntimeInstanceName: "east", CertificateSigningRequest: csr}

	// a certificate with no key
	halfDir := writeSigningCa(t)
	require.NoError(t, os.Remove(filepath.Join(halfDir, "tls.key")))

	// a key that is not the certificate's
	mismatchDir := writeSigningCa(t)
	other, err := auth.GetAuthConfig()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mismatchDir, "tls.key"), []byte(other.CAPrivateKeyPemEncoded), 0o600))

	for name, run := range map[string]func() error{
		"certificate without key": func() error {
			_, err := issueAgentCertificate(context.Background(), halfDir, runtimeController, request, runtimeExistsFor("east"), zap.NewNop())
			return err
		},
		"key that is not the certificate's": func() error {
			_, err := issueAgentCertificate(context.Background(), mismatchDir, runtimeController, request, runtimeExistsFor("east"), zap.NewNop())
			return err
		},
		"database failure": func() error {
			_, err := issueAgentCertificate(context.Background(), writeSigningCa(t), runtimeController, request,
				func(context.Context, string) (bool, error) { return false, errors.New("connection refused") }, zap.NewNop())
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			require.Error(t, err)
			assert.Zero(t, statusOf(err), "a server fault is not reported as a client error")
		})
	}
}

// newAgentCertificateServer returns an echo server that serves the handler the
// way the API does, with the caller the request is made as, over a database
// holding the named runtime instances.
func newAgentCertificateServer(t *testing.T, caller api_lib.CallerIdentity, runtimes ...string) *echo.Echo {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&api_v0.KubernetesRuntimeInstance{}))
	for _, name := range runtimes {
		// the create hooks validate a whole runtime; only the name matters here
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&api_v0.KubernetesRuntimeInstance{
			Instance:                      api_v0.Instance{Name: util.Ptr(name)},
			Location:                      util.Ptr("test"),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
		}).Error)
	}

	h := Handler{DB: db, Logger: zap.NewNop()}
	e := echo.New()
	e.POST(api_v0.PathAgentCertificates, func(c echo.Context) error {
		c.SetRequest(c.Request().WithContext(api_lib.WithCaller(c.Request().Context(), caller)))
		return h.CreateAgentCertificate(c)
	})

	return e
}

// post sends the request to the server and decodes the answer.
func post(t *testing.T, e *echo.Echo, body any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, api_v0.PathAgentCertificates, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
	return rec.Code, decoded
}

// TestCreateAgentCertificateHandler covers the endpoint end to end: the route
// the controller calls, the database lookup, and the answer's shape.
func TestCreateAgentCertificateHandler(t *testing.T) {
	_, csr := agentTestMaterial(t)
	original := signingCaDir
	signingCaDir = writeSigningCa(t)
	t.Cleanup(func() { signingCaDir = original })

	request := map[string]string{"RuntimeInstanceName": "east", "CertificateSigningRequest": csr}

	t.Run("the runtime controller gets a certificate", func(t *testing.T) {
		code, body := post(t, newAgentCertificateServer(t, runtimeController, "east"), request)

		require.Equal(t, http.StatusCreated, code)
		data := body["Data"].([]any)
		require.Len(t, data, 1)
		certificate := data[0].(map[string]any)
		assert.Contains(t, certificate["Certificate"], "BEGIN CERTIFICATE")
		assert.Contains(t, certificate["CACertificate"], "BEGIN CERTIFICATE")
	})

	t.Run("another controller does not", func(t *testing.T) {
		other := runtimeController
		other.CommonName = "gcp-controller"

		code, _ := post(t, newAgentCertificateServer(t, other, "east"), request)

		assert.Equal(t, http.StatusForbidden, code)
	})

	t.Run("a runtime that is not in the database gets none", func(t *testing.T) {
		code, _ := post(t, newAgentCertificateServer(t, runtimeController, "west"), request)

		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("a body that is not a request is a client error", func(t *testing.T) {
		code, _ := post(t, newAgentCertificateServer(t, runtimeController, "east"), map[string]int{"RuntimeInstanceName": 7})

		assert.Equal(t, http.StatusBadRequest, code)
	})
}
