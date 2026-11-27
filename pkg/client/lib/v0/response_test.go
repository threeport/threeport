package v0

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TestGetResponse_ConflictCauses covers 409 responses that map to a
// delete-in-progress, delete-blocked, or general conflict error.
func TestGetResponse_ConflictCauses(t *testing.T) {
	// build 409 message cases
	cases := []struct {
		name      string
		message   string
		wantIs    []error
		wantNotIs []error
	}{
		{
			name:      "delete already underway",
			message:   fmt.Sprintf("object with ID 7 %s", api_v0.ErrMsgAlreadyBeingDeleted),
			wantIs:    []error{ErrDeleteInProgress, ErrConflict},
			wantNotIs: []error{ErrDeleteBlocked},
		},
		{
			name:      "delete blocked by attached objects",
			message:   fmt.Sprintf("threeport.io/v0.Foo/1 %s while 1 object(s) still reference it", api_v0.ErrMsgDeleteBlocked),
			wantIs:    []error{ErrDeleteBlocked, ErrConflict},
			wantNotIs: []error{ErrDeleteInProgress},
		},
		{
			name:      "delete blocked by related instances",
			message:   "terraform definition has related terraform instances - " + api_v0.ErrMsgDeleteBlocked,
			wantIs:    []error{ErrDeleteBlocked, ErrConflict},
			wantNotIs: []error{ErrDeleteInProgress},
		},
		{
			name:      "name already taken stays a general conflict",
			message:   "object with provided name already exists",
			wantIs:    []error{ErrConflict},
			wantNotIs: []error{ErrDeleteInProgress, ErrDeleteBlocked},
		},
	}

	// run each 409 message case
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// serve a 409 with the case message
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := json.Marshal(apiserver_lib.Response{
					Status: apiserver_lib.Status{
						Code:    http.StatusConflict,
						Message: http.StatusText(http.StatusConflict),
						Error:   tc.message,
					},
				})
				require.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			// call GetResponse against the 409
			_, err := GetResponse(
				&http.Client{},
				strings.TrimPrefix(srv.URL, "http://")+"/v0/objects",
				http.MethodDelete,
				bytes.NewBuffer(nil),
				nil,
				http.StatusOK,
			)
			// require an error
			require.Error(t, err)

			// accept the expected errors
			for _, want := range tc.wantIs {
				assert.Truef(t, errors.Is(err, want), "expected errors.Is(..., %v), got %v", want, err)
			}
			// reject the excluded errors
			for _, not := range tc.wantNotIs {
				assert.Falsef(t, errors.Is(err, not), "did not expect errors.Is(..., %v), got %v", not, err)
			}
		})
	}
}

// TestApiUrl covers the scheme chosen for each kind of client: plain http
// unless the transport is set up for TLS.
func TestApiUrl(t *testing.T) {
	cases := []struct {
		name   string
		client *http.Client
		want   string
	}{
		{
			name:   "a client with no transport calls over http",
			client: &http.Client{},
			want:   "http://localhost:80/version",
		},
		{
			name:   "auth disabled calls over http",
			client: &http.Client{Transport: &CustomTransport{IsTlsEnabled: false}},
			want:   "http://localhost:80/version",
		},
		{
			name:   "auth enabled calls over https",
			client: &http.Client{Transport: &CustomTransport{IsTlsEnabled: true}},
			want:   "https://localhost:80/version",
		},
		{
			name:   "a plain transport with TLS config calls over https",
			client: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{}}},
			want:   "https://localhost:80/version",
		},
		{
			name:   "a plain transport without TLS config calls over http",
			client: &http.Client{Transport: &http.Transport{}},
			want:   "http://localhost:80/version",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ApiUrl(tc.client, "localhost:80/version"))
		})
	}
}
