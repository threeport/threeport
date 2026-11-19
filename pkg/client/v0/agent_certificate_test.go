package v0

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// TestCreateAgentCertificateSendsOneRequest covers the request the API gets and
// the certificate read from its answer.
func TestCreateAgentCertificateSendsOneRequest(t *testing.T) {
	var requests []string
	var sent map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"Data":[{"Certificate":"CERT","CACertificate":"CA"}]}`)
	}))
	defer server.Close()

	certificate, err := CreateAgentCertificate(server.Client(), apiAddr(server), "east", "CSR")

	require.NoError(t, err)
	assert.Equal(t, []string{"POST " + v0.PathAgentCertificates}, requests)
	assert.Equal(t, map[string]string{"RuntimeInstanceName": "east", "CertificateSigningRequest": "CSR"}, sent)
	assert.Equal(t, "CERT", certificate.Certificate)
	assert.Equal(t, "CA", certificate.CACertificate)
}

// TestCreateAgentCertificateReportsRefusals covers the answers a caller has to
// tell apart: not allowed, unknown runtime, and a control plane without auth.
func TestCreateAgentCertificateReportsRefusals(t *testing.T) {
	cases := []struct {
		name   string
		status int
		wantIs error
	}{
		{"auth disabled", http.StatusConflict, client_lib.ErrConflict},
		{"forbidden", http.StatusForbidden, client_lib.ErrForbidden},
		{"unknown runtime", http.StatusNotFound, client_lib.ErrObjectNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"Status":{"Code":%d,"Error":"refused"}}`, tc.status)
			}))
			defer server.Close()

			certificate, err := CreateAgentCertificate(server.Client(), apiAddr(server), "east", "CSR")

			require.Error(t, err)
			assert.Nil(t, certificate)
			assert.ErrorIs(t, err, tc.wantIs)
		})
	}
}

// TestCreateAgentCertificateRejectsAnAnswerWithNoCertificate covers a server
// that answers 201 with nothing usable in it.
func TestCreateAgentCertificateRejectsAnAnswerWithNoCertificate(t *testing.T) {
	for name, body := range map[string]string{
		"no data":           `{"Data":[]}`,
		"empty certificate": `{"Data":[{"Certificate":"","CACertificate":"CA"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, body)
			}))
			defer server.Close()

			certificate, err := CreateAgentCertificate(server.Client(), apiAddr(server), "east", "CSR")

			assert.Error(t, err)
			assert.Nil(t, certificate)
		})
	}
}
